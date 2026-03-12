package services

import "context"

func (s *ImageService) runS3(ctx context.Context, fn func(ctx context.Context) error) error {
	return s.s3Exec.Execute(ctx, fn)
}

func (s *ImageService) runSQS(ctx context.Context, fn func(ctx context.Context) error) error {
	return s.sqsExec.Execute(ctx, fn)
}
