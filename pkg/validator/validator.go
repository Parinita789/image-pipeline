package validator

import (
	"fmt"
	"strings"

	"github.com/go-playground/validator/v10"
)

var validate = validator.New()

// fieldLabels maps DTO field names to user-friendly labels.
var fieldLabels = map[string]string{
	"Email":           "email",
	"Password":        "password",
	"FirstName":       "first name",
	"LastName":        "last name",
	"NewPassword":     "new password",
	"CurrentPassword": "current password",
	"Token":           "token",
}

func friendlyField(field string) string {
	if label, ok := fieldLabels[field]; ok {
		return label
	}
	return strings.ToLower(field)
}

func friendlyMessage(fe validator.FieldError) string {
	field := friendlyField(fe.Field())
	switch fe.Tag() {
	case "required":
		return fmt.Sprintf("%s is required", field)
	case "email":
		return fmt.Sprintf("%s must be a valid email address", field)
	case "min":
		return fmt.Sprintf("%s must be at least %s characters", field, fe.Param())
	default:
		return fmt.Sprintf("%s is invalid", field)
	}
}

func ValidateStruct(v interface{}) error {
	err := validate.Struct(v)
	if err == nil {
		return nil
	}
	errs, ok := err.(validator.ValidationErrors)
	if !ok {
		return err
	}
	msgs := make([]string, 0, len(errs))
	for _, fe := range errs {
		msgs = append(msgs, friendlyMessage(fe))
	}
	return fmt.Errorf("%s", strings.Join(msgs, "; "))
}
