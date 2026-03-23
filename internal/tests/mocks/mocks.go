package mocks

import (
	"context"
	"image-pipeline/internal/models"
	"io"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

// ─── IdempotencyRepo Mock ────────────────────────────────────────────────────

type MockIdemRepo struct {
	GetFn          func(ctx context.Context, key string) (*models.IdempotencyRecord, error)
	CreateFn       func(ctx context.Context, key, hash string) error
	UpdateStatusFn func(ctx context.Context, key string, status models.IdempotencyStatus) error
	AcquireFn      func(ctx context.Context, key, hash string) (*models.IdempotencyRecord, bool, error)
}

func (m *MockIdemRepo) Get(ctx context.Context, key string) (*models.IdempotencyRecord, error) {
	return m.GetFn(ctx, key)
}
func (m *MockIdemRepo) Create(ctx context.Context, key, hash string) error {
	return m.CreateFn(ctx, key, hash)
}
func (m *MockIdemRepo) UpdateStatus(ctx context.Context, key string, status models.IdempotencyStatus) error {
	return m.UpdateStatusFn(ctx, key, status)
}
func (m *MockIdemRepo) Acquire(ctx context.Context, key, hash string) (*models.IdempotencyRecord, bool, error) {
	return m.AcquireFn(ctx, key, hash)
}

// ─── ImageRepo Mock ──────────────────────────────────────────────────────────

type MockImageRepo struct {
	SaveFn                    func(ctx context.Context, image models.Image) error
	FindByIdFn                func(ctx context.Context, id string) (*models.Image, error)
	FindRequestByIdFn         func(ctx context.Context, requestId string) (*models.Image, error)
	GetPaginatedImagesFn      func(ctx context.Context, page, limit int, userId string, filters models.ImageFilters) ([]models.Image, int64, error)
	DeleteImageFn             func(ctx context.Context, id string) (*models.Image, error)
	DeleteManyImagesFn        func(ctx context.Context, ids []string, userId string) ([]models.Image, error)
	UpdateImageFn             func(ctx context.Context, id string, update bson.M) (*models.Image, error)
	SumStorageByUserFn        func(ctx context.Context, userId string) (int64, error)
	CreateProcessingRecordFn  func(ctx context.Context, requestId, userId, filename, rawS3Key string) error
	ExpireStuckProcessingFn   func(ctx context.Context, userId string, timeout time.Duration)
}

func (m *MockImageRepo) Save(ctx context.Context, image models.Image) error {
	return m.SaveFn(ctx, image)
}
func (m *MockImageRepo) FindById(ctx context.Context, id string) (*models.Image, error) {
	if m.FindByIdFn != nil {
		return m.FindByIdFn(ctx, id)
	}
	return nil, nil
}
func (m *MockImageRepo) FindRequestById(ctx context.Context, requestId string) (*models.Image, error) {
	return m.FindRequestByIdFn(ctx, requestId)
}
func (m *MockImageRepo) GetPaginatedImages(ctx context.Context, page, limit int, userId string, filters models.ImageFilters) ([]models.Image, int64, error) {
	return m.GetPaginatedImagesFn(ctx, page, limit, userId, filters)
}
func (m *MockImageRepo) DeleteImage(ctx context.Context, id string) (*models.Image, error) {
	return m.DeleteImageFn(ctx, id)
}
func (m *MockImageRepo) DeleteManyImages(ctx context.Context, ids []string, userId string) ([]models.Image, error) {
	if m.DeleteManyImagesFn != nil {
		return m.DeleteManyImagesFn(ctx, ids, userId)
	}
	return nil, nil
}
func (m *MockImageRepo) UpdateImage(ctx context.Context, id string, update bson.M) (*models.Image, error) {
	return m.UpdateImageFn(ctx, id, update)
}
func (m *MockImageRepo) SumStorageByUser(ctx context.Context, userId string) (int64, error) {
	if m.SumStorageByUserFn != nil {
		return m.SumStorageByUserFn(ctx, userId)
	}
	return 0, nil
}
func (m *MockImageRepo) CreateProcessingRecord(ctx context.Context, requestId, userId, filename, rawS3Key string) error {
	if m.CreateProcessingRecordFn != nil {
		return m.CreateProcessingRecordFn(ctx, requestId, userId, filename, rawS3Key)
	}
	return nil
}
func (m *MockImageRepo) UpdateImageByRequestId(ctx context.Context, requestId string, fields bson.M) error {
	return nil
}
func (m *MockImageRepo) ExpireStuckProcessing(ctx context.Context, userId string, timeout time.Duration) {
	if m.ExpireStuckProcessingFn != nil {
		m.ExpireStuckProcessingFn(ctx, userId, timeout)
	}
}

// ─── UserRepo Mock ───────────────────────────────────────────────────────────

type MockUserRepo struct {
	CreateUserFn     func(ctx context.Context, user *models.User) (string, error)
	GetUserByEmailFn func(ctx context.Context, email string) (*models.User, error)
}

func (m *MockUserRepo) CreateUser(ctx context.Context, user *models.User) (string, error) {
	return m.CreateUserFn(ctx, user)
}
func (m *MockUserRepo) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	return m.GetUserByEmailFn(ctx, email)
}

// ─── S3Client Mock ───────────────────────────────────────────────────────────

type MockS3Client struct {
	UploadStreamFn   func(ctx context.Context, key string, body io.Reader) (string, error)
	DownloadStreamFn func(ctx context.Context, key string) (io.ReadCloser, error)
	CopyObjectFn     func(ctx context.Context, srcKey, dstKey string) (string, error)
	DeleteObjectFn   func(ctx context.Context, key string) error
}

func (m *MockS3Client) UploadStream(ctx context.Context, key string, body io.Reader) (string, error) {
	return m.UploadStreamFn(ctx, key, body)
}
func (m *MockS3Client) DownloadStream(ctx context.Context, key string) (io.ReadCloser, error) {
	return m.DownloadStreamFn(ctx, key)
}
func (m *MockS3Client) CopyObject(ctx context.Context, srcKey, dstKey string) (string, error) {
	return m.CopyObjectFn(ctx, srcKey, dstKey)
}
func (m *MockS3Client) DeleteObject(ctx context.Context, key string) error {
	return m.DeleteObjectFn(ctx, key)
}

// ─── SQSClient Mock ──────────────────────────────────────────────────────────

type MockSQSClient struct {
	PublishUploadFn func(ctx context.Context, msg models.UploadMessage) error
}

func (m *MockSQSClient) PublishUpload(ctx context.Context, msg models.UploadMessage) error {
	return m.PublishUploadFn(ctx, msg)
}
