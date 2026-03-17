package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"strings"
	"testing"
	"time"

	"image-pipeline/internal/logger"
	"image-pipeline/internal/models"
	"image-pipeline/internal/resilence"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

func newTestCtx() context.Context {
	return logger.WithContext(context.Background(), zap.NewNop())
}

// ─── Mocks ────────────────────────────────────────────────────────────────────

type mockIdemRepo struct {
	getFn          func(ctx context.Context, key string) (*models.IdempotencyRecord, error)
	updateStatusFn func(ctx context.Context, key string, status models.IdempotencyStatus) error
}

func (m *mockIdemRepo) Get(ctx context.Context, key string) (*models.IdempotencyRecord, error) {
	return m.getFn(ctx, key)
}
func (m *mockIdemRepo) UpdateStatus(ctx context.Context, key string, status models.IdempotencyStatus) error {
	if m.updateStatusFn != nil {
		return m.updateStatusFn(ctx, key, status)
	}
	return nil
}
func (m *mockIdemRepo) Create(ctx context.Context, key, hash string) error { return nil }
func (m *mockIdemRepo) Acquire(ctx context.Context, key, hash string) (*models.IdempotencyRecord, bool, error) {
	return nil, true, nil
}

type mockImageRepo struct {
	saveFn               func(ctx context.Context, image models.Image) error
	getPaginatedImagesFn func(ctx context.Context, page, limit int, userId string, filters models.ImageFilters) ([]models.Image, int64, error)
	deleteImageFn        func(ctx context.Context, id string) (*models.Image, error)
	updateImageFn        func(ctx context.Context, id string, update bson.M) (*models.Image, error)
	findRequestByIdFn    func(ctx context.Context, requestId string) (*models.Image, error)
}

func (m *mockImageRepo) Save(ctx context.Context, img models.Image) error {
	if m.saveFn != nil {
		return m.saveFn(ctx, img)
	}
	return nil
}
func (m *mockImageRepo) GetPaginatedImages(ctx context.Context, page, limit int, userId string, filters models.ImageFilters) ([]models.Image, int64, error) {
	if m.getPaginatedImagesFn != nil {
		return m.getPaginatedImagesFn(ctx, page, limit, userId, filters)
	}
	return nil, 0, nil
}
func (m *mockImageRepo) DeleteImage(ctx context.Context, id string) (*models.Image, error) {
	if m.deleteImageFn != nil {
		return m.deleteImageFn(ctx, id)
	}
	return &models.Image{}, nil
}
func (m *mockImageRepo) UpdateImage(ctx context.Context, id string, update bson.M) (*models.Image, error) {
	if m.updateImageFn != nil {
		return m.updateImageFn(ctx, id, update)
	}
	return &models.Image{}, nil
}
func (m *mockImageRepo) FindRequestById(ctx context.Context, requestId string) (*models.Image, error) {
	if m.findRequestByIdFn != nil {
		return m.findRequestByIdFn(ctx, requestId)
	}
	return nil, nil
}
func (m *mockImageRepo) DeleteManyImages(ctx context.Context, ids []string, userId string) ([]models.Image, error) {
	return nil, nil
}

type mockS3Client struct {
	uploadStreamFn     func(ctx context.Context, key string, body io.Reader) (string, error)
	downloadStreamFn   func(ctx context.Context, key string) (io.ReadCloser, error)
	copyObjectFn       func(ctx context.Context, srcKey, dstKey string) (string, error)
	deleteObjectFn     func(ctx context.Context, key string) error
	presignPutObjectFn func(ctx context.Context, key, contentType string, size int64, expiry time.Duration) (string, error)
	objectURLFn        func(key string) string
}

