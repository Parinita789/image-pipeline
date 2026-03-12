package models

type UploadMessage struct {
	IdempotencyKey string `json:"idempotencyKey"`
	FileName       string `json:"filename"`
	UserId         string `json:"userId"`
	RequestId      string `json:"requestId"`
	TempS3Key      string `json:"tempS3Key"`
	ContentType    string `json:"contentType`
}
