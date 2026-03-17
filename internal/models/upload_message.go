package models

type UploadMessage struct {
	IdempotencyKey string `json:"idempotencyKey"`
	FileName       string `json:"filename"`
	UserId         string `json:"userId"`
	RequestId      string `json:"requestId"`
	RawS3Key       string `json:"rawS3Key"`
	ContentType    string `json:"contentType"`
}
