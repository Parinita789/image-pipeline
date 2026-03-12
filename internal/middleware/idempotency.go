package middleware

import (
	"context"
	"time"

	"image-pipeline/internal/models"
	"image-pipeline/internal/repository"
	"image-pipeline/internal/utils"
	"image-pipeline/pkg/response"
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

			hash := utils.HashBody(filename, int(r.ContentLength), GetUserID(r))

			record, acquired, err := repo.Acquire(r.Context(), key, hash)
			if err != nil {
				response.Error(w, http.StatusInternalServerError, "Something Went Wrong!")
				return
			}

			// Not first owner of this key in the race condition - handle based on exisiting record
			if !acquired {
				handled := handleExistingIdempotentRecord(w, record, hash)
				if handled {
					return
				}
			}

			ctx := context.WithValue(r.Context(), IdemKey, key)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func handleExistingIdempotentRecord(
	w http.ResponseWriter,
	record *models.IdempotencyRecord,
	hash string,
) (handled bool) {
	if record == nil {
		return false
	}

	if record.RequestHash != hash {
		response.Error(
			w,
			http.StatusConflict,
			"Idempotency key reused with different request",
		)
		return true
	}

	switch record.Status {
	case models.StatusCompleted:
		response.Success(w, "Image Upload Successful!", record.Response)
		return true

	case models.StatusProcessing, models.StatusStarted:
		stuckThreshold := 1 * time.Minute
		if time.Since(record.UpdatedAt) < stuckThreshold {
			response.Error(w, http.StatusConflict, "request is still processing, please wait")
			return true
		}
		return false

	case models.StatusFailed:
		response.Error(w, http.StatusInternalServerError, "Something Went Wrong!")
		return false
	}

	return false
}
