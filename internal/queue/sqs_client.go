package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"image-pipeline/internal/models"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

type SQSClient struct {
	Client   *sqs.Client
	QueueURL string
}

func NewSQSClient(queueURL string) (*SQSClient, error) {
	opts := []func(*config.LoadOptions) error{}

	if endpoint := os.Getenv("AWS_ENDPOINT_URL"); endpoint != "" {
		opts = append(opts, config.WithEndpointResolverWithOptions(
			aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
				return aws.Endpoint{URL: endpoint, HostnameImmutable: true}, nil
			}),
		))
	}

	cfg, err := config.LoadDefaultConfig(context.Background(), opts...)
	if err != nil {
		return nil, fmt.Errorf("sqs config load failed: %w", err)
	}

	client := sqs.NewFromConfig(cfg)

	return &SQSClient{
		Client:   client,
		QueueURL: queueURL,
	}, nil
}

// NewSQSClientFromConfig accepts an existing config — for tests
func NewSQSClientFromConfig(cfg aws.Config, queueURL string) *SQSClient {
	return &SQSClient{
		Client:   sqs.NewFromConfig(cfg),
		QueueURL: queueURL,
	}
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
