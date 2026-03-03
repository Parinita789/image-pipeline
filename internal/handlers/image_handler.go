package handlers

import (
	"encoding/json"
	"image-pipeline/internal/middleware"
	"image-pipeline/internal/services"
	"image-pipeline/internal/utils"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

type ImageHandler struct {
	Service *services.ImageService
	logger  *zap.Logger
}

func NewImageHandler(service *services.ImageService, logger *zap.Logger) *ImageHandler {
	return &ImageHandler{
		Service: service,
		logger:  logger,
	}
}

func (h *ImageHandler) Upload(w http.ResponseWriter, r *http.Request) {
	requestId := r.Header.Get("X-Request-ID")
	// userId := middleware.GetUserID(r)

	ctx := r.Context()
	file, header, err := r.FormFile("file")
	if err != nil {
		h.logger.Error("Failed to read file from request", zap.Error(err))
		http.Error(w, "Failed to read file from request", http.StatusBadRequest)
		return
	}

	if requestId == "" {
		requestId = utils.GenerateFingerPrint(header.Filename, []byte(r.RemoteAddr), r.Header.Get("X-User-ID"))
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		h.logger.Error("Failed to read file data", zap.Error(err))
		http.Error(w, "Failed to read file data", http.StatusInternalServerError)
		return
	}

	raw, compressed, err := h.Service.ProcessUpload(
		ctx,
		requestId,
		header.Filename,
		data,
	)
	if err != nil {
		h.logger.Error("Failed to process upload", zap.Error(err))
		http.Error(w, "Failed to process upload", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{
		"raw_url":        raw,
		"compressed_url": compressed,
	})

	h.logger.Info("Upload successful",
		zap.String("filename", header.Filename),
		zap.String("raw_url", raw),
		zap.String("compressed_url", compressed),
	)
}

func (h *ImageHandler) GetImages(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	if page == 0 {
		page = 1
	}

	if limit == 0 {
		limit = 10
	}

	userId := r.Context().
		Value(middleware.UserIdKey).(string)

	paginatedResponse, err := h.Service.GetImages(ctx, page, limit, userId)
	if err != nil {
		h.logger.Error("Failed to get images", zap.Error(err))
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
	id := chi.URLParam(r, "id")

	err := h.Service.DeleteImage(ctx, id)
	if err != nil {
		h.logger.Error("Failed to delete image", zap.Error(err))
		http.Error(w, "Failed to delete image", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
