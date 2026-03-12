package handlers

import (
	"encoding/json"
	"image-pipeline/internal/logger"
	"image-pipeline/internal/middleware"
	"image-pipeline/internal/services"
	"image-pipeline/pkg/response"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

type ImageHandler struct {
	Service *services.ImageService
}

func NewImageHandler(service *services.ImageService) *ImageHandler {
	return &ImageHandler{
		Service: service,
	}
}

func (h *ImageHandler) Upload(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.FromContext(ctx)
	log.Info("upload started")

	requestId := middleware.GetRequestId(r)
	userId := middleware.GetUserID(r)
	idemKey := middleware.GetIdemKey(r)

	if requestId == "" {
		response.Error(w, http.StatusBadRequest, "missing X-Request-ID")
		return
	}

	err := r.ParseMultipartForm(32 << 20)
	if err != nil {
		log.Error("invalid multipart form", zap.Error(err))
		response.Error(w, http.StatusBadRequest, "Failed to read file from request")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		log.Error("File missing", zap.Error(err))
		response.Error(w, http.StatusBadRequest, "file missing")
		return
	}
	defer file.Close()

	// validate content-type
	buf := make([]byte, 512)
	_, err = file.Read(buf)
	if err != nil {
		log.Error("failed to read file", zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "failed to read file")
		return
	}
	contentType := http.DetectContentType(buf)

	if !isAllowedType(contentType) {
		response.Error(w, http.StatusBadRequest, "unsupported file type")
		return
	}

	// validate size
	if header.Size > 100*1024*1024 {
		response.Error(w, http.StatusBadRequest, "file too large (max 100MB)")
		return
	}

	// streaming begins here
	err = h.Service.EnqueueUpload(ctx, requestId, userId, idemKey, header.Filename, file)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to enqueue upload")
		return
	}

	response.Success(w, "upload started!", map[string]string{
		"status":     "processing",
		"request_id": requestId,
	})
}

func isAllowedType(ct string) bool {
	allowed := map[string]bool{
		"image/jpeg": true,
		"image/png":  true,
		"image/webp": true,
	}
	return allowed[ct]
}

func (h *ImageHandler) GetImages(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.FromContext(ctx)
	log.Info("fetching images...")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	if page == 0 {
		page = 1
	}

	if limit == 0 {
		limit = 10
	}

	userId := middleware.GetUserID(r)

	paginatedResponse, err := h.Service.GetImages(ctx, page, limit, userId)
	if err != nil {
		log.Error("Failed to get images", zap.Error(err))
		http.Error(w, "Failed to get images", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"images": paginatedResponse.Images,
		"total":  paginatedResponse.Total,
		"page":   paginatedResponse.Page,
		"limit":  paginatedResponse.Limit,
	})
}

func (h *ImageHandler) DeleteImage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.FromContext(r.Context())
	log.Info("deleting image...")
	id := chi.URLParam(r, "id")

	err := h.Service.DeleteImage(ctx, id)
	if err != nil {
		log.Error("Failed to delete image", zap.Error(err))
		http.Error(w, "Failed to delete image", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
