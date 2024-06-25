// Code generated by mockery v2.27.1. DO NOT EDIT.

package circle

import (
	context "context"

	mock "github.com/stretchr/testify/mock"
)

// MockClient is an autogenerated mock type for the ClientInterface type
type MockClient struct {
	mock.Mock
}

// GetTransferByID provides a mock function with given fields: ctx, id
func (_m *MockClient) GetTransferByID(ctx context.Context, id string) (*Transfer, error) {
	ret := _m.Called(ctx, id)

	var r0 *Transfer
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, string) (*Transfer, error)); ok {
		return rf(ctx, id)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string) *Transfer); ok {
		r0 = rf(ctx, id)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*Transfer)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, string) error); ok {
		r1 = rf(ctx, id)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetWalletByID provides a mock function with given fields: ctx, id
func (_m *MockClient) GetWalletByID(ctx context.Context, id string) (*Wallet, error) {
	ret := _m.Called(ctx, id)

	var r0 *Wallet
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, string) (*Wallet, error)); ok {
		return rf(ctx, id)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string) *Wallet); ok {
		r0 = rf(ctx, id)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*Wallet)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, string) error); ok {
		r1 = rf(ctx, id)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Ping provides a mock function with given fields: ctx
func (_m *MockClient) Ping(ctx context.Context) (bool, error) {
	ret := _m.Called(ctx)

	var r0 bool
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context) (bool, error)); ok {
		return rf(ctx)
	}
	if rf, ok := ret.Get(0).(func(context.Context) bool); ok {
		r0 = rf(ctx)
	} else {
		r0 = ret.Get(0).(bool)
	}

	if rf, ok := ret.Get(1).(func(context.Context) error); ok {
		r1 = rf(ctx)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// PostTransfer provides a mock function with given fields: ctx, transferRequest
func (_m *MockClient) PostTransfer(ctx context.Context, transferRequest TransferRequest) (*Transfer, error) {
	ret := _m.Called(ctx, transferRequest)

	var r0 *Transfer
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, TransferRequest) (*Transfer, error)); ok {
		return rf(ctx, transferRequest)
	}
	if rf, ok := ret.Get(0).(func(context.Context, TransferRequest) *Transfer); ok {
		r0 = rf(ctx, transferRequest)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*Transfer)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, TransferRequest) error); ok {
		r1 = rf(ctx, transferRequest)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

type mockConstructorTestingTNewMockClient interface {
	mock.TestingT
	Cleanup(func())
}

// NewMockClient creates a new instance of MockClient. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
func NewMockClient(t mockConstructorTestingTNewMockClient) *MockClient {
	mock := &MockClient{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
