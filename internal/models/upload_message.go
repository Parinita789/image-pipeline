package models

const (
	ActionCompress       = "compress"
	ActionTransform      = "transform"
	ActionBatchTransform = "batch-transform"
	ActionBatchRevert    = "batch-revert"
)

type UploadMessage struct {
	Action          string            `json:"action"`
	IdempotencyKey  string            `json:"idempotencyKey"`
	FileName        string            `json:"filename"`
	UserId          string            `json:"userId"`
	RequestId       string            `json:"requestId"`
	RawS3Key        string            `json:"rawS3Key"`
	ContentType     string            `json:"contentType"`
	Transformations []TransformConfig `json:"transformations,omitempty"`
	BatchId         string            `json:"batchId,omitempty"`
	ImageId         string            `json:"imageId,omitempty"`
}
