package mocks

import (
	"context"

	"github.com/memberclass-backend-golang/internal/domain/ports"
	"github.com/stretchr/testify/mock"
)

type MockRateLimiterIP struct {
	mock.Mock
}

type MockRateLimiterIP_Expecter struct {
	mock *mock.Mock
}

func (_m *MockRateLimiterIP) EXPECT() *MockRateLimiterIP_Expecter {
	return &MockRateLimiterIP_Expecter{mock: &_m.Mock}
}

func (m *MockRateLimiterIP) CheckLimit(ctx context.Context, ip string) (bool, ports.RateLimitInfo, error) {
	args := m.Called(ctx, ip)
	if args.Get(1) == nil {
		return args.Bool(0), ports.RateLimitInfo{}, args.Error(2)
	}
	return args.Bool(0), args.Get(1).(ports.RateLimitInfo), args.Error(2)
}

type MockRateLimiterIP_CheckLimit_Call struct {
	*mock.Call
}

func (_e *MockRateLimiterIP_Expecter) CheckLimit(ctx interface{}, ip interface{}) *MockRateLimiterIP_CheckLimit_Call {
	return &MockRateLimiterIP_CheckLimit_Call{Call: _e.mock.On("CheckLimit", ctx, ip)}
}

func (_c *MockRateLimiterIP_CheckLimit_Call) Return(_a0 bool, _a1 ports.RateLimitInfo, _a2 error) *MockRateLimiterIP_CheckLimit_Call {
	_c.Call.Return(_a0, _a1, _a2)
	return _c
}

func (m *MockRateLimiterIP) Increment(ctx context.Context, ip string) error {
	args := m.Called(ctx, ip)
	return args.Error(0)
}

type MockRateLimiterIP_Increment_Call struct {
	*mock.Call
}

func (_e *MockRateLimiterIP_Expecter) Increment(ctx interface{}, ip interface{}) *MockRateLimiterIP_Increment_Call {
	return &MockRateLimiterIP_Increment_Call{Call: _e.mock.On("Increment", ctx, ip)}
}

func (_c *MockRateLimiterIP_Increment_Call) Return(_a0 error) *MockRateLimiterIP_Increment_Call {
	_c.Call.Return(_a0)
	return _c
}

func NewMockRateLimiterIP(t interface {
	mock.TestingT
	Cleanup(func())
}) *MockRateLimiterIP {
	mock := &MockRateLimiterIP{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}

