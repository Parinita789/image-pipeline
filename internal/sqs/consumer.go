package sqs

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"go.uber.org/zap"
)

type Consumer struct {
	Client   *sqs.Client
	QueueURL string
	logger   *zap.Logger
}

func (c *Consumer) Poll(handler func(string)) {
	for {
		resp, err := c.Client.ReceiveMessage(context.TODO(), &sqs.ReceiveMessageInput{
			QueueUrl:            &c.QueueURL,
			MaxNumberOfMessages: 5,
		})
		if err != nil {
			c.logger.Error("Error receiving message", zap.Error(err))
			continue
		}
		for _, msg := range resp.Messages {
			handler(*msg.Body)

			_, err := c.Client.DeleteMessage(context.TODO(), &sqs.DeleteMessageInput{
				QueueUrl:      &c.QueueURL,
				ReceiptHandle: msg.ReceiptHandle,
			})
			if err != nil {
				c.logger.Error("Error deleting message", zap.Error(err))
			}
		}
	}

}