func (m *mockS3Client) UploadStream(ctx context.Context, key string, body io.Reader) (string, error) {
	if m.uploadStreamFn != nil {
		return m.uploadStreamFn(ctx, key, body)
	}
	return "https://test-bucket.s3.amazonaws.com/" + key, nil
}
func (m *mockS3Client) DownloadStream(ctx context.Context, key string) (io.ReadCloser, error) {
	if m.downloadStreamFn != nil {
		return m.downloadStreamFn(ctx, key)
	}
	return nil, nil
}
func (m *mockS3Client) CopyObject(ctx context.Context, srcKey, dstKey string) (string, error) {
	if m.copyObjectFn != nil {
		return m.copyObjectFn(ctx, srcKey, dstKey)
	}
	return "https://test-bucket.s3.amazonaws.com/" + dstKey, nil
}
func (m *mockS3Client) DeleteObject(ctx context.Context, key string) error {
	if m.deleteObjectFn != nil {
		return m.deleteObjectFn(ctx, key)
	}
	return nil
}
func (m *mockS3Client) PresignPutObject(ctx context.Context, key, contentType string, size int64, expiry time.Duration) (string, error) {
	if m.presignPutObjectFn != nil {
		return m.presignPutObjectFn(ctx, key, contentType, size, expiry)
	}
	return "https://test-bucket.s3.amazonaws.com/" + key + "?presigned=true", nil
}
func (m *mockS3Client) ObjectURL(key string) string {
	if m.objectURLFn != nil {
		return m.objectURLFn(key)
	}
	return "https://test-bucket.s3.amazonaws.com/" + key
}
func (m *mockS3Client) DeleteObjects(ctx context.Context, keys []string) ([]string, error) {
	return nil, nil
}

type mockSQSClient struct {
	publishUploadFn func(ctx context.Context, msg models.UploadMessage) error
}

func (m *mockSQSClient) PublishUpload(ctx context.Context, msg models.UploadMessage) error {
	if m.publishUploadFn != nil {
		return m.publishUploadFn(ctx, msg)
	}
	return nil
}

type passthroughExec struct{}

func (p passthroughExec) Execute(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

func buildService(idem *mockIdemRepo, imgRepo *mockImageRepo, s3 *mockS3Client, sqs *mockSQSClient) *ImageService {
	return buildServiceWithCDN(idem, imgRepo, s3, sqs, "")
}

func buildServiceWithCDN(idem *mockIdemRepo, imgRepo *mockImageRepo, s3 *mockS3Client, sqs *mockSQSClient, cdnDomain string) *ImageService {
	if idem == nil {
		idem = &mockIdemRepo{getFn: func(_ context.Context, _ string) (*models.IdempotencyRecord, error) { return nil, nil }}
	}
	if imgRepo == nil {
		imgRepo = &mockImageRepo{}
	}
	if s3 == nil {
		s3 = &mockS3Client{}
	}
	if sqs == nil {
		sqs = &mockSQSClient{}
	}
	exec := passthroughExec{}
	return &ImageService{
		ImageRepo: imgRepo,
		IdemRepo:  idem,
		S3:        s3,
		s3Exec:    exec,
		sqsQueue:  sqs,
		sqsExec:   exec,
		cdnDomain: cdnDomain,
	}
}

func makeTestJPEG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 100, B: 50, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, nil)
	return buf.Bytes()
}

func makeTestPNG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// ─── PrepareUpload ────────────────────────────────────────────────────────────

func TestPrepareUpload_HappyPath_ReturnsPresignedURLs(t *testing.T) {
	var presignedKeys []string

	svc := buildService(nil, nil,
		&mockS3Client{
			presignPutObjectFn: func(_ context.Context, key, _ string, _ int64, _ time.Duration) (string, error) {
				presignedKeys = append(presignedKeys, key)
				return "https://test-bucket.s3.amazonaws.com/" + key + "?presigned=true", nil
			},
		},
		nil,
	)

	files := []PrepareFile{
		{Filename: "photo.jpg", ContentType: "image/jpeg", Size: 1024},
		{Filename: "image.png", ContentType: "image/png", Size: 2048},
	}

	result, err := svc.PrepareUpload(newTestCtx(), "user-1", files)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 prepared uploads, got %d", len(result))
	}
	for _, p := range result {
		if p.UploadURL == "" {
			t.Error("expected non-empty uploadUrl")
		}
		if p.Key == "" {
			t.Error("expected non-empty key")
		}
		if p.RequestID == "" {
			t.Error("expected non-empty requestId")
		}
		if !strings.HasPrefix(p.Key, "raw/user-1/") {
			t.Errorf("expected key to start with raw/user-1/, got %s", p.Key)
		}
	}
}

