package handlers

import (
	"encoding/json"
	"fmt"
	"image-pipeline/internal/logger"
	"image-pipeline/internal/middleware"
	"image-pipeline/internal/models"
	"image-pipeline/internal/services"
	apperr "image-pipeline/pkg/errors"
	"image-pipeline/pkg/response"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

const (
	maxFiles     = 30
	maxFileSize  = 100 * 1024 * 1024 // 100MB per file
	maxTotalSize = 500 * 1024 * 1024 // 500MB total per batch
)

type ImageHandler struct {
	Service *services.ImageService
}

func NewImageHandler(service *services.ImageService) *ImageHandler {
	return &ImageHandler{Service: service}
}

func (h *ImageHandler) PrepareUpload(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.FromContext(ctx)
	userId := middleware.GetUserID(r)

	var req struct {
		Files []struct {
			Filename    string `json:"filename"`
			ContentType string `json:"contentType"`
			Size        int64  `json:"size"`
		} `json:"files"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.AppError(w, apperr.ErrInvalidJSON)
		return
	}

	if len(req.Files) == 0 {
		response.AppError(w, apperr.ErrNoFilesProvided)
		return
	}
	if len(req.Files) > maxFiles {
		response.AppError(w, apperr.ErrTooManyFiles.Withf(maxFiles))
		return
	}

	for _, f := range req.Files {
		if f.Size > maxFileSize {
			response.AppError(w, apperr.ErrFileTooLarge.Withf(f.Filename))
			return
		}
		if !isAllowedType(f.ContentType) {
			response.AppError(w, apperr.ErrUnsupportedType.Withf(f.Filename))
			return
		}
	}

	// Check storage quota
	var totalRequestSize int64
	for _, f := range req.Files {
		totalRequestSize += f.Size
	}
	if err := h.Service.CheckStorageQuota(ctx, userId, totalRequestSize); err != nil {
		response.HandleError(w, err)
		return
	}

	prepareFiles := make([]services.PrepareFile, len(req.Files))
	for i, f := range req.Files {
		prepareFiles[i] = services.PrepareFile{
			Filename:    f.Filename,
			ContentType: f.ContentType,
			Size:        f.Size,
		}
	}

	log.Info("prepare upload received", zap.String("userId", userId), zap.Int("fileCount", len(prepareFiles)))

	prepared, err := h.Service.PrepareUpload(ctx, userId, prepareFiles)
	if err != nil {
		log.Error("failed to prepare upload", zap.Error(err), zap.String("userId", userId))
		response.AppError(w, apperr.ErrPrepareFailed)
		return
	}

	response.Success(w, "upload prepared", prepared)
}

func (h *ImageHandler) ConfirmUpload(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.FromContext(ctx)
	userId := middleware.GetUserID(r)
	idemKey := r.Header.Get("X-Idempotency-Key")

	if idemKey == "" {
		response.AppError(w, apperr.ErrMissingIdemKey)
		return
	}

	var req struct {
		Files []struct {
			Key       string `json:"key"`
			Filename  string `json:"filename"`
			RequestID string `json:"requestId"`
		} `json:"files"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.AppError(w, apperr.ErrInvalidJSON)
		return
	}

	if len(req.Files) == 0 {
		response.AppError(w, apperr.ErrNoFilesToConfirm)
		return
	}

	confirmFiles := make([]services.ConfirmFile, len(req.Files))
	for i, f := range req.Files {
		confirmFiles[i] = services.ConfirmFile{
			Key:       f.Key,
			Filename:  f.Filename,
			RequestID: f.RequestID,
		}
	}

	log.Info("confirm upload received",
		zap.String("idemKey", idemKey),
		zap.String("userId", userId),
		zap.Int("fileCount", len(confirmFiles)),
	)

	enqueued, err := h.Service.ConfirmUpload(ctx, userId, idemKey, confirmFiles)
	if err != nil {
		log.Error("failed to confirm upload", zap.Error(err), zap.String("idemKey", idemKey))
		response.AppError(w, apperr.ErrEnqueueFailed)
		return
	}

	log.Info("upload confirmed", zap.Int("enqueued", enqueued), zap.String("idemKey", idemKey))
	response.Success(w, fmt.Sprintf("%d files enqueued", enqueued), map[string]any{
		"enqueued": enqueued,
		"total":    len(req.Files),
	})
}

func (h *ImageHandler) GetImages(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.FromContext(ctx)
	log.Info("fetching images...")

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	search := r.URL.Query().Get("search")
	status := r.URL.Query().Get("status")

	if page == 0 {
		page = 1
	}
	if limit == 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	userId := middleware.GetUserID(r)
	filters := models.ImageFilters{Search: search, Status: status}

	paginatedResponse, err := h.Service.GetImages(ctx, page, limit, userId, filters)
	if err != nil {
		log.Error("failed to get images", zap.Error(err))
		response.AppError(w, apperr.ErrImageFetchFailed)
		return
	}

	response.Success(w, "images fetched successfully", paginatedResponse)
}

func (h *ImageHandler) GetImageByRequestId(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.FromContext(ctx)

	requestId := chi.URLParam(r, "requestId")
	if requestId == "" {
		response.AppError(w, apperr.ErrMissingRequestID)
		return
	}

	img, err := h.Service.GetImageByRequestId(ctx, requestId)
	if err != nil {
		log.Error("failed to get image", zap.Error(err))
		response.AppError(w, apperr.ErrImageFetchFailed)
		return
	}
	if img == nil {
		response.AppError(w, apperr.ErrImageNotFound)
		return
	}

	response.Success(w, "image fetched successfully", img)
}

func (h *ImageHandler) DeleteImage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.FromContext(ctx)
	log.Info("deleting image...")

	id := chi.URLParam(r, "id")
	userId := middleware.GetUserID(r)

	err := h.Service.DeleteImage(ctx, id, userId)
	if err != nil {
		log.Error("failed to delete image", zap.Error(err))
		response.HandleError(w, err)
		return
	}

	response.Success(w, "image deleted successfully", nil)
}

