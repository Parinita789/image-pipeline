package validator

import "github.com/go-playground/validator/v10"

var validate = validator.New()

func ValidateStruct(v interface{}) error {
	return validate.Struct(v)
}
