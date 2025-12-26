package topic

import (
	"context"
	"database/sql"
	"errors"

	"github.com/lib/pq"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/domain/ports"
)

type TopicRepository struct {
	db  *sql.DB
	log ports.Logger
}

func NewTopicRepository(db *sql.DB, log ports.Logger) ports.TopicRepository {
	return &TopicRepository{
		db:  db,
		log: log,
	}
}

func (r *TopicRepository) FindByIDWithDeliveries(ctx context.Context, topicID string) (*ports.TopicInfo, error) {
	query := `
		SELECT t.id, t."onlyAdmin", COALESCE(array_agg(tod."deliveryId") FILTER (WHERE tod."deliveryId" IS NOT NULL), '{}')
		FROM "Topic" t
		LEFT JOIN "TopicOnDelivery" tod ON tod."topicId" = t.id
		WHERE t.id = $1
		GROUP BY t.id, t."onlyAdmin"
	`

	var topic ports.TopicInfo
	var deliveryIDs pq.StringArray

	err := r.db.QueryRowContext(ctx, query, topicID).Scan(&topic.ID, &topic.OnlyAdmin, &deliveryIDs)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		r.log.Error("Error finding topic: " + err.Error())
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error finding topic",
		}
	}

	topic.DeliveryIDs = []string(deliveryIDs)
	return &topic, nil
}

