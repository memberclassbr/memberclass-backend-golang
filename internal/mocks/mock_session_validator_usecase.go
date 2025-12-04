package mocks

import (
	"github.com/stretchr/testify/mock"
)

type MockSessionValidatorUseCase struct {
	mock.Mock
}

func NewMockSessionValidatorUseCase(t interface {
	mock.TestingT
	Cleanup(func())
}) *MockSessionValidatorUseCase {
	m := &MockSessionValidatorUseCase{}
	m.Mock.Test(t)
	t.Cleanup(func() { m.AssertExpectations(t) })
	return m
}

func (m *MockSessionValidatorUseCase) ValidateUserExists(userID string) error {
	args := m.Called(userID)
	return args.Error(0)
}

func (m *MockSessionValidatorUseCase) ValidateUserBelongsToTenant(userID string, tenantID string) error {
	args := m.Called(userID, tenantID)
	return args.Error(0)
}

