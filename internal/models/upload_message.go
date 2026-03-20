package models

const (
	ActionCompress  = "compress"
	ActionTransform = "transform"
)

type UploadMessage struct {
	Action          string   `json:"action"`
	IdempotencyKey  string   `json:"idempotencyKey"`
	FileName        string   `json:"filename"`
	UserId          string   `json:"userId"`
	RequestId       string   `json:"requestId"`
	RawS3Key        string   `json:"rawS3Key"`
	ContentType     string   `json:"contentType"`
	Transformations []string `json:"transformations,omitempty"`
}
