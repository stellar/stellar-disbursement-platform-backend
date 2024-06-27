// Code generated by mockery v2.43.2. DO NOT EDIT.

package mocks

import (
	context "context"

	mock "github.com/stretchr/testify/mock"
)

// MockCircleReconciliationService is an autogenerated mock type for the CircleReconciliationServiceInterface type
type MockCircleReconciliationService struct {
	mock.Mock
}

// Reconcile provides a mock function with given fields: ctx
func (_m *MockCircleReconciliationService) Reconcile(ctx context.Context) error {
	ret := _m.Called(ctx)

	if len(ret) == 0 {
		panic("no return value specified for Reconcile")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context) error); ok {
		r0 = rf(ctx)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// NewMockCircleReconciliationService creates a new instance of MockCircleReconciliationService. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewMockCircleReconciliationService(t interface {
	mock.TestingT
	Cleanup(func())
}) *MockCircleReconciliationService {
	mock := &MockCircleReconciliationService{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
