package response

import (
	"encoding/json"
	"net/http"
)

type APIResponse struct {
	Status  string      `json:"status"`
	Code    int         `json:"code"`
	Message string      `json:"message",omitempty"`
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

func Error(w http.ResponseWriter, code int, msg string) {
	JSON(w, code, APIResponse{
		Status:  "error",
		Code:    code,
		Message: msg,
	})
}
