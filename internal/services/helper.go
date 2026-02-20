package services

import "context"

func (s *UploadService) runS3(ctx context.Context, fn func(ctx context.Context) error) error {
	return s.s3Exec.Execute(ctx, fn)
}
