package middleware

import (
	"bytes"
	"context"

	"image-pipeline/internal/models"
	"image-pipeline/internal/repository"
	"image-pipeline/internal/utils"
	"image-pipeline/pkg/response"
	"io"
	"net/http"

	"github.com/google/uuid"
)

func IdempotencyCheck(repo *repository.IdempotencyRepo) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get("X-Idempotency-Key")
			filename := r.Header.Get("X-File-Name")

			if filename == "" {
				response.Error(w, http.StatusBadRequest, "filename required!")
				return
			}
			if key == "" {
				key = uuid.New().String()
			}

			// Read body for hashing
			body, err := io.ReadAll(r.Body)
			if err != nil {
				response.Error(w, http.StatusInternalServerError, "failed to read request")
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))

			hash := utils.HashBody(filename, int(r.ContentLength), GetUserID(r))

			record, err := repo.Get(r.Context(), key)

			if err != nil {
				response.Error(w, http.StatusInternalServerError, "Something Went Wrong!")
				return
			}

			if record != nil {
				if record.RequestHash != hash {
					response.Error(w,
						http.StatusConflict,
						"Idempotency key reused with different request",
					)
					return
				}
			}

			if record != nil && record.Status == models.StatusCompleted {
				response.Success(w, "Image Upload Successful!", record.Response)
				return
			}

			if record == nil {
				if err := repo.Create(r.Context(), key, hash); err != nil {
					response.Error(w, http.StatusInternalServerError, "Something Went Wrong!")
					return
				}
			}

			ctx := context.WithValue(r.Context(), IdemKey, key)
			next.ServeHTTP(w, r.WithContext(ctx))

		})
	}
}