func (h *ImageHandler) BatchDeleteImages(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.FromContext(ctx)
	userId := middleware.GetUserID(r)

	var req struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.AppError(w, apperr.ErrInvalidJSON)
		return
	}
	if len(req.IDs) == 0 {
		response.AppError(w, apperr.ErrNoIDsProvided)
		return
	}
	if len(req.IDs) > 50 {
		response.AppError(w, apperr.ErrTooManyIDs)
		return
	}

	log.Info("batch delete received", zap.String("userId", userId), zap.Int("count", len(req.IDs)))

	result, err := h.Service.BatchDeleteImages(ctx, req.IDs, userId)
	if err != nil {
		log.Error("batch delete failed", zap.Error(err))
		response.AppError(w, apperr.ErrBatchDeleteFailed)
		return
	}

	log.Info("batch delete complete", zap.Int("deleted", len(result.Deleted)), zap.Int("failed", len(result.Failed)))
	response.Success(w, fmt.Sprintf("%d deleted", len(result.Deleted)), result)
}

func (h *ImageHandler) TransformImage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.FromContext(ctx)
	userId := middleware.GetUserID(r)
	imageId := chi.URLParam(r, "id")

	var req struct {
		Transformations []string `json:"transformations"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.AppError(w, apperr.ErrInvalidJSON)
		return
	}

	if len(req.Transformations) == 0 {
		response.AppError(w, apperr.ErrInvalidTransform.Withf("none provided"))
		return
	}

	for _, t := range req.Transformations {
		if !services.IsValidTransform(t) {
			response.AppError(w, apperr.ErrInvalidTransform.Withf(t))
			return
		}
	}

	updated, err := h.Service.TransformExistingImage(ctx, imageId, userId, req.Transformations)
	if err != nil {
		log.Error("failed to transform image", zap.Error(err))
		response.HandleError(w, err)
		return
	}

	response.Success(w, "transform applied", updated)
}

func (h *ImageHandler) CancelTransform(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.FromContext(ctx)
	userId := middleware.GetUserID(r)
	imageId := chi.URLParam(r, "id")

	if err := h.Service.CancelTransform(ctx, imageId, userId); err != nil {
		log.Error("failed to cancel transform", zap.Error(err))
		response.HandleError(w, err)
		return
	}

	response.Success(w, "transform cancelled", nil)
}

func (h *ImageHandler) GetStorageInfo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.FromContext(ctx)
	userId := middleware.GetUserID(r)

	user, err := h.Service.UserRepo.GetUserById(ctx, userId)
	if err != nil {
		log.Error("failed to get user for storage info", zap.Error(err))
		response.AppError(w, apperr.ErrInternalServer)
		return
	}

	var usedPercent float64
	if user.StorageLimitBytes > 0 {
		usedPercent = float64(user.StorageUsedBytes) / float64(user.StorageLimitBytes) * 100
	}

	response.Success(w, "storage info", map[string]any{
		"usedBytes":   user.StorageUsedBytes,
		"limitBytes":  user.StorageLimitBytes,
		"usedPercent": usedPercent,
	})
}

func isAllowedType(ct string) bool {
	return map[string]bool{
		"image/jpeg": true,
		"image/png":  true,
		"image/webp": true,
	}[ct]
}
