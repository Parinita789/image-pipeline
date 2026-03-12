package middleware

type contextKey string

const (
	RequestIdKey contextKey = "requestId"
	UserIdKey    contextKey = "userId"
	IdemKey      contextKey = "idemKey"
	LoggerKey    contextKey = "logger"
)
