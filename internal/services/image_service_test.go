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
	"testing"

	"image-pipeline/internal/logger"
	"image-pipeline/internal/models"
	"image-pipeline/internal/resilence"

	"go.mongodb.org/mongo-driver/bson"
	"go.uber.org/zap"
)

func testCtx() context.Context {
	ctx := context.Background()
	return logger.WithContext(ctx, zap.NewNop())
}

// Inline mocks keep this file self-contained — no import cycle risk.
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
	getPaginatedImagesFn func(ctx context.Context, page, limit int, userId string) ([]models.Image, int64, error)
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
func (m *mockImageRepo) GetPaginatedImages(ctx context.Context, page, limit int, userId string) ([]models.Image, int64, error) {
	if m.getPaginatedImagesFn != nil {
		return m.getPaginatedImagesFn(ctx, page, limit, userId)
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

type mockS3Client struct {
	uploadStreamFn   func(ctx context.Context, key string, body io.Reader) (string, error)
	downloadStreamFn func(ctx context.Context, key string) (io.ReadCloser, error)
	copyObjectFn     func(ctx context.Context, srcKey, dstKey string) (string, error)
	deleteObjectFn   func(ctx context.Context, key string) error
}

func (m *mockS3Client) UploadStream(ctx context.Context, key string, body io.Reader) (string, error) {
	if m.uploadStreamFn != nil {
		return m.uploadStreamFn(ctx, key, body)
	}
	return "https://s3.amazonaws.com/" + key, nil
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
	return "https://s3.amazonaws.com/" + dstKey, nil
}
func (m *mockS3Client) DeleteObject(ctx context.Context, key string) error {
	if m.deleteObjectFn != nil {
		return m.deleteObjectFn(ctx, key)
	}
	return nil
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

// passthrough executor — executes the function directly with no retry logic
type passthroughExec struct{}

func (p passthroughExec) Execute(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

// Any nil mock falls back to a sensible no-op default.
func buildService(
	idem *mockIdemRepo,
	imgRepo *mockImageRepo,
	s3 *mockS3Client,
	sqs *mockSQSClient,
) *ImageService {
	if idem == nil {
		idem = &mockIdemRepo{getFn: func(_ context.Context, _ string) (*models.IdempotencyRecord, error) {
			return nil, nil
		}}
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
	_ = zap.NewNop()

	return &ImageService{
		ImageRepo: imgRepo,
		IdemRepo:  idem,
		S3:        s3,
		s3Exec:    exec,
		sqsQueue:  sqs,
		sqsExec:   exec,
	}
}

// makeTestJPEG returns a minimal valid JPEG as []byte.
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

// makeTestPNG returns a minimal valid PNG as []byte.
func makeTestPNG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func TestEnqueueUpload_HappyPath(t *testing.T) {
	var capturedKey string
	var capturedMsg models.UploadMessage

	svc := buildService(nil, nil,
		&mockS3Client{
			uploadStreamFn: func(_ context.Context, key string, _ io.Reader) (string, error) {
				capturedKey = key
				return "https://s3.amazonaws.com/" + key, nil
			},
		},
		&mockSQSClient{
			publishUploadFn: func(_ context.Context, msg models.UploadMessage) error {
				capturedMsg = msg
				return nil
			},
		},
	)

	ctx := testCtx()
	err := svc.EnqueueUpload(ctx, "req-1", "user-1", "idem-1", "photo.jpg", bytes.NewReader([]byte("data")))

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if capturedKey == "" {
		t.Error("expected S3 upload to be called")
	}
	if capturedMsg.IdempotencyKey != "idem-1" {
		t.Errorf("expected idem key 'idem-1', got '%s'", capturedMsg.IdempotencyKey)
	}
	if capturedMsg.FileName != "photo.jpg" {
		t.Errorf("expected filename 'photo.jpg', got '%s'", capturedMsg.FileName)
	}
}

func TestEnqueueUpload_S3Fails_ReturnsError(t *testing.T) {
	s3Err := errors.New("s3 connection refused")

	svc := buildService(nil, nil,
		&mockS3Client{
			uploadStreamFn: func(_ context.Context, _ string, _ io.Reader) (string, error) {
				return "", s3Err
			},
		},
		// SQS should never be called if S3 fails
		&mockSQSClient{
			publishUploadFn: func(_ context.Context, _ models.UploadMessage) error {
				t.Error("SQS should not be called when S3 upload fails")
				return nil
			},
		},
	)

	err := svc.EnqueueUpload(testCtx(), "req-1", "user-1", "idem-1", "photo.jpg", bytes.NewReader([]byte("data")))

	if err == nil {
		t.Fatal("expected error when S3 fails, got nil")
	}
}

func TestEnqueueUpload_SQSFails_ReturnsError(t *testing.T) {
	sqsErr := errors.New("sqs timeout")

	svc := buildService(nil, nil,
		&mockS3Client{},
		&mockSQSClient{
			publishUploadFn: func(_ context.Context, _ models.UploadMessage) error {
				return sqsErr
			},
		},
	)

	err := svc.EnqueueUpload(testCtx(), "req-1", "user-1", "idem-1", "photo.jpg", bytes.NewReader([]byte("data")))

	if err == nil {
		t.Fatal("expected error when SQS fails, got nil")
	}
}

func TestEnqueueUpload_TempKeyFormat(t *testing.T) {
	var uploadedKey string

	svc := buildService(nil, nil,
		&mockS3Client{
			uploadStreamFn: func(_ context.Context, key string, _ io.Reader) (string, error) {
				uploadedKey = key
				return "https://s3.amazonaws.com/" + key, nil
			},
		},
		nil,
	)

	err := svc.EnqueueUpload(testCtx(), "req-abc", "user-xyz", "idem-1", "photo.jpg", bytes.NewReader([]byte("data")))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := fmt.Sprintf("tmp/%s/%s/%s", "user-xyz", "req-abc", "photo.jpg")
	if uploadedKey != expected {
		t.Errorf("expected key format '%s', got '%s'", expected, uploadedKey)
	}
}

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
			copyObjectFn: func(_ context.Context, _, dst string) (string, error) {
				return "https://s3.amazonaws.com/" + dst, nil
			},
			downloadStreamFn: func(_ context.Context, _ string) (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader(jpegData)), nil
			},
			uploadStreamFn: func(_ context.Context, key string, _ io.Reader) (string, error) {
				return "https://s3.amazonaws.com/" + key, nil
			},
			deleteObjectFn: func(_ context.Context, _ string) error {
				return nil
			},
		},
		nil,
	)

	msg := models.UploadMessage{
		IdempotencyKey: "idem-1",
		RequestId:      "req-1",
		UserId:         "user-1",
		FileName:       "photo.jpg",
		TempS3Key:      "tmp/user-1/req-1_photo.jpg",
	}

	err := svc.ProcessUpload(testCtx(), msg)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if savedImage.RequestID != "idem-1" {
		t.Errorf("expected saved requestID 'idem-1', got '%s'", savedImage.RequestID)
	}
	if finalStatus != models.StatusCompleted {
		t.Errorf("expected final status Completed, got %v", finalStatus)
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
			copyObjectFn: func(_ context.Context, _, dst string) (string, error) {
				return "https://s3.amazonaws.com/" + dst, nil
			},
			downloadStreamFn: func(_ context.Context, _ string) (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader(pngData)), nil
			},
			uploadStreamFn: func(_ context.Context, key string, _ io.Reader) (string, error) {
				return "https://s3.amazonaws.com/" + key, nil
			},
		},
		nil,
	)

	err := svc.ProcessUpload(testCtx(), models.UploadMessage{
		IdempotencyKey: "idem-1",
		RequestId:      "req-1",
		UserId:         "user-1",
		FileName:       "photo.png",
		TempS3Key:      "tmp/user-1/req-1_photo.png",
	})

	if err != nil {
		t.Fatalf("expected no error for PNG, got %v", err)
	}
}

