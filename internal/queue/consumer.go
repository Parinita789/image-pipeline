package queue

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

func (s *SQSClient) ReceiveMessage(ctx context.Context) ([]types.Message, error) {
	out, err := s.Client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:            aws.String(s.QueueURL),
		MaxNumberOfMessages: 5,
		WaitTimeSeconds:     10,
	})

	if err != nil {
		return nil, err
	}

	return out.Messages, nil
}
