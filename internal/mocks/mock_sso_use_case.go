package mocks

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/dto/request/sso"
	"github.com/memberclass-backend-golang/internal/domain/dto/response"
	"github.com/stretchr/testify/mock"
)

type MockSSOUseCase struct {
	mock.Mock
}

func (m *MockSSOUseCase) GenerateSSOToken(ctx context.Context, req sso.GenerateSSOTokenRequest, externalURL string) (*response.GenerateSSOTokenResponse, error) {
	args := m.Called(ctx, req, externalURL)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*response.GenerateSSOTokenResponse), args.Error(1)
}

func (m *MockSSOUseCase) ValidateSSOToken(ctx context.Context, token, ip string) (*response.ValidateSSOTokenResponse, error) {
	args := m.Called(ctx, token, ip)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*response.ValidateSSOTokenResponse), args.Error(1)
}
