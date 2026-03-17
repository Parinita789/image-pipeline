package middleware

import (
	"context"
	"time"

	"image-pipeline/internal/models"
	"image-pipeline/internal/repository"
	"image-pipeline/internal/utils"
	apperr "image-pipeline/pkg/errors"
	"image-pipeline/pkg/response"
	"net/http"

	"github.com/google/uuid"
)

func IdempotencyCheck(repo *repository.IdempotencyRepo) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get("X-Idempotency-Key")
			filename := r.Header.Get("X-File-Name")

			if key == "" {
				key = uuid.New().String()
			}

			hash := utils.HashBody(filename, int(r.ContentLength), GetUserID(r))

			record, acquired, err := repo.Acquire(r.Context(), key, hash)
			if err != nil {
				response.AppError(w, apperr.ErrInternalServer)
				return
			}

			// Not first owner of this key in the race condition - handle based on existing record
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
		response.AppError(w, apperr.ErrIdemKeyConflict)
		return true
	}

	switch record.Status {
	case models.StatusCompleted:
		response.Success(w, "upload already processed", record.Response)
		return true

	case models.StatusProcessing, models.StatusStarted:
		stuckThreshold := 1 * time.Minute
		if time.Since(record.UpdatedAt) < stuckThreshold {
			response.AppError(w, apperr.ErrRequestProcessing)
			return true
		}
		return false

	case models.StatusFailed:
		response.AppError(w, apperr.ErrInternalServer)
		return false
	}

	return false
}
