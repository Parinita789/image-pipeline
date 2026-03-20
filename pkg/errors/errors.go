package errors

import "net/http"

var (
	ErrInvalidJSON    = New(http.StatusBadRequest, "INVALID_JSON", "invalid request body")
	ErrInternalServer = New(http.StatusInternalServerError, "INTERNAL_ERROR", "something went wrong")
)

// Auth
var (
	ErrMissingToken       = New(http.StatusUnauthorized, "MISSING_TOKEN", "missing authorization token")
	ErrInvalidToken       = New(http.StatusUnauthorized, "INVALID_TOKEN", "invalid or expired token")
	ErrInvalidClaims      = New(http.StatusUnauthorized, "INVALID_CLAIMS", "invalid token claims")
	ErrMissingUserID      = New(http.StatusUnauthorized, "MISSING_USER_ID", "userId missing in token")
	ErrInvalidCredentials = New(http.StatusUnauthorized, "INVALID_CREDENTIALS", "invalid email or password")
	ErrEmailExists        = New(http.StatusConflict, "EMAIL_EXISTS", "email already exists")
	ErrRegistrationFailed = New(http.StatusInternalServerError, "REGISTRATION_FAILED", "registration failed")
)

// Images

var (
	ErrNoFilesProvided   = New(http.StatusBadRequest, "NO_FILES", "no files provided")
	ErrTooManyFiles      = New(http.StatusBadRequest, "TOO_MANY_FILES", "too many files — max %d allowed")
	ErrFileTooLarge      = New(http.StatusBadRequest, "FILE_TOO_LARGE", "%s exceeds max file size (100MB)")
	ErrUnsupportedType   = New(http.StatusBadRequest, "UNSUPPORTED_TYPE", "%s: unsupported type — allowed: jpeg, png, webp")
	ErrPrepareFailed     = New(http.StatusInternalServerError, "PREPARE_FAILED", "failed to prepare upload")
	ErrNoFilesToConfirm  = New(http.StatusBadRequest, "NO_FILES_CONFIRM", "no files to confirm")
	ErrEnqueueFailed     = New(http.StatusInternalServerError, "ENQUEUE_FAILED", "failed to enqueue uploads")
	ErrImageNotFound     = New(http.StatusNotFound, "IMAGE_NOT_FOUND", "image not found")
	ErrImageForbidden    = New(http.StatusForbidden, "IMAGE_FORBIDDEN", "you do not own this image")
	ErrImageFetchFailed  = New(http.StatusInternalServerError, "IMAGE_FETCH_FAILED", "failed to get images")
	ErrImageDeleteFailed = New(http.StatusInternalServerError, "IMAGE_DELETE_FAILED", "failed to delete image")
	ErrBatchDeleteFailed = New(http.StatusInternalServerError, "BATCH_DELETE_FAILED", "batch delete failed")
	ErrMissingRequestID  = New(http.StatusBadRequest, "MISSING_REQUEST_ID", "missing requestId")
)

// Upload

var (
	ErrMissingIdemKey    = New(http.StatusBadRequest, "MISSING_IDEM_KEY", "missing X-Idempotency-Key")
	ErrIdemKeyConflict   = New(http.StatusConflict, "IDEM_KEY_CONFLICT", "idempotency key reused with different request")
	ErrRequestProcessing = New(http.StatusConflict, "REQUEST_PROCESSING", "request is still processing, please wait")
)

// Rate Limiting

var (
	ErrTooManyRequests = New(http.StatusTooManyRequests, "RATE_LIMITED", "too many requests")
)

// Batch Delete

var (
	ErrNoIDsProvided = New(http.StatusBadRequest, "NO_IDS", "no ids provided")
	ErrTooManyIDs    = New(http.StatusBadRequest, "TOO_MANY_IDS", "too many ids — max 50 per request")
)

// Storage

var (
	ErrStorageQuotaExceeded = New(http.StatusRequestEntityTooLarge, "STORAGE_QUOTA_EXCEEDED", "storage quota exceeded")
)

// Transforms

var (
	ErrInvalidTransform = New(http.StatusBadRequest, "INVALID_TRANSFORM", "invalid transformation: %s")
)
