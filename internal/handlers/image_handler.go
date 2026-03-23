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
	"sync"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

const batchSQSThreshold = 5 // batches larger than this go through SQS

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

// PrepareUpload godoc
// @Summary      Prepare file upload
// @Description  Validate files and generate presigned S3 URLs. Max 30 files, 100MB each, 500MB total. Allowed types: image/jpeg, image/png, image/webp.
// @Tags         Images
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body      docs.PrepareUploadRequest  true  "Files to upload"
// @Success      200   {object}  docs.APIResponse{data=[]docs.PreparedFile}
// @Failure      400   {object}  docs.APIResponse  "Validation error"
// @Failure      413   {object}  docs.APIResponse  "Storage quota exceeded"
// @Failure      500   {object}  docs.APIResponse  "Prepare failed"
// @Router       /images/prepare [post]
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

// ConfirmUpload godoc
// @Summary      Confirm upload and enqueue processing
// @Description  Confirm that files have been uploaded to S3 and enqueue them for compression. Requires an idempotency key.
// @Tags         Images
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        X-Idempotency-Key  header  string                    true  "Idempotency key"
// @Param        body               body    docs.ConfirmUploadRequest true  "Uploaded files to confirm"
// @Success      200  {object}  docs.APIResponse{data=docs.ConfirmData}
// @Failure      400  {object}  docs.APIResponse  "Missing idem key or no files"
// @Failure      409  {object}  docs.APIResponse  "Idempotency conflict"
// @Failure      500  {object}  docs.APIResponse  "Enqueue failed"
// @Router       /images/confirm [post]
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

// GetImages godoc
// @Summary      List images
// @Description  Get a paginated list of the authenticated user's images. Supports search and status filtering.
// @Tags         Images
// @Security     BearerAuth
// @Produce      json
// @Param        page    query   int     false  "Page number"   default(1)
// @Param        limit   query   int     false  "Items per page (max 100)"  default(10)
// @Param        search  query   string  false  "Search by filename"
// @Param        status  query   string  false  "Filter by status"
// @Success      200  {object}  docs.APIResponse{data=docs.PaginatedImages}
// @Failure      500  {object}  docs.APIResponse  "Fetch failed"
// @Router       /images [get]
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

// GetImageByRequestId godoc
// @Summary      Get image by request ID
// @Description  Fetch a single image by its request ID (UUID assigned during prepare).
// @Tags         Images
// @Security     BearerAuth
// @Produce      json
// @Param        requestId  path  string  true  "Request ID (UUID)"
// @Success      200  {object}  docs.APIResponse{data=models.Image}
// @Failure      400  {object}  docs.APIResponse  "Missing request ID"
// @Failure      404  {object}  docs.APIResponse  "Image not found"
// @Failure      500  {object}  docs.APIResponse  "Fetch failed"
// @Router       /images/{requestId} [get]
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

// DeleteImage godoc
// @Summary      Delete an image
// @Description  Delete a single image by ID. Only the owning user can delete.
// @Tags         Images
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  string  true  "Image ID"
// @Success      200  {object}  docs.APIResponse  "Deleted"
// @Failure      403  {object}  docs.APIResponse  "Forbidden"
// @Failure      404  {object}  docs.APIResponse  "Not found"
// @Failure      500  {object}  docs.APIResponse  "Delete failed"
// @Router       /image/{id} [delete]
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

// BatchDeleteImages godoc
// @Summary      Batch delete images
// @Description  Delete multiple images by IDs. Max 50 per request.
// @Tags         Images
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body      docs.BatchIDsRequest  true  "Image IDs to delete"
// @Success      200   {object}  docs.APIResponse{data=docs.DeleteResult}
// @Failure      400   {object}  docs.APIResponse  "No IDs or too many"
// @Failure      500   {object}  docs.APIResponse  "Batch delete failed"
// @Router       /images [delete]
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

// TransformImage godoc
// @Summary      Transform a single image
// @Description  Enqueue transformations (grayscale, sepia, blur, sharpen, invert, resize, crop, watermark, format, remove-bg) for a single image.
// @Tags         Transforms
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  string                 true  "Image ID"
// @Param        body  body  docs.TransformRequest  true  "Transformations to apply"
// @Success      200   {object}  docs.APIResponse{data=models.Image}
// @Failure      400   {object}  docs.APIResponse  "Invalid transform"
// @Failure      403   {object}  docs.APIResponse  "Forbidden"
// @Failure      404   {object}  docs.APIResponse  "Not found"
// @Router       /images/{id}/transform [post]
func (h *ImageHandler) TransformImage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.FromContext(ctx)
	userId := middleware.GetUserID(r)
	imageId := chi.URLParam(r, "id")

	var req struct {
		Transformations []models.TransformConfig `json:"transformations"`
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
		if err := services.IsValidTransform(t); err != nil {
			response.AppError(w, apperr.ErrInvalidTransform.Withf(err.Error()))
			return
		}
	}

	img, err := h.Service.EnqueueTransform(ctx, imageId, userId, req.Transformations)
	if err != nil {
		log.Error("failed to enqueue transform", zap.Error(err))
		response.HandleError(w, err)
		return
	}

	response.Success(w, "transform enqueued", img)
}

