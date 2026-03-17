package handlers

import (
	"encoding/json"
	"fmt"
	"image-pipeline/internal/logger"
	"image-pipeline/internal/middleware"
	"image-pipeline/internal/models"
	"image-pipeline/internal/services"
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
		response.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(req.Files) == 0 {
		response.Error(w, http.StatusBadRequest, "no files provided")
		return
	}
	if len(req.Files) > maxFiles {
		response.Error(w, http.StatusBadRequest, fmt.Sprintf("too many files — max %d allowed", maxFiles))
		return
	}

	for _, f := range req.Files {
		if f.Size > maxFileSize {
			response.Error(w, http.StatusBadRequest, fmt.Sprintf("%s exceeds max file size (100MB)", f.Filename))
			return
		}
		if !isAllowedType(f.ContentType) {
			response.Error(w, http.StatusBadRequest, fmt.Sprintf("%s: unsupported type — allowed: jpeg, png, webp", f.Filename))
			return
		}
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
		response.Error(w, http.StatusInternalServerError, "failed to prepare upload")
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
		response.Error(w, http.StatusBadRequest, "missing X-Idempotency-Key")
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
		response.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(req.Files) == 0 {
		response.Error(w, http.StatusBadRequest, "no files to confirm")
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
		response.Error(w, http.StatusInternalServerError, "failed to enqueue uploads")
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
		response.Error(w, http.StatusInternalServerError, "failed to get images")
		return
	}

	response.Success(w, "images fetched successfully", paginatedResponse)
}

func (h *ImageHandler) GetImageByRequestId(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.FromContext(ctx)

	requestId := chi.URLParam(r, "requestId")
	if requestId == "" {
		response.Error(w, http.StatusBadRequest, "missing requestId")
		return
	}

	img, err := h.Service.GetImageByRequestId(ctx, requestId)
	if err != nil {
		log.Error("failed to get image", zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "failed to get image")
		return
	}
	if img == nil {
		response.Error(w, http.StatusNotFound, "image not found")
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
		if err == services.ErrImageNotFound {
			response.Error(w, http.StatusNotFound, "image not found")
			return
		}
		if err == services.ErrUnauthorized {
			response.Error(w, http.StatusForbidden, "you do not own this image")
			return
		}
		log.Error("failed to delete image", zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "failed to delete image")
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
		response.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.IDs) == 0 {
		response.Error(w, http.StatusBadRequest, "no ids provided")
		return
	}
	if len(req.IDs) > 50 {
		response.Error(w, http.StatusBadRequest, "too many ids — max 50 per request")
		return
	}

	log.Info("batch delete received", zap.String("userId", userId), zap.Int("count", len(req.IDs)))

	result, err := h.Service.BatchDeleteImages(ctx, req.IDs, userId)
	if err != nil {
		log.Error("batch delete failed", zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "batch delete failed")
		return
	}

	log.Info("batch delete complete", zap.Int("deleted", len(result.Deleted)), zap.Int("failed", len(result.Failed)))
	response.Success(w, fmt.Sprintf("%d deleted", len(result.Deleted)), result)
}

func isAllowedType(ct string) bool {
	return map[string]bool{
		"image/jpeg": true,
		"image/png":  true,
		"image/webp": true,
	}[ct]
}
