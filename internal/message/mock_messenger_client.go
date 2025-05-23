// Code generated by mockery v2.40.1. DO NOT EDIT.

package message

import (
	context "context"

	mock "github.com/stretchr/testify/mock"
)

// MessengerClientMock is an autogenerated mock type for the MessengerClient type
type MessengerClientMock struct {
	mock.Mock
}

// MessengerType provides a mock function with given fields:
func (_m *MessengerClientMock) MessengerType() MessengerType {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for MessengerType")
	}

	var r0 MessengerType
	if rf, ok := ret.Get(0).(func() MessengerType); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(MessengerType)
	}

	return r0
}

// SendMessage provides a mock function with given fields: ctx, message
func (_m *MessengerClientMock) SendMessage(ctx context.Context, message Message) error {
	ret := _m.Called(ctx, message)

	if len(ret) == 0 {
		panic("no return value specified for SendMessage")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, Message) error); ok {
		r0 = rf(ctx, message)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// NewMessengerClientMock creates a new instance of MessengerClientMock. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewMessengerClientMock(t interface {
	mock.TestingT
	Cleanup(func())
}) *MessengerClientMock {
	mock := &MessengerClientMock{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