// BatchTransformImages godoc
// @Summary      Batch transform images
// @Description  Apply the same transformations to multiple images. Batches ≤10 are processed synchronously; larger batches are enqueued to SQS.
// @Tags         Transforms
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  docs.BatchTransformRequest  true  "Image IDs and transformations"
// @Success      200   {object}  docs.APIResponse{data=docs.BatchSyncResult}   "Small batch (≤10) — sync result"
// @Success      200   {object}  docs.APIResponse{data=docs.BatchAsyncResult}  "Large batch (>10) — async enqueued"
// @Failure      400   {object}  docs.APIResponse  "Invalid input"
// @Failure      500   {object}  docs.APIResponse  "Enqueue failed"
// @Router       /images/batch-transform [post]
func (h *ImageHandler) BatchTransformImages(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.FromContext(ctx)
	userId := middleware.GetUserID(r)

	var req struct {
		Ids             []string                 `json:"ids"`
		Transformations []models.TransformConfig `json:"transformations"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.AppError(w, apperr.ErrInvalidJSON)
		return
	}

	if len(req.Ids) == 0 {
		response.AppError(w, apperr.ErrInvalidJSON)
		return
	}
	if len(req.Transformations) == 0 {
		response.AppError(w, apperr.ErrInvalidTransform.Withf("none provided"))
		return
	}

	for _, t := range req.Transformations {
		if err := services.IsValidTransform(t); err != nil {
			response.AppError(w, apperr.ErrInvalidTransform.Withf(err.Error()))
			return
		}
	}

	// Small batches: process concurrently in-handler (fast, no SQS latency)
	// Large batches: enqueue to SQS (bounded memory, horizontally scalable)
	if len(req.Ids) <= batchSQSThreshold {
		type result struct {
			Id    string `json:"id"`
			Error string `json:"error,omitempty"`
		}
		results := make([]result, len(req.Ids))
		var wg sync.WaitGroup
		sem := make(chan struct{}, 5) // max 5 concurrent transforms

		for i, id := range req.Ids {
			wg.Add(1)
			go func(idx int, imageId string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				_, err := h.Service.TransformExistingImage(ctx, imageId, userId, req.Transformations)
				if err != nil {
					log.Error("batch transform failed", zap.String("id", imageId), zap.Error(err))
					results[idx] = result{Id: imageId, Error: err.Error()}
				} else {
					results[idx] = result{Id: imageId}
				}
			}(i, id)
		}
		wg.Wait()

		var succeeded, failed []result
		for _, r := range results {
			if r.Error != "" {
				failed = append(failed, r)
			} else {
				succeeded = append(succeeded, r)
			}
		}

		response.Success(w, "batch transform completed", map[string]any{
			"succeeded": succeeded,
			"failed":    failed,
		})
		return
	}

	// Large batch: enqueue to SQS
	batchId, err := h.Service.EnqueueBatchTransform(ctx, userId, req.Ids, req.Transformations)
	if err != nil {
		log.Error("failed to enqueue batch transform", zap.Error(err))
		response.HandleError(w, err)
		return
	}

	response.Success(w, "batch transform enqueued", map[string]any{
		"batchId": batchId,
		"total":   len(req.Ids),
	})
}

// RevertTransform godoc
// @Summary      Revert transform on a single image
// @Description  Remove all applied transformations and restore the compressed version.
// @Tags         Transforms
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  string  true  "Image ID"
// @Success      200  {object}  docs.APIResponse{data=models.Image}
// @Failure      403  {object}  docs.APIResponse  "Forbidden"
// @Failure      404  {object}  docs.APIResponse  "Not found"
// @Router       /images/{id}/revert-transform [post]
func (h *ImageHandler) RevertTransform(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.FromContext(ctx)
	userId := middleware.GetUserID(r)
	imageId := chi.URLParam(r, "id")

	updated, err := h.Service.RevertTransform(ctx, imageId, userId)
	if err != nil {
		log.Error("failed to revert transform", zap.Error(err))
		response.HandleError(w, err)
		return
	}

	response.Success(w, "transform reverted", updated)
}

// BatchRevertTransform godoc
// @Summary      Batch revert transforms
// @Description  Revert transformations on multiple images. Batches ≤10 processed synchronously; larger batches enqueued to SQS.
// @Tags         Transforms
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  docs.BatchIDsRequest  true  "Image IDs to revert"
// @Success      200   {object}  docs.APIResponse{data=docs.BatchSyncResult}   "Small batch — sync result"
// @Success      200   {object}  docs.APIResponse{data=docs.BatchAsyncResult}  "Large batch — async enqueued"
// @Failure      400   {object}  docs.APIResponse  "Invalid input"
// @Failure      500   {object}  docs.APIResponse  "Enqueue failed"
// @Router       /images/batch-revert-transform [post]
func (h *ImageHandler) BatchRevertTransform(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.FromContext(ctx)
	userId := middleware.GetUserID(r)

	var req struct {
		Ids []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.AppError(w, apperr.ErrInvalidJSON)
		return
	}
	if len(req.Ids) == 0 {
		response.AppError(w, apperr.ErrInvalidJSON)
		return
	}

	// Small batches: process concurrently in-handler
	// Large batches: enqueue to SQS
	if len(req.Ids) <= batchSQSThreshold {
		type result struct {
			Id    string `json:"id"`
			Error string `json:"error,omitempty"`
		}
		results := make([]result, len(req.Ids))
		var wg sync.WaitGroup
		sem := make(chan struct{}, 5)

		for i, id := range req.Ids {
			wg.Add(1)
			go func(idx int, imageId string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				_, err := h.Service.RevertTransform(ctx, imageId, userId)
				if err != nil {
					log.Error("batch revert failed", zap.String("id", imageId), zap.Error(err))
					results[idx] = result{Id: imageId, Error: err.Error()}
				} else {
					results[idx] = result{Id: imageId}
				}
			}(i, id)
		}
		wg.Wait()

		var succeeded, failed []result
		for _, r := range results {
			if r.Error != "" {
				failed = append(failed, r)
			} else {
				succeeded = append(succeeded, r)
			}
		}

		response.Success(w, "batch revert completed", map[string]any{
			"succeeded": succeeded,
			"failed":    failed,
		})
		return
	}

	// Large batch: enqueue to SQS
	batchId, err := h.Service.EnqueueBatchRevert(ctx, userId, req.Ids)
	if err != nil {
		log.Error("failed to enqueue batch revert", zap.Error(err))
		response.HandleError(w, err)
		return
	}

	response.Success(w, "batch revert enqueued", map[string]any{
		"batchId": batchId,
		"total":   len(req.Ids),
	})
}

// GetBatchStatus godoc
// @Summary      Get batch job status
// @Description  Poll the progress of a batch transform or revert operation.
// @Tags         Transforms
// @Security     BearerAuth
// @Produce      json
// @Param        batchId  path  string  true  "Batch ID"
// @Success      200  {object}  docs.APIResponse{data=models.BatchJob}
// @Failure      403  {object}  docs.APIResponse  "Forbidden"
// @Failure      404  {object}  docs.APIResponse  "Not found"
// @Router       /batches/{batchId} [get]
func (h *ImageHandler) GetBatchStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.FromContext(ctx)
	userId := middleware.GetUserID(r)
	batchId := chi.URLParam(r, "batchId")

	batch, err := h.Service.GetBatchStatus(ctx, batchId, userId)
	if err != nil {
		log.Error("failed to get batch status", zap.Error(err))
		response.HandleError(w, err)
		return
	}

	response.Success(w, "batch status", batch)
}

// CancelTransform godoc
// @Summary      Cancel a pending transform
// @Description  Cancel a transform that is still in "processing" status.
// @Tags         Transforms
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  string  true  "Image ID"
// @Success      200  {object}  docs.APIResponse  "Cancelled"
// @Failure      403  {object}  docs.APIResponse  "Forbidden"
// @Failure      404  {object}  docs.APIResponse  "Not found"
// @Router       /images/{id}/cancel-transform [post]
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

// GetStorageInfo godoc
// @Summary      Get storage usage
// @Description  Returns the user's current storage usage, quota limit, and usage percentage.
// @Tags         Storage
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  docs.APIResponse{data=docs.StorageInfo}
// @Failure      500  {object}  docs.APIResponse  "Internal error"
// @Router       /storage [get]
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
