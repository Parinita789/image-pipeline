package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"image-pipeline/internal/models"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

type SQSClient struct {
	Client   *sqs.Client
	QueueURL string
}

func NewSQSClient(queueURL string) (*SQSClient, error) {
	cfg, err := config.LoadDefaultConfig(context.Background())

	if err != nil {
		return nil, err
	}

	client := sqs.NewFromConfig(cfg)

	return &SQSClient{
		Client:   client,
		QueueURL: queueURL,
	}, nil
}

func (s *SQSClient) DeleteMessage(
	ctx context.Context,
	receiptHandle string,
) error {

	_, err := s.Client.DeleteMessage(
		ctx,
		&sqs.DeleteMessageInput{
			QueueUrl:      &s.QueueURL,
			ReceiptHandle: &receiptHandle,
		},
	)

	return err
}

func (s *SQSClient) PublishUpload(
	ctx context.Context,
	msg models.UploadMessage,
) error {
	if s.Client == nil {
		return fmt.Errorf("sqs client is nil")
	}
	body, err := json.Marshal(msg)
	if err != nil {
		fmt.Println("erorr in sqs msg", err)
		return err
	}

	out, err := s.Client.SendMessage(
		ctx,
		&sqs.SendMessageInput{
			QueueUrl:    &s.QueueURL,
			MessageBody: aws.String(string(body)),
		},
	)

	if err != nil {
		return fmt.Errorf("failed to send sqs message: %w", err)
	}

	if out.MessageId != nil {
		fmt.Println("SQS message sent:", *out.MessageId)
	}

	return nil
}
