package ports

import (
	"context"
)

type TopicRepository interface {
	FindByIDWithDeliveries(ctx context.Context, topicID string) (*TopicInfo, error)
}

type TopicInfo struct {
	ID          string
	OnlyAdmin   bool
	DeliveryIDs []string
}

