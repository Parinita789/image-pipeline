package handlers

import (
	"encoding/json"
	"fmt"
	"image-pipeline/internal/services"
	"io"
	"net/http"
)

type UploadHandler struct {
	Service *services.UploadService
}

func NewUploadHandler(service *services.UploadService) *UploadHandler {
	return &UploadHandler{
		Service: service,
	}
}

func (h *UploadHandler) Upload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Failed to read file from request", http.StatusBadRequest)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "Failed to read file data", http.StatusInternalServerError)
		return
	}

	raw, compressed, err := h.Service.ProcessUpload(header.Filename, data)
	if err != nil {
		http.Error(w, "Failed to process upload", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{
		"raw_url":        raw,
		"compressed_url": compressed,
	})

	fmt.Fprintf(w, "Uploaded: %s", compressed)
}