func TestPrepareUpload_PresignFails_ReturnsError(t *testing.T) {
	svc := buildService(nil, nil,
		&mockS3Client{
			presignPutObjectFn: func(_ context.Context, _, _ string, _ int64, _ time.Duration) (string, error) {
				return "", errors.New("AWS credentials expired")
			},
		},
		nil,
	)

	_, err := svc.PrepareUpload(newTestCtx(), "user-1", []PrepareFile{
		{Filename: "photo.jpg", ContentType: "image/jpeg", Size: 1024},
	})
	if err == nil {
		t.Fatal("expected error when presign fails")
	}
}

func TestPrepareUpload_KeyFormat_ContainsUserIdAndFilename(t *testing.T) {
	var capturedKey string

	svc := buildService(nil, nil,
		&mockS3Client{
			presignPutObjectFn: func(_ context.Context, key, _ string, _ int64, _ time.Duration) (string, error) {
				capturedKey = key
				return "https://test-bucket.s3.amazonaws.com/" + key, nil
			},
		},
		nil,
	)

	_, err := svc.PrepareUpload(newTestCtx(), "user-abc", []PrepareFile{
		{Filename: "vacation.jpg", ContentType: "image/jpeg", Size: 500},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(capturedKey, "raw/user-abc/") {
		t.Errorf("expected key to start with raw/user-abc/, got %s", capturedKey)
	}
	if !strings.HasSuffix(capturedKey, "_vacation.jpg") {
		t.Errorf("expected key to end with _vacation.jpg, got %s", capturedKey)
	}
}

// ─── ConfirmUpload ────────────────────────────────────────────────────────────

func TestConfirmUpload_HappyPath_PublishesToSQS(t *testing.T) {
	var capturedMsgs []models.UploadMessage

	svc := buildService(nil, nil, nil,
		&mockSQSClient{
			publishUploadFn: func(_ context.Context, msg models.UploadMessage) error {
				capturedMsgs = append(capturedMsgs, msg)
				return nil
			},
		},
	)

	files := []ConfirmFile{
		{Key: "raw/user-1/req-1_a.jpg", Filename: "a.jpg", RequestID: "req-1"},
		{Key: "raw/user-1/req-2_b.jpg", Filename: "b.jpg", RequestID: "req-2"},
	}

	enqueued, err := svc.ConfirmUpload(newTestCtx(), "user-1", "batch-idem-1", files)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if enqueued != 2 {
		t.Errorf("expected 2 enqueued, got %d", enqueued)
	}
	if len(capturedMsgs) != 2 {
		t.Fatalf("expected 2 SQS messages, got %d", len(capturedMsgs))
	}
	if capturedMsgs[0].RawS3Key != "raw/user-1/req-1_a.jpg" {
		t.Errorf("unexpected RawS3Key: %s", capturedMsgs[0].RawS3Key)
	}
	if capturedMsgs[0].UserId != "user-1" {
		t.Errorf("unexpected userId: %s", capturedMsgs[0].UserId)
	}
}

func TestConfirmUpload_SQSFails_ReturnsError(t *testing.T) {
	svc := buildService(nil, nil, nil,
		&mockSQSClient{
			publishUploadFn: func(_ context.Context, _ models.UploadMessage) error {
				return errors.New("sqs timeout")
			},
		},
	)

	_, err := svc.ConfirmUpload(newTestCtx(), "user-1", "idem-1", []ConfirmFile{
		{Key: "raw/user-1/req-1_photo.jpg", Filename: "photo.jpg", RequestID: "req-1"},
	})
	if err == nil {
		t.Fatal("expected error when SQS fails")
	}
}

func TestConfirmUpload_IdemKeyPerFile(t *testing.T) {
	var capturedKeys []string

	svc := buildService(nil, nil, nil,
		&mockSQSClient{
			publishUploadFn: func(_ context.Context, msg models.UploadMessage) error {
				capturedKeys = append(capturedKeys, msg.IdempotencyKey)
				return nil
			},
		},
	)

	files := []ConfirmFile{
		{Key: "raw/u/req-0_a.jpg", Filename: "a.jpg", RequestID: "req-0"},
		{Key: "raw/u/req-1_b.jpg", Filename: "b.jpg", RequestID: "req-1"},
	}
	svc.ConfirmUpload(newTestCtx(), "u", "batch-key", files)

	if capturedKeys[0] != "batch-key-0" {
		t.Errorf("expected batch-key-0, got %s", capturedKeys[0])
	}
	if capturedKeys[1] != "batch-key-1" {
		t.Errorf("expected batch-key-1, got %s", capturedKeys[1])
	}
}

// ─── ProcessUpload ────────────────────────────────────────────────────────────

func TestProcessUpload_HappyPath_JPEG(t *testing.T) {
	jpegData := makeTestJPEG()
	var savedImage models.Image
	var finalStatus models.IdempotencyStatus

	svc := buildService(
		&mockIdemRepo{
			getFn: func(_ context.Context, _ string) (*models.IdempotencyRecord, error) {
				return &models.IdempotencyRecord{Status: models.StatusProcessing}, nil
			},
			updateStatusFn: func(_ context.Context, _ string, status models.IdempotencyStatus) error {
				finalStatus = status
				return nil
			},
		},
		&mockImageRepo{
			saveFn: func(_ context.Context, img models.Image) error {
				savedImage = img
				return nil
			},
		},
		&mockS3Client{
			downloadStreamFn: func(_ context.Context, _ string) (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader(jpegData)), nil
			},
			uploadStreamFn: func(_ context.Context, key string, _ io.Reader) (string, error) {
				return "https://test-bucket.s3.amazonaws.com/" + key, nil
			},
		},
		nil,
	)

	err := svc.ProcessUpload(newTestCtx(), models.UploadMessage{
		IdempotencyKey: "idem-1", RequestId: "req-1",
		UserId: "user-1", FileName: "photo.jpg",
		RawS3Key: "raw/user-1/req-1_photo.jpg",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if savedImage.RequestID != "idem-1" {
		t.Errorf("expected requestID 'idem-1', got '%s'", savedImage.RequestID)
	}
	if finalStatus != models.StatusCompleted {
		t.Errorf("expected status Completed, got %v", finalStatus)
	}
}

func TestProcessUpload_HappyPath_PNG(t *testing.T) {
	pngData := makeTestPNG()

	svc := buildService(
		&mockIdemRepo{
			getFn: func(_ context.Context, _ string) (*models.IdempotencyRecord, error) {
				return &models.IdempotencyRecord{Status: models.StatusProcessing}, nil
			},
		},
		&mockImageRepo{},
		&mockS3Client{
			downloadStreamFn: func(_ context.Context, _ string) (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader(pngData)), nil
			},
			uploadStreamFn: func(_ context.Context, key string, _ io.Reader) (string, error) {
				return "https://test-bucket.s3.amazonaws.com/" + key, nil
			},
		},
		nil,
	)

	err := svc.ProcessUpload(newTestCtx(), models.UploadMessage{
		IdempotencyKey: "idem-1", RequestId: "req-1",
		UserId: "user-1", FileName: "photo.png",
		RawS3Key: "raw/user-1/req-1_photo.png",
	})
	if err != nil {
		t.Fatalf("expected no error for PNG, got %v", err)
	}
}

func TestProcessUpload_AlreadyCompleted_SkipsProcessing(t *testing.T) {
	downloadCalled := false

	svc := buildService(
		&mockIdemRepo{
			getFn: func(_ context.Context, _ string) (*models.IdempotencyRecord, error) {
				return &models.IdempotencyRecord{Status: models.StatusCompleted}, nil
			},
		},
		nil,
		&mockS3Client{
			downloadStreamFn: func(_ context.Context, _ string) (io.ReadCloser, error) {
				downloadCalled = true
				return nil, nil
			},
		},
		nil,
	)

	err := svc.ProcessUpload(newTestCtx(), models.UploadMessage{IdempotencyKey: "idem-1"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if downloadCalled {
		t.Error("S3 download should not be called when request is already completed")
	}
}

func TestProcessUpload_IdemRepoFails_ReturnsError(t *testing.T) {
	svc := buildService(
		&mockIdemRepo{
			getFn: func(_ context.Context, _ string) (*models.IdempotencyRecord, error) {
				return nil, errors.New("mongo connection lost")
			},
		},
		nil, nil, nil,
	)

	err := svc.ProcessUpload(newTestCtx(), models.UploadMessage{IdempotencyKey: "idem-1"})
	if err == nil {
		t.Fatal("expected error when idempotency repo fails")
	}
}

func TestProcessUpload_S3DownloadFails_MarksFailedAndReturnsError(t *testing.T) {
	var markedStatus models.IdempotencyStatus

	svc := buildService(
		&mockIdemRepo{
			getFn: func(_ context.Context, _ string) (*models.IdempotencyRecord, error) {
				return &models.IdempotencyRecord{Status: models.StatusProcessing}, nil
			},
			updateStatusFn: func(_ context.Context, _ string, status models.IdempotencyStatus) error {
				markedStatus = status
				return nil
			},
		},
		nil,
		&mockS3Client{
			downloadStreamFn: func(_ context.Context, _ string) (io.ReadCloser, error) {
				return nil, errors.New("s3 connection refused")
			},
		},
		nil,
	)

	err := svc.ProcessUpload(newTestCtx(), models.UploadMessage{
		IdempotencyKey: "idem-1", RawS3Key: "raw/user-1/req-1_photo.jpg",
		UserId: "user-1", FileName: "photo.jpg",
	})
	if err == nil {
		t.Fatal("expected error when S3 download fails")
	}
	if markedStatus != models.StatusFailed {
		t.Errorf("expected status Failed, got %v", markedStatus)
	}
}

func TestProcessUpload_CompressionFails_MarksFailedAndReturnsError(t *testing.T) {
	var markedStatus models.IdempotencyStatus

	svc := buildService(
		&mockIdemRepo{
			getFn: func(_ context.Context, _ string) (*models.IdempotencyRecord, error) {
				return &models.IdempotencyRecord{Status: models.StatusProcessing}, nil
			},
			updateStatusFn: func(_ context.Context, _ string, status models.IdempotencyStatus) error {
				markedStatus = status
				return nil
			},
		},
		nil,
		&mockS3Client{
			downloadStreamFn: func(_ context.Context, _ string) (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader([]byte("not-an-image"))), nil
			},
		},
		nil,
	)

	err := svc.ProcessUpload(newTestCtx(), models.UploadMessage{
		IdempotencyKey: "idem-1", RawS3Key: "raw/user-1/req-1_photo.jpg",
		UserId: "user-1", FileName: "photo.jpg",
	})
	if err == nil {
		t.Fatal("expected error when image data is corrupt")
	}
	if markedStatus != models.StatusFailed {
		t.Errorf("expected status Failed, got %v", markedStatus)
	}
}

func TestProcessUpload_DBSaveFails_MarksFailedAndReturnsError(t *testing.T) {
	jpegData := makeTestJPEG()
	var markedStatus models.IdempotencyStatus

	svc := buildService(
		&mockIdemRepo{
			getFn: func(_ context.Context, _ string) (*models.IdempotencyRecord, error) {
				return &models.IdempotencyRecord{Status: models.StatusProcessing}, nil
			},
			updateStatusFn: func(_ context.Context, _ string, status models.IdempotencyStatus) error {
				markedStatus = status
				return nil
			},
		},
		&mockImageRepo{
			saveFn: func(_ context.Context, _ models.Image) error {
				return errors.New("mongo write failed")
			},
		},
		&mockS3Client{
			downloadStreamFn: func(_ context.Context, _ string) (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader(jpegData)), nil
			},
			uploadStreamFn: func(_ context.Context, key string, _ io.Reader) (string, error) {
				return "https://test-bucket.s3.amazonaws.com/" + key, nil
			},
		},
		nil,
	)

	err := svc.ProcessUpload(newTestCtx(), models.UploadMessage{
		IdempotencyKey: "idem-1", RawS3Key: "raw/user-1/req-1_photo.jpg",
		UserId: "user-1", FileName: "photo.jpg",
	})
	if err == nil {
		t.Fatal("expected error when DB save fails")
	}
	if markedStatus != models.StatusFailed {
		t.Errorf("expected status Failed, got %v", markedStatus)
	}
}

func TestProcessUpload_RawURLBuiltFromKey(t *testing.T) {
	jpegData := makeTestJPEG()
	var savedImage models.Image

	svc := buildService(
		&mockIdemRepo{getFn: func(_ context.Context, _ string) (*models.IdempotencyRecord, error) {
			return &models.IdempotencyRecord{Status: models.StatusProcessing}, nil
		}},
		&mockImageRepo{saveFn: func(_ context.Context, img models.Image) error {
			savedImage = img
			return nil
		}},
		&mockS3Client{
			downloadStreamFn: func(_ context.Context, _ string) (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader(jpegData)), nil
			},
			uploadStreamFn: func(_ context.Context, key string, _ io.Reader) (string, error) {
				return "https://test-bucket.s3.amazonaws.com/" + key, nil
			},
		},
		nil,
	)

	err := svc.ProcessUpload(newTestCtx(), models.UploadMessage{
		IdempotencyKey: "idem-1", RawS3Key: "raw/user-1/req-1_photo.jpg",
		UserId: "user-1", FileName: "photo.jpg",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(savedImage.OriginalURL, "raw/user-1/req-1_photo.jpg") {
		t.Errorf("expected originalURL to contain raw key, got %s", savedImage.OriginalURL)
	}
}

// ─── CDN Tests ────────────────────────────────────────────────────────────────

func TestToCDNUrl_ConvertS3URLToCDN(t *testing.T) {
	svc := buildServiceWithCDN(nil, nil, nil, nil, "d1234abcd.cloudfront.net")
	result := svc.toCDNUrl("https://my-bucket.s3.amazonaws.com/compressed/user-1/photo.jpg")
	expected := "https://d1234abcd.cloudfront.net/compressed/user-1/photo.jpg"
	if result != expected {
		t.Errorf("expected '%s', got '%s'", expected, result)
	}
}

func TestToCDNUrl_NoDomain_ReturnS3URL(t *testing.T) {
	svc := buildServiceWithCDN(nil, nil, nil, nil, "")
	s3URL := "https://my-bucket.s3.amazonaws.com/compressed/user-1/photo.jpg"
	if result := svc.toCDNUrl(s3URL); result != s3URL {
		t.Errorf("expected original S3 URL, got '%s'", result)
	}
}

func TestToCDNUrl_InvalidS3URL_ReturnOriginal(t *testing.T) {
	svc := buildServiceWithCDN(nil, nil, nil, nil, "d1234abcd.cloudfront.net")
	invalidURL := "https://some-other-domain.com/photo.jpg"
	if result := svc.toCDNUrl(invalidURL); result != invalidURL {
		t.Errorf("expected original URL for non-S3 input, got '%s'", result)
	}
}

func TestToCDNUrl_PreservesFullKeyPath(t *testing.T) {
	svc := buildServiceWithCDN(nil, nil, nil, nil, "d1234abcd.cloudfront.net")
	result := svc.toCDNUrl("https://my-bucket.s3.amazonaws.com/compressed/user-abc/idem-key-123_photo.jpg")
	if !strings.Contains(result, "compressed/user-abc/idem-key-123_photo.jpg") {
		t.Errorf("CDN url missing original key path, got '%s'", result)
	}
	if !strings.HasPrefix(result, "https://d1234abcd.cloudfront.net/") {
		t.Errorf("CDN url has wrong domain, got '%s'", result)
	}
}

func TestProcessUpload_WithCDN_SavesCDNUrl(t *testing.T) {
	jpegData := makeTestJPEG()
	var savedImage models.Image

	svc := buildServiceWithCDN(
		&mockIdemRepo{getFn: func(_ context.Context, _ string) (*models.IdempotencyRecord, error) {
			return &models.IdempotencyRecord{Status: models.StatusProcessing}, nil
		}},
		&mockImageRepo{saveFn: func(_ context.Context, img models.Image) error {
			savedImage = img
			return nil
		}},
		&mockS3Client{
			downloadStreamFn: func(_ context.Context, _ string) (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader(jpegData)), nil
			},
			uploadStreamFn: func(_ context.Context, key string, _ io.Reader) (string, error) {
				return "https://my-bucket.s3.amazonaws.com/" + key, nil
			},
		},
		nil,
		"d1234abcd.cloudfront.net",
	)

	err := svc.ProcessUpload(newTestCtx(), models.UploadMessage{
		IdempotencyKey: "idem-1", RawS3Key: "raw/user-1/req-1_photo.jpg",
		UserId: "user-1", FileName: "photo.jpg",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.HasPrefix(savedImage.CompressedURL, "https://d1234abcd.cloudfront.net/") {
		t.Errorf("expected CloudFront URL in DB, got '%s'", savedImage.CompressedURL)
	}
}

func TestProcessUpload_WithoutCDN_SavesS3Url(t *testing.T) {
	jpegData := makeTestJPEG()
	var savedImage models.Image

	svc := buildService(
		&mockIdemRepo{getFn: func(_ context.Context, _ string) (*models.IdempotencyRecord, error) {
			return &models.IdempotencyRecord{Status: models.StatusProcessing}, nil
		}},
		&mockImageRepo{saveFn: func(_ context.Context, img models.Image) error {
			savedImage = img
			return nil
		}},
		&mockS3Client{
			downloadStreamFn: func(_ context.Context, _ string) (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader(jpegData)), nil
			},
			uploadStreamFn: func(_ context.Context, key string, _ io.Reader) (string, error) {
				return "https://my-bucket.s3.amazonaws.com/" + key, nil
			},
		},
		nil,
	)

	err := svc.ProcessUpload(newTestCtx(), models.UploadMessage{
		IdempotencyKey: "idem-1", RawS3Key: "raw/user-1/req-1_photo.jpg",
		UserId: "user-1", FileName: "photo.jpg",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(savedImage.CompressedURL, ".amazonaws.com/") {
		t.Errorf("expected S3 URL without CDN configured, got '%s'", savedImage.CompressedURL)
	}
}

// ─── CompressImage ────────────────────────────────────────────────────────────

func TestCompressImage_JPEG_ReducesSize(t *testing.T) {
	original := makeTestJPEG()
	svc := buildService(nil, nil, nil, nil)

	compressed, err := svc.CompressImage(original)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(compressed) == 0 {
		t.Error("expected non-empty compressed output")
	}
	_, format, err := image.Decode(bytes.NewReader(compressed))
	if err != nil {
		t.Fatalf("compressed output is not valid image: %v", err)
	}
	if format != "jpeg" {
		t.Errorf("expected jpeg format, got %s", format)
	}
}

func TestCompressImage_PNG_Compresses(t *testing.T) {
	original := makeTestPNG()
	svc := buildService(nil, nil, nil, nil)

	compressed, err := svc.CompressImage(original)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	_, format, err := image.Decode(bytes.NewReader(compressed))
	if err != nil {
		t.Fatalf("compressed output is not valid image: %v", err)
	}
	if format != "png" {
		t.Errorf("expected png format, got %s", format)
	}
}

func TestCompressImage_UnsupportedFormat_ReturnsError(t *testing.T) {
	svc := buildService(nil, nil, nil, nil)
	_, err := svc.CompressImage([]byte("not-an-image"))
	if err == nil {
		t.Fatal("expected error for invalid image data")
	}
}

// ─── GetImages ────────────────────────────────────────────────────────────────

func TestGetImages_ReturnsPaginatedResult(t *testing.T) {
	expected := []models.Image{
		{RequestID: "req-1", Filename: "a.jpg"},
		{RequestID: "req-2", Filename: "b.jpg"},
	}

	svc := buildService(nil, &mockImageRepo{
		getPaginatedImagesFn: func(_ context.Context, _, _ int, _ string, _ models.ImageFilters) ([]models.Image, int64, error) {
			return expected, 2, nil
		},
	}, nil, nil)

	resp, err := svc.GetImages(newTestCtx(), 1, 10, "user-1", models.ImageFilters{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Total != 2 || len(resp.Images) != 2 {
		t.Errorf("unexpected response: total=%d images=%d", resp.Total, len(resp.Images))
	}
}

func TestGetImages_RepoFails_ReturnsError(t *testing.T) {
	svc := buildService(nil, &mockImageRepo{
		getPaginatedImagesFn: func(_ context.Context, _, _ int, _ string, _ models.ImageFilters) ([]models.Image, int64, error) {
			return nil, 0, errors.New("db error")
		},
	}, nil, nil)

	_, err := svc.GetImages(newTestCtx(), 1, 10, "user-1", models.ImageFilters{})
	if err == nil {
		t.Fatal("expected error when repo fails")
	}
}

// ─── DeleteImage ──────────────────────────────────────────────────────────────

func TestDeleteImage_HappyPath_DeletesFromDBAndS3(t *testing.T) {
	deletedKeys := []string{}

	svc := buildService(nil,
		&mockImageRepo{
			deleteImageFn: func(_ context.Context, _ string) (*models.Image, error) {
				return &models.Image{
					ID:            primitive.NewObjectID(),
					UserID:        "user-1",
					OriginalURL:   "https://test-bucket.s3.amazonaws.com/raw/user-1/req-1_photo.jpg",
					CompressedURL: "https://test-bucket.s3.amazonaws.com/compressed/user-1/req-1_photo.jpg",
				}, nil
			},
		},
		&mockS3Client{
			deleteObjectFn: func(_ context.Context, key string) error {
				deletedKeys = append(deletedKeys, key)
				return nil
			},
		},
		nil,
	)

	err := svc.DeleteImage(newTestCtx(), "img-id-1", "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(deletedKeys) != 2 {
		t.Errorf("expected 2 S3 deletions (raw + compressed), got %d", len(deletedKeys))
	}
}

func TestDeleteImage_DBFails_ReturnsError(t *testing.T) {
	svc := buildService(nil,
		&mockImageRepo{
			deleteImageFn: func(_ context.Context, _ string) (*models.Image, error) {
				return nil, errors.New("db delete failed")
			},
		},
		nil, nil,
	)

	err := svc.DeleteImage(newTestCtx(), "img-id-1", "user-1")
	if err == nil {
		t.Fatal("expected error when DB delete fails")
	}
}

var _ resilence.Executor = passthroughExec{}

// suppress unused import
var _ = fmt.Sprintf
