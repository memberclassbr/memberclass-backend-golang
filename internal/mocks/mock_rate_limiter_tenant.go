package mocks

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/ports"
	"github.com/stretchr/testify/mock"
)

type MockRateLimiterTenant struct {
	mock.Mock
}

type MockRateLimiterTenant_Expecter struct {
	mock *mock.Mock
}

func (_m *MockRateLimiterTenant) EXPECT() *MockRateLimiterTenant_Expecter {
	return &MockRateLimiterTenant_Expecter{mock: &_m.Mock}
}

func (m *MockRateLimiterTenant) CheckLimit(ctx context.Context, tenantID string, endpoint string) (bool, ports.RateLimitInfo, error) {
	args := m.Called(ctx, tenantID, endpoint)
	if args.Get(1) == nil {
		return args.Bool(0), ports.RateLimitInfo{}, args.Error(2)
	}
	return args.Bool(0), args.Get(1).(ports.RateLimitInfo), args.Error(2)
}

type MockRateLimiterTenant_CheckLimit_Call struct {
	*mock.Call
}

func (_e *MockRateLimiterTenant_Expecter) CheckLimit(ctx interface{}, tenantID interface{}, endpoint interface{}) *MockRateLimiterTenant_CheckLimit_Call {
	return &MockRateLimiterTenant_CheckLimit_Call{Call: _e.mock.On("CheckLimit", ctx, tenantID, endpoint)}
}

func (_c *MockRateLimiterTenant_CheckLimit_Call) Return(_a0 bool, _a1 ports.RateLimitInfo, _a2 error) *MockRateLimiterTenant_CheckLimit_Call {
	_c.Call.Return(_a0, _a1, _a2)
	return _c
}

func (m *MockRateLimiterTenant) Increment(ctx context.Context, tenantID string, endpoint string) error {
	args := m.Called(ctx, tenantID, endpoint)
	return args.Error(0)
}

type MockRateLimiterTenant_Increment_Call struct {
	*mock.Call
}

func (_e *MockRateLimiterTenant_Expecter) Increment(ctx interface{}, tenantID interface{}, endpoint interface{}) *MockRateLimiterTenant_Increment_Call {
	return &MockRateLimiterTenant_Increment_Call{Call: _e.mock.On("Increment", ctx, tenantID, endpoint)}
}

func (_c *MockRateLimiterTenant_Increment_Call) Return(_a0 error) *MockRateLimiterTenant_Increment_Call {
	_c.Call.Return(_a0)
	return _c
}

func NewMockRateLimiterTenant(t interface {
	mock.TestingT
	Cleanup(func())
}) *MockRateLimiterTenant {
	mock := &MockRateLimiterTenant{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}

