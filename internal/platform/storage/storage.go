// Package storage provides object storage on DigitalOcean Spaces (S3 API).
//
// Each deployment owns exactly one bucket, configured via DO_SPACES_BUCKET.
package storage

import "context"

// Storage is the contract satisfied by the Spaces implementation in this
// package.
type Storage interface {
	Upload(ctx context.Context, data []byte, filename string, contentType string) (string, error)
	Download(ctx context.Context, urlOrKey string) ([]byte, error)
	Delete(ctx context.Context, urlOrKey string) error
	Exists(ctx context.Context, urlOrKey string) (bool, error)
}
