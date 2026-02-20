package repository

import "context"

func (r *ImageRepo) runMongo(ctx context.Context, fn func(context.Context) error) error {
	return r.exec.Execute(ctx, fn)
}