func TestProcessUpload_AlreadyCompleted_SkipsProcessing(t *testing.T) {
	s3Called := false

	svc := buildService(
		&mockIdemRepo{
			getFn: func(_ context.Context, _ string) (*models.IdempotencyRecord, error) {
				return &models.IdempotencyRecord{Status: models.StatusCompleted}, nil
			},
		},
		nil,
		&mockS3Client{
			copyObjectFn: func(_ context.Context, _, _ string) (string, error) {
				s3Called = true
				return "", nil
			},
		},
		nil,
	)

	err := svc.ProcessUpload(testCtx(), models.UploadMessage{
		IdempotencyKey: "idem-1",
		RequestId:      "req-1",
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if s3Called {
		t.Error("S3 should not be called when request is already completed")
	}
}

func TestProcessUpload_IdemRepoFails_ReturnsError(t *testing.T) {
	dbErr := errors.New("mongo connection lost")

	svc := buildService(
		&mockIdemRepo{
			getFn: func(_ context.Context, _ string) (*models.IdempotencyRecord, error) {
				return nil, dbErr
			},
		},
		nil, nil, nil,
	)

	err := svc.ProcessUpload(testCtx(), models.UploadMessage{IdempotencyKey: "idem-1"})

	if err == nil {
		t.Fatal("expected error when idempotency repo fails")
	}
}

func TestProcessUpload_S3CopyFails_MarksFailedAndReturnsError(t *testing.T) {
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
			copyObjectFn: func(_ context.Context, _, _ string) (string, error) {
				return "", errors.New("s3 copy failed")
			},
		},
		nil,
	)

	err := svc.ProcessUpload(testCtx(), models.UploadMessage{
		IdempotencyKey: "idem-1",
		RequestId:      "req-1",
		UserId:         "user-1",
		FileName:       "photo.jpg",
		TempS3Key:      "tmp/user-1/req-1_photo.jpg",
	})

	if err == nil {
		t.Fatal("expected error when S3 copy fails")
	}
	if markedStatus != models.StatusFailed {
		t.Errorf("expected status to be marked Failed, got %v", markedStatus)
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
			copyObjectFn: func(_ context.Context, _, dst string) (string, error) {
				return "https://s3.amazonaws.com/" + dst, nil
			},
			downloadStreamFn: func(_ context.Context, _ string) (io.ReadCloser, error) {
				// return corrupt image data — will fail image.Decode
				return io.NopCloser(bytes.NewReader([]byte("not-an-image"))), nil
			},
		},
		nil,
	)

	err := svc.ProcessUpload(testCtx(), models.UploadMessage{
		IdempotencyKey: "idem-1",
		RequestId:      "req-1",
		UserId:         "user-1",
		FileName:       "photo.jpg",
		TempS3Key:      "tmp/user-1/req-1_photo.jpg",
	})

	if err == nil {
		t.Fatal("expected error when image data is corrupt")
	}
	if markedStatus != models.StatusFailed {
		t.Errorf("expected status to be marked Failed, got %v", markedStatus)
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
			copyObjectFn: func(_ context.Context, _, dst string) (string, error) {
				return "https://s3.amazonaws.com/" + dst, nil
			},
			downloadStreamFn: func(_ context.Context, _ string) (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader(jpegData)), nil
			},
			uploadStreamFn: func(_ context.Context, key string, _ io.Reader) (string, error) {
				return "https://s3.amazonaws.com/" + key, nil
			},
		},
		nil,
	)

	err := svc.ProcessUpload(testCtx(), models.UploadMessage{
		IdempotencyKey: "idem-1",
		RequestId:      "req-1",
		UserId:         "user-1",
		FileName:       "photo.jpg",
		TempS3Key:      "tmp/user-1/req-1_photo.jpg",
	})

	if err == nil {
		t.Fatal("expected error when DB save fails")
	}
	if markedStatus != models.StatusFailed {
		t.Errorf("expected status Failed, got %v", markedStatus)
	}
}

