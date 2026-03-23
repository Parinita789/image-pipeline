package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/go-chi/chi/v5"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/localstack"
	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/mongo"
	mongooptions "go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
	"golang.org/x/time/rate"

	"image-pipeline/internal/app"
	authpkg "image-pipeline/internal/auth"
	"image-pipeline/internal/handlers"
	"image-pipeline/internal/logger"
	"image-pipeline/internal/middleware"
	"image-pipeline/internal/queue"
	"image-pipeline/internal/repository"
	"image-pipeline/internal/resilence"
	s3client "image-pipeline/internal/s3"
	"image-pipeline/internal/services"
)

// ─── No-op email service for tests ───────────────────────────────────────────

type noopEmailService struct{}

func (n *noopEmailService) SendPasswordResetEmail(toEmail, token string) error { return nil }

// ─── Test Suite ───────────────────────────────────────────────────────────────

type suite struct {
	router    *chi.Mux
	db        *mongo.Database
	s3Client  *s3client.S3Client
	sqsClient *queue.SQSClient

	mongoC      testcontainers.Container
	localstackC *localstack.LocalStackContainer
}

var ts *suite

// ─── TestMain ─────────────────────────────────────────────────────────────────

func TestMain(m *testing.M) {
	ctx := context.Background()

	log, _ := zap.NewDevelopment()
	logger.Init(log)

	s, err := setupSuite(ctx)
	if err != nil {
		log.Fatal("failed to setup integration suite", zap.Error(err))
	}
	ts = s

	code := m.Run()

	_ = ts.mongoC.Terminate(ctx)
	_ = ts.localstackC.Terminate(ctx)

	os.Exit(code)
}

// ─── Setup ────────────────────────────────────────────────────────────────────

func setupSuite(ctx context.Context) (*suite, error) {
	s := &suite{}

	mongoC, err := mongodb.RunContainer(ctx, testcontainers.WithImage("mongo:7"))
	if err != nil {
		return nil, fmt.Errorf("start mongo container: %w", err)
	}
	s.mongoC = mongoC

	mongoURI, err := mongoC.ConnectionString(ctx)
	if err != nil {
		return nil, fmt.Errorf("mongo connection string: %w", err)
	}

	mongoClient, err := mongo.Connect(ctx, mongooptions.Client().ApplyURI(mongoURI))
	if err != nil {
		return nil, fmt.Errorf("mongo connect: %w", err)
	}
	s.db = mongoClient.Database("image-pipeline-test")

	lsC, err := localstack.RunContainer(ctx,
		testcontainers.WithImage("localstack/localstack:3"),
		testcontainers.WithEnv(map[string]string{"SERVICES": "s3,sqs"}),
	)
	if err != nil {
		return nil, fmt.Errorf("start localstack container: %w", err)
	}
	s.localstackC = lsC

	host, err := lsC.Host(ctx)
	if err != nil {
		return nil, fmt.Errorf("localstack host: %w", err)
	}
	port, err := lsC.MappedPort(ctx, "4566/tcp")
	if err != nil {
		return nil, fmt.Errorf("localstack port: %w", err)
	}
	lsEndpoint := fmt.Sprintf("http://%s:%s", host, port.Port())

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion("us-east-1"),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "")),
		awsconfig.WithEndpointResolverWithOptions(
			aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
				return aws.Endpoint{URL: lsEndpoint, HostnameImmutable: true}, nil
			}),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("aws config: %w", err)
	}

	s3Svc := s3.NewFromConfig(awsCfg, func(o *s3.Options) { o.UsePathStyle = true })
	bucketName := "test-bucket"
	_, err = s3Svc.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucketName)})
	if err != nil {
		return nil, fmt.Errorf("create s3 bucket: %w", err)
	}

	sqsSvc := sqs.NewFromConfig(awsCfg)
	queueOut, err := sqsSvc.CreateQueue(ctx, &sqs.CreateQueueInput{QueueName: aws.String("test-queue")})
	if err != nil {
		return nil, fmt.Errorf("create sqs queue: %w", err)
	}

	exec := resilence.NewExecutor(zap.NewNop(), "test", 1, 5*time.Second)
	s.s3Client = s3client.NewS3ClientFromConfig(awsCfg, bucketName)
	s.sqsClient = queue.NewSQSClientFromConfig(awsCfg, *queueOut.QueueUrl)

	imageRepo := repository.NewImageRepo(s.db, exec)
	_ = imageRepo.CreateIndexes(ctx)
	idemRepo := repository.NewIdemRepo(s.db)
	userRepo := repository.NewUserRepo(s.db)

	jwtSecret := "integration-test-secret"
	batchRepo := repository.NewBatchRepo(s.db)
	imageService := services.NewImageService(imageRepo, idemRepo, userRepo, batchRepo, s.s3Client, exec, s.sqsClient, exec, "")
	resetRepo := repository.NewPasswordResetRepo(s.db)
	authService := authpkg.NewAuthService(userRepo, resetRepo, &noopEmailService{}, jwtSecret)
	authHandler := authpkg.NewAuthHandler(authService)
	imageHandler := handlers.NewImageHandler(imageService)

	router := chi.NewRouter()
	app.RegisterRoutes(
		router,
		authHandler,
		imageHandler,
		jwtSecret,
		middleware.NewRateLimiter(rate.Every(time.Millisecond), 10000),
		idemRepo,
	)

	s.router = router
	return s, nil
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func registerAndLogin(t *testing.T, email, password string) string {
	t.Helper()

	body, _ := json.Marshal(map[string]string{
		"firstName": "Test", "lastName": "User", "email": email, "password": password,
	})
	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ts.router.ServeHTTP(httptest.NewRecorder(), req)

	body, _ = json.Marshal(map[string]string{"email": email, "password": password})
	req = httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	ts.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("login failed: status=%d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp.Data.Token == "" {
		t.Fatalf("empty token: %s", rr.Body.String())
	}
	return resp.Data.Token
}

