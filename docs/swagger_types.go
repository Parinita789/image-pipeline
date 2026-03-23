package docs

import "image-pipeline/internal/models"

// ─── Request Bodies ──────────────────────────────────────────────────────────

// RegisterRequest represents the registration payload.
type RegisterRequest struct {
	Email     string `json:"email" example:"user@example.com"`
	FirstName string `json:"firstName" example:"Jane"`
	LastName  string `json:"lastName" example:"Doe"`
	Password  string `json:"password" example:"MyStr0ng!Pass"`
}

// LoginRequest represents the login payload.
type LoginRequest struct {
	Email    string `json:"email" example:"user@example.com"`
	Password string `json:"password" example:"MyStr0ng!Pass"`
}

// ForgotPasswordRequest represents the forgot-password payload.
type ForgotPasswordRequest struct {
	Email string `json:"email" example:"user@example.com"`
}

// ResetPasswordRequest represents the reset-password payload.
type ResetPasswordRequest struct {
	Token       string `json:"token" example:"a1b2c3d4e5f6..."`
	NewPassword string `json:"newPassword" example:"NewStr0ng!Pass"`
}

// PrepareFileRequest is one entry in the prepare-upload payload.
type PrepareFileRequest struct {
	Filename    string `json:"filename" example:"photo.jpg"`
	ContentType string `json:"contentType" example:"image/jpeg"`
	Size        int64  `json:"size" example:"2097152"`
}

// PrepareUploadRequest represents the prepare-upload payload.
type PrepareUploadRequest struct {
	Files []PrepareFileRequest `json:"files"`
}

// ConfirmFileRequest is one entry in the confirm-upload payload.
type ConfirmFileRequest struct {
	Key       string `json:"key" example:"raw/user123/uuid_photo.jpg"`
	Filename  string `json:"filename" example:"photo.jpg"`
	RequestID string `json:"requestId" example:"550e8400-e29b-41d4-a716-446655440000"`
}

// ConfirmUploadRequest represents the confirm-upload payload.
type ConfirmUploadRequest struct {
	Files []ConfirmFileRequest `json:"files"`
}

// TransformRequest represents the transform payload.
type TransformRequest struct {
	Transformations []models.TransformConfig `json:"transformations"`
}

// BatchTransformRequest represents the batch transform payload.
type BatchTransformRequest struct {
	Ids             []string                 `json:"ids" example:"id1,id2"`
	Transformations []models.TransformConfig `json:"transformations"`
}

// BatchIDsRequest is used for batch revert and batch delete.
type BatchIDsRequest struct {
	Ids []string `json:"ids" example:"id1,id2"`
}

// ─── Response Bodies ─────────────────────────────────────────────────────────

// APIResponse is the standard envelope for all API responses.
type APIResponse struct {
	Status  string      `json:"status" example:"success"`
	Code    int         `json:"code" example:"200"`
	Message string      `json:"message,omitempty" example:"operation completed"`
	Data    interface{} `json:"data,omitempty"`
}

// LoginData is the data field returned on login.
type LoginData struct {
	Token string `json:"token" example:"eyJhbGciOiJIUzI1NiIs..."`
}

// ForgotPasswordData is the data field for forgot-password.
type ForgotPasswordData struct {
	Message    string `json:"message" example:"if the email exists, a reset link has been sent"`
	ResetToken string `json:"resetToken,omitempty" example:"a1b2c3d4e5f6..."`
}

// PreparedFile represents a single file ready for upload.
type PreparedFile struct {
	Key       string `json:"key" example:"raw/user123/uuid_photo.jpg"`
	UploadURL string `json:"uploadUrl" example:"https://s3.amazonaws.com/..."`
	Filename  string `json:"filename" example:"photo.jpg"`
	RequestID string `json:"requestId" example:"550e8400-e29b-41d4-a716-446655440000"`
}

// ConfirmData is the data field for confirm-upload.
type ConfirmData struct {
	Enqueued int `json:"enqueued" example:"3"`
	Total    int `json:"total" example:"3"`
}

// PaginatedImages is the data field for the image list.
type PaginatedImages struct {
	Page   int            `json:"page" example:"1"`
	Limit  int            `json:"limit" example:"10"`
	Total  int64          `json:"total" example:"42"`
	Images []models.Image `json:"images"`
}

// StorageInfo is the data field for GET /storage.
type StorageInfo struct {
	UsedBytes   int64   `json:"usedBytes" example:"524288000"`
	LimitBytes  int64   `json:"limitBytes" example:"1073741824"`
	UsedPercent float64 `json:"usedPercent" example:"48.8"`
}

// BatchSyncResult is returned when a small batch is processed in-handler.
type BatchSyncResult struct {
	Succeeded []BatchResultItem `json:"succeeded"`
	Failed    []BatchResultItem `json:"failed"`
}

// BatchResultItem is one item in a sync batch result.
type BatchResultItem struct {
	ID    string `json:"id" example:"60d5ec49f1b2c72d88c1e1a1"`
	Error string `json:"error,omitempty" example:"image not found"`
}

// BatchAsyncResult is returned when a large batch is enqueued to SQS.
type BatchAsyncResult struct {
	BatchID string `json:"batchId" example:"550e8400-e29b-41d4-a716-446655440000"`
	Total   int    `json:"total" example:"25"`
}

// DeleteResult is the data field for batch delete.
type DeleteResult struct {
	Deleted []string          `json:"deleted"`
	Failed  []BatchResultItem `json:"failed"`
}