func TestProcessUpload_TempFileDeletion_DoesNotFailOnError(t *testing.T) {
	// temp file deletion errors should be logged but not propagate —
	// the job is already complete at that point
	jpegData := makeTestJPEG()

	svc := buildService(
		&mockIdemRepo{
			getFn: func(_ context.Context, _ string) (*models.IdempotencyRecord, error) {
				return &models.IdempotencyRecord{Status: models.StatusProcessing}, nil
			},
		},
		&mockImageRepo{},
		&mockS3Client{
			copyObjectFn: func(_ context.Context, _, dst string) (string, error) {
				return "https://s3.amazonaws.com/" + dst, nil
			},
			downloadStreamFn: func(_ context.Context, _ string) (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader(jpegData)), nil
			},
			uploadStreamFn: func(_ context.Context, key string, _ io.Reader) (string, error) {
				return "https://s3.amazonaws.com/" + key, nil
			},
			deleteObjectFn: func(_ context.Context, _ string) error {
				return errors.New("s3 delete failed") // should not surface
			},
		},
		nil,
	)

	err := svc.ProcessUpload(testCtx(), models.UploadMessage{
		IdempotencyKey: "idem-1",
		RequestId:      "req-1",
		UserId:         "user-1",
		FileName:       "photo.jpg",
		TempS3Key:      "tmp/user-1/req-1_photo.jpg",
	})

	if err != nil {
		t.Fatalf("temp file delete error should not fail the job, got %v", err)
	}
}

