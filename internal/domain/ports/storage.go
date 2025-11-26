package ports

import "context"

type Storage interface {
	Upload(ctx context.Context, data []byte, filename string, contentType string) (string, error)
	Download(ctx context.Context, urlOrKey string) ([]byte, error)
	Delete(ctx context.Context, urlOrKey string) error
	Exists(ctx context.Context, urlOrKey string) (bool, error)
}
