package queue

import (
	"context"
	"encoding/json"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

type UploadMessage struct {
	IdempotencyKey string `json:"idempotencyKey"`
	Filename       string `json:"filename"`
}

func (s *SQSClient) Publish(ctx context.Context, msg UploadMessage) error {
	body, _ := json.Marshal(msg)
	_, err := s.Client.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    aws.String(string(s.QueueURL)),
		MessageBody: aws.String(string(body)),
	})

	return err
}
