package handlers

import (
	"encoding/json"
	"image-pipeline/internal/services"
	"io"
	"net/http"

	"go.uber.org/zap"
)

type UploadHandler struct {
	Service *services.UploadService
	logger  *zap.Logger
}

func NewUploadHandler(service *services.UploadService) *UploadHandler {
	return &UploadHandler{
		Service: service,
		logger:  service.Logger,
	}
}

func (h *UploadHandler) Upload(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	file, header, err := r.FormFile("file")
	if err != nil {
		h.logger.Error("Failed to read file from request", zap.Error(err))
		http.Error(w, "Failed to read file from request", http.StatusBadRequest)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		h.logger.Error("Failed to read file data", zap.Error(err))
		http.Error(w, "Failed to read file data", http.StatusInternalServerError)
		return
	}

	raw, compressed, err := h.Service.ProcessUpload(ctx, header.Filename, data)
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
