package mocks

import (
	"github.com/memberclass-backend-golang/internal/domain/entities"
	"github.com/stretchr/testify/mock"
)

type MockUserRepository struct {
	mock.Mock
}

func NewMockUserRepository(t interface {
	mock.TestingT
	Cleanup(func())
}) *MockUserRepository {
	m := &MockUserRepository{}
	m.Mock.Test(t)
	t.Cleanup(func() { m.AssertExpectations(t) })
	return m
}

func (m *MockUserRepository) FindByID(userID string) (*entities.User, error) {
	args := m.Called(userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entities.User), args.Error(1)
}

func (m *MockUserRepository) ExistsByID(userID string) (bool, error) {
	args := m.Called(userID)
	return args.Bool(0), args.Error(1)
}

func (m *MockUserRepository) BelongsToTenant(userID string, tenantID string) (bool, error) {
	args := m.Called(userID, tenantID)
	return args.Bool(0), args.Error(1)
}