func makeJPEG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 20, 20))
	for y := 0; y < 20; y++ {
		for x := 0; x < 20; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 100, B: 50, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, nil)
	return buf.Bytes()
}

// prepareFiles calls POST /images/prepare and returns the presigned upload descriptors.
type preparedFile struct {
	Key       string `json:"key"`
	UploadURL string `json:"uploadUrl"`
	Filename  string `json:"filename"`
	RequestID string `json:"requestId"`
}

func prepare(t *testing.T, token string, files []map[string]any) []preparedFile {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"files": files})
	req := httptest.NewRequest(http.MethodPost, "/images/prepare", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	ts.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("prepare failed: %d %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Data []preparedFile `json:"data"`
	}
	json.NewDecoder(rr.Body).Decode(&resp)
	return resp.Data
}

// putToS3 uploads bytes directly to a presigned URL (simulating the browser PUT).
func putToS3(t *testing.T, uploadURL, contentType string, data []byte) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPut, uploadURL, bytes.NewReader(data))
	req.Header.Set("Content-Type", contentType)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("S3 PUT failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("S3 PUT returned %d", resp.StatusCode)
	}
}

// confirm calls POST /images/confirm.
func confirm(t *testing.T, token, idemKey string, files []preparedFile) {
	t.Helper()
	fileList := make([]map[string]any, len(files))
	for i, f := range files {
		fileList[i] = map[string]any{"key": f.Key, "filename": f.Filename, "requestId": f.RequestID}
	}
	body, _ := json.Marshal(map[string]any{"files": fileList})
	req := httptest.NewRequest(http.MethodPost, "/images/confirm", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Idempotency-Key", idemKey)
	rr := httptest.NewRecorder()
	ts.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("confirm failed: %d %s", rr.Code, rr.Body.String())
	}
}

// ─── Auth Tests ───────────────────────────────────────────────────────────────

