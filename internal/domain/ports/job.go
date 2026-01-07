package ports

import "context"

type Job interface {
	Execute(ctx context.Context) error
	Name() string
}

