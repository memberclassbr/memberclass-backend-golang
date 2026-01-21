package topic

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/domain/ports/topic"
	"github.com/memberclass-backend-golang/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestTopicRepository_FindByIDWithDeliveries(t *testing.T) {
	tests := []struct {
		name          string
		topicID       string
		mockSetup     func(sqlmock.Sqlmock)
		expectedError error
		expectedTopic *topic.TopicInfo
	}{
		{
			name:    "should return topic with deliveries when found",
			topicID: "topic-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				deliveryIDs := pq.StringArray{"delivery-1", "delivery-2", "delivery-3"}
				rows := sqlmock.NewRows([]string{"id", "onlyAdmin", "array_agg"}).
					AddRow("topic-123", true, deliveryIDs)
				sqlMock.ExpectQuery(`SELECT t.id, t."onlyAdmin", COALESCE\(array_agg\(tod."deliveryId"\) FILTER \(WHERE tod."deliveryId" IS NOT NULL\), '\{\}'\)`).
					WithArgs("topic-123").
					WillReturnRows(rows)
			},
			expectedError: nil,
			expectedTopic: &topic.TopicInfo{
				ID:          "topic-123",
				OnlyAdmin:   true,
				DeliveryIDs: []string{"delivery-1", "delivery-2", "delivery-3"},
			},
		},
		{
			name:    "should return topic without deliveries when found",
			topicID: "topic-456",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				deliveryIDs := pq.StringArray{}
				rows := sqlmock.NewRows([]string{"id", "onlyAdmin", "array_agg"}).
					AddRow("topic-456", false, deliveryIDs)
				sqlMock.ExpectQuery(`SELECT t.id, t."onlyAdmin", COALESCE\(array_agg\(tod."deliveryId"\) FILTER \(WHERE tod."deliveryId" IS NOT NULL\), '\{\}'\)`).
					WithArgs("topic-456").
					WillReturnRows(rows)
			},
			expectedError: nil,
			expectedTopic: &topic.TopicInfo{
				ID:          "topic-456",
				OnlyAdmin:   false,
				DeliveryIDs: []string{},
			},
		},
		{
			name:    "should return nil when topic does not exist",
			topicID: "non-existent",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				sqlMock.ExpectQuery(`SELECT t.id, t."onlyAdmin", COALESCE\(array_agg\(tod."deliveryId"\) FILTER \(WHERE tod."deliveryId" IS NOT NULL\), '\{\}'\)`).
					WithArgs("non-existent").
					WillReturnError(sql.ErrNoRows)
			},
			expectedError: nil,
			expectedTopic: nil,
		},
		{
			name:    "should return MemberClassError when database error occurs",
			topicID: "topic-123",
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				sqlMock.ExpectQuery(`SELECT t.id, t."onlyAdmin", COALESCE\(array_agg\(tod."deliveryId"\) FILTER \(WHERE tod."deliveryId" IS NOT NULL\), '\{\}'\)`).
					WithArgs("topic-123").
					WillReturnError(errors.New("database connection error"))
			},
			expectedError: &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "error finding topic",
			},
			expectedTopic: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, sqlMock, err := sqlmock.New()
			assert.NoError(t, err)
			defer db.Close()

			mockLogger := mocks.NewMockLogger(t)
			if tt.expectedError != nil {
				mockLogger.EXPECT().Error(mock.Anything).Return()
			}

			repository := NewTopicRepository(db, mockLogger)
			tt.mockSetup(sqlMock)

			result, err := repository.FindByIDWithDeliveries(context.Background(), tt.topicID)

			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedError, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				if tt.expectedTopic == nil {
					assert.Nil(t, result)
				} else {
					assert.NotNil(t, result)
					assert.Equal(t, tt.expectedTopic.ID, result.ID)
					assert.Equal(t, tt.expectedTopic.OnlyAdmin, result.OnlyAdmin)
					assert.Equal(t, tt.expectedTopic.DeliveryIDs, result.DeliveryIDs)
				}
			}
			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})
	}
}

func TestNewTopicRepository(t *testing.T) {
	t.Run("should create new topic repository instance", func(t *testing.T) {
		db, _, err := sqlmock.New()
		assert.NoError(t, err)
		defer db.Close()

		mockLogger := mocks.NewMockLogger(t)
		repository := NewTopicRepository(db, mockLogger)

		assert.NotNil(t, repository)
	})
}
