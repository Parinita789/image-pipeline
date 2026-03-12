package middleware

import "net/http"

func GetUserID(r *http.Request) string {
	id, _ := r.Context().Value(UserIdKey).(string)
	return id
}

func GetRequestId(r *http.Request) string {
	requestId, _ := r.Context().Value(RequestIdKey).(string)
	return requestId
}

func GetIdemKey(r *http.Request) string {
	idemKey, _ := r.Context().Value(IdemKey).(string)
	return idemKey
}
