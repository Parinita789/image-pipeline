package response

import (
	"encoding/json"
	apperr "image-pipeline/pkg/errors"
	"net/http"
)

type APIResponse struct {
	Status  string      `json:"status"`
	Code    int         `json:"code"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

func JSON(w http.ResponseWriter, code int, resp APIResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(resp)
}

func Success(w http.ResponseWriter, msg string, data interface{}) {
	JSON(w, http.StatusOK, APIResponse{
		Status:  "success",
		Code:    200,
		Message: msg,
		Data:    data,
	})
}

// Error sends a plain error response (for cases where you don't have an AppError).
func Error(w http.ResponseWriter, code int, msg string) {
	JSON(w, code, APIResponse{
		Status:  "error",
		Code:    code,
		Message: msg,
	})
}

// AppError sends a structured error response from an *AppError.
func AppError(w http.ResponseWriter, err *apperr.AppError) {
	JSON(w, err.HTTPCode, APIResponse{
		Status:  "error",
		Code:    err.HTTPCode,
		Message: err.Message,
	})
}

// HandleError inspects err: if it's an *AppError, sends the typed response;
// otherwise logs as unexpected and sends a generic 500.
func HandleError(w http.ResponseWriter, err error) {
	if ae, ok := err.(*apperr.AppError); ok {
		AppError(w, ae)
		return
	}
	AppError(w, apperr.ErrInternalServer)
}
