package mocks

import (
	"context"
	"time"

	"github.com/memberclass-backend-golang/internal/domain/dto/response"
	"github.com/stretchr/testify/mock"
)

type MockSSORepository struct {
	mock.Mock
}

func (m *MockSSORepository) UpdateSSOToken(ctx context.Context, userID, tenantID, tokenHash string, validUntil time.Time) error {
	args := m.Called(ctx, userID, tenantID, tokenHash, validUntil)
	return args.Error(0)
}

func (m *MockSSORepository) ValidateAndConsumeSSOToken(ctx context.Context, tokenHash, ip string) (*response.ValidateSSOTokenResponse, error) {
	args := m.Called(ctx, tokenHash, ip)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*response.ValidateSSOTokenResponse), args.Error(1)
}

func (m *MockSSORepository) GetUserDocument(ctx context.Context, userID string) (*string, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*string), args.Error(1)
}