func TestIntegration_Register_HappyPath(t *testing.T) {
	body, _ := json.Marshal(map[string]string{
		"firstName": "Jane", "lastName": "Doe",
		"email": "jane_register@example.com", "password": "Password1!",
	})
	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	ts.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK && rr.Code != http.StatusCreated {
		t.Fatalf("expected 200/201, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestIntegration_Register_DuplicateEmail_Returns400(t *testing.T) {
	email := "duplicate@example.com"
	body, _ := json.Marshal(map[string]string{
		"firstName": "Jane", "lastName": "Doe", "email": email, "password": "Password1!",
	})
	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ts.router.ServeHTTP(httptest.NewRecorder(), req)

	body, _ = json.Marshal(map[string]string{
		"firstName": "Jane", "lastName": "Doe", "email": email, "password": "Password1!",
	})
	req = httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	ts.router.ServeHTTP(rr, req)

	if rr.Code == http.StatusOK || rr.Code == http.StatusCreated {
		t.Fatalf("expected error for duplicate email, got %d", rr.Code)
	}
}

func TestIntegration_Login_HappyPath(t *testing.T) {
	email := "login_happy@example.com"
	registerAndLogin(t, email, "MyPassword1!")

	body, _ := json.Marshal(map[string]string{"email": email, "password": "MyPassword1!"})
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	ts.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Data struct{ Token string `json:"token"` } `json:"data"`
	}
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp.Data.Token == "" {
		t.Error("expected JWT token in response")
	}
	if len(strings.Split(resp.Data.Token, ".")) != 3 {
		t.Errorf("invalid JWT format: %s", resp.Data.Token)
	}
}

func TestIntegration_Login_WrongPassword_Returns401(t *testing.T) {
	email := "login_wrong_pass@example.com"
	registerAndLogin(t, email, "Correct1!")

	body, _ := json.Marshal(map[string]string{"email": email, "password": "Wrong1!xx"})
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	ts.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

// ─── Prepare Tests ────────────────────────────────────────────────────────────

func TestIntegration_Prepare_HappyPath_ReturnsPresignedURLs(t *testing.T) {
	token := registerAndLogin(t, "prepare_happy@example.com", "Password1!")
	jpegData := makeJPEG()

	prepared := prepare(t, token, []map[string]any{
		{"filename": "photo.jpg", "contentType": "image/jpeg", "size": int64(len(jpegData))},
	})

	if len(prepared) != 1 {
		t.Fatalf("expected 1 prepared file, got %d", len(prepared))
	}
	if prepared[0].UploadURL == "" {
		t.Error("expected non-empty uploadUrl")
	}
	if !strings.HasPrefix(prepared[0].Key, "raw/") {
		t.Errorf("expected key to start with raw/, got %s", prepared[0].Key)
	}
	if prepared[0].RequestID == "" {
		t.Error("expected non-empty requestId")
	}
}

func TestIntegration_Prepare_NoAuth_Returns401(t *testing.T) {
	body, _ := json.Marshal(map[string]any{
		"files": []map[string]any{{"filename": "photo.jpg", "contentType": "image/jpeg", "size": 1024}},
	})
	req := httptest.NewRequest(http.MethodPost, "/images/prepare", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	ts.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestIntegration_Prepare_UnsupportedFileType_Returns400(t *testing.T) {
	token := registerAndLogin(t, "prepare_badtype@example.com", "Password1!")

	body, _ := json.Marshal(map[string]any{
		"files": []map[string]any{{"filename": "doc.pdf", "contentType": "application/pdf", "size": 1024}},
	})
	req := httptest.NewRequest(http.MethodPost, "/images/prepare", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	ts.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unsupported type, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestIntegration_Prepare_FileTooLarge_Returns400(t *testing.T) {
	token := registerAndLogin(t, "prepare_toolarge@example.com", "Password1!")

	body, _ := json.Marshal(map[string]any{
		"files": []map[string]any{{"filename": "big.jpg", "contentType": "image/jpeg", "size": int64(200 * 1024 * 1024)}},
	})
	req := httptest.NewRequest(http.MethodPost, "/images/prepare", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	ts.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for oversized file, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ─── Confirm Tests ────────────────────────────────────────────────────────────

func TestIntegration_PrepareAndConfirm_FileActuallyInS3(t *testing.T) {
	token := registerAndLogin(t, "confirm_s3@example.com", "Password1!")
	jpegData := makeJPEG()

	// Step 1: prepare
	prepared := prepare(t, token, []map[string]any{
		{"filename": "photo.jpg", "contentType": "image/jpeg", "size": int64(len(jpegData))},
	})

	// Step 2: PUT directly to S3 via presigned URL
	putToS3(t, prepared[0].UploadURL, "image/jpeg", jpegData)

	// Verify file is in S3
	ctx := context.Background()
	exists, err := ts.s3Client.ObjectExists(ctx, prepared[0].Key)
	if err != nil {
		t.Fatalf("S3 check failed: %v", err)
	}
	if !exists {
		t.Error("expected file to exist in S3 after PUT")
	}
}

func TestIntegration_PrepareAndConfirm_MessageLandsInSQS(t *testing.T) {
	token := registerAndLogin(t, "confirm_sqs@example.com", "Password1!")
	jpegData := makeJPEG()

	// Step 1: prepare
	prepared := prepare(t, token, []map[string]any{
		{"filename": "photo.jpg", "contentType": "image/jpeg", "size": int64(len(jpegData))},
	})

	// Step 2: PUT to S3
	putToS3(t, prepared[0].UploadURL, "image/jpeg", jpegData)

	// Step 3: confirm
	idemKey := fmt.Sprintf("idem-confirm-%d", time.Now().UnixNano())
	confirm(t, token, idemKey, prepared)

	// Verify SQS message
	ctx := context.Background()
	msgs, err := ts.sqsClient.ReceiveMessage(ctx)
	if err != nil {
		t.Fatalf("SQS receive failed: %v", err)
	}
	if len(msgs) == 0 {
		t.Error("expected SQS message after confirm, got none")
	}
}

func TestIntegration_Confirm_MissingIdemKey_Returns400(t *testing.T) {
	token := registerAndLogin(t, "confirm_noidem@example.com", "Password1!")

	body, _ := json.Marshal(map[string]any{
		"files": []map[string]any{{"key": "raw/u/req_photo.jpg", "filename": "photo.jpg", "requestId": "req-1"}},
	})
	req := httptest.NewRequest(http.MethodPost, "/images/confirm", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	// no X-Idempotency-Key
	rr := httptest.NewRecorder()
	ts.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestIntegration_Confirm_NoAuth_Returns401(t *testing.T) {
	body, _ := json.Marshal(map[string]any{
		"files": []map[string]any{{"key": "raw/u/req_photo.jpg", "filename": "photo.jpg", "requestId": "req-1"}},
	})
	req := httptest.NewRequest(http.MethodPost, "/images/confirm", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Idempotency-Key", "some-key")
	rr := httptest.NewRecorder()
	ts.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestIntegration_Confirm_BatchOfFiles_AllEnqueued(t *testing.T) {
	token := registerAndLogin(t, "confirm_batch@example.com", "Password1!")
	jpegData := makeJPEG()

	// Prepare 3 files
	fileDescs := make([]map[string]any, 3)
	for i := range fileDescs {
		fileDescs[i] = map[string]any{
			"filename":    fmt.Sprintf("photo%d.jpg", i),
			"contentType": "image/jpeg",
			"size":        int64(len(jpegData)),
		}
	}
	prepared := prepare(t, token, fileDescs)

	// PUT all 3 to S3
	for _, p := range prepared {
		putToS3(t, p.UploadURL, "image/jpeg", jpegData)
	}

	// Confirm all 3
	idemKey := fmt.Sprintf("idem-batch-%d", time.Now().UnixNano())
	body, _ := json.Marshal(map[string]any{
		"files": []map[string]any{
			{"key": prepared[0].Key, "filename": prepared[0].Filename, "requestId": prepared[0].RequestID},
			{"key": prepared[1].Key, "filename": prepared[1].Filename, "requestId": prepared[1].RequestID},
			{"key": prepared[2].Key, "filename": prepared[2].Filename, "requestId": prepared[2].RequestID},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/images/confirm", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Idempotency-Key", idemKey)
	rr := httptest.NewRecorder()
	ts.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Data struct{ Enqueued int `json:"enqueued"` } `json:"data"`
	}
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp.Data.Enqueued != 3 {
		t.Errorf("expected 3 enqueued, got %d", resp.Data.Enqueued)
	}
}

// ─── GetImages Tests ──────────────────────────────────────────────────────────

func TestIntegration_GetImages_NoAuth_Returns401(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/images", nil)
	rr := httptest.NewRecorder()
	ts.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestIntegration_GetImages_WithAuth_Returns200(t *testing.T) {
	token := registerAndLogin(t, "getimages@example.com", "Password1!")

	req := httptest.NewRequest(http.MethodGet, "/images?limit=10", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	ts.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Data struct {
			Images     []interface{} `json:"images"`
			Total      int64         `json:"total"`
			NextCursor string        `json:"nextCursor"`
			Limit      int           `json:"limit"`
		} `json:"data"`
	}
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp.Data.Limit != 10 {
		t.Errorf("unexpected pagination: limit=%d", resp.Data.Limit)
	}
}