// ─── CompressImage Tests ─────────────────────────────────────────────────────

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
	// verify the output is still a valid JPEG
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
		t.Fatal("expected error for unsupported/invalid image data")
	}
}

func TestGetImages_ReturnsPaginatedResult(t *testing.T) {
	expected := []models.Image{
		{RequestID: "req-1", Filename: "a.jpg"},
		{RequestID: "req-2", Filename: "b.jpg"},
	}

	svc := buildService(nil, &mockImageRepo{
		getPaginatedImagesFn: func(_ context.Context, page, limit int, userId string) ([]models.Image, int64, error) {
			return expected, 2, nil
		},
	}, nil, nil)

	resp, err := svc.GetImages(testCtx(), 1, 10, "user-1")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Total != 2 {
		t.Errorf("expected total 2, got %d", resp.Total)
	}
	if len(resp.Images) != 2 {
		t.Errorf("expected 2 images, got %d", len(resp.Images))
	}
	if resp.Page != 1 || resp.Limit != 10 {
		t.Errorf("unexpected pagination: page=%d limit=%d", resp.Page, resp.Limit)
	}
}

func TestGetImages_RepoFails_ReturnsError(t *testing.T) {
	svc := buildService(nil, &mockImageRepo{
		getPaginatedImagesFn: func(_ context.Context, _, _ int, _ string) ([]models.Image, int64, error) {
			return nil, 0, errors.New("db error")
		},
	}, nil, nil)

	_, err := svc.GetImages(testCtx(), 1, 10, "user-1")
	if err == nil {
		t.Fatal("expected error when repo fails")
	}
}

func TestDeleteImage_HappyPath_DeletesFromDBAndS3(t *testing.T) {
	deletedKeys := []string{}

	svc := buildService(nil,
		&mockImageRepo{
			deleteImageFn: func(_ context.Context, _ string) (*models.Image, error) {
				return &models.Image{
					OriginalURL:   "raw/user-1/req-1_photo.jpg",
					CompressedURL: "compressed/user-1/req-1_photo.jpg",
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

	err := svc.DeleteImage(testCtx(), "img-id-1")

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

	err := svc.DeleteImage(testCtx(), "img-id-1")
	if err == nil {
		t.Fatal("expected error when DB delete fails")
	}
}

var _ resilence.Executor = passthroughExec{}
