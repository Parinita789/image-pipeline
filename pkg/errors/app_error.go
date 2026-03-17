package errors

import "fmt"

type AppError struct {
	HTTPCode int    `json:"-"`
	Code     string `json:"code"`
	Message  string `json:"message"`
}

func (e *AppError) Error() string {
	return e.Message
}

// New creates an AppError.
func New(httpCode int, code, message string) *AppError {
	return &AppError{HTTPCode: httpCode, Code: code, Message: message}
}

func (e *AppError) Withf(args ...any) *AppError {
	return &AppError{
		HTTPCode: e.HTTPCode,
		Code:     e.Code,
		Message:  fmt.Sprintf(e.Message, args...),
	}
}
