package request

import (
	"encoding/json"
	"errors"
	"net/http"
)

const MaxBodySize = 10 << 20 // 10MB

func DecodeJSON(r *http.Request, dst interface{}) error {
	if r.Body == nil {
		return errors.New("empty request body")
	}

	r.Body = http.MaxBytesReader(nil, r.Body, MaxBodySize)

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(dst); err != nil {
		return err
	}
	return nil
}
