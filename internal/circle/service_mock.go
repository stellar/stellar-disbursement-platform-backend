// Code generated by mockery v2.40.1. DO NOT EDIT.

package circle

import (
	context "context"

	mock "github.com/stretchr/testify/mock"
)

// MockService is an autogenerated mock type for the ServiceInterface type
type MockService struct {
	mock.Mock
}

// GetAccountConfiguration provides a mock function with given fields: ctx
func (_m *MockService) GetAccountConfiguration(ctx context.Context) (*AccountConfiguration, error) {
	ret := _m.Called(ctx)

	if len(ret) == 0 {
		panic("no return value specified for GetAccountConfiguration")
	}

	var r0 *AccountConfiguration
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context) (*AccountConfiguration, error)); ok {
		return rf(ctx)
	}
	if rf, ok := ret.Get(0).(func(context.Context) *AccountConfiguration); ok {
		r0 = rf(ctx)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*AccountConfiguration)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context) error); ok {
		r1 = rf(ctx)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetBusinessBalances provides a mock function with given fields: ctx
func (_m *MockService) GetBusinessBalances(ctx context.Context) (*Balances, error) {
	ret := _m.Called(ctx)

	if len(ret) == 0 {
		panic("no return value specified for GetBusinessBalances")
	}

	var r0 *Balances
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context) (*Balances, error)); ok {
		return rf(ctx)
	}
	if rf, ok := ret.Get(0).(func(context.Context) *Balances); ok {
		r0 = rf(ctx)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*Balances)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context) error); ok {
		r1 = rf(ctx)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetPayoutByID provides a mock function with given fields: ctx, id
func (_m *MockService) GetPayoutByID(ctx context.Context, id string) (*Payout, error) {
	ret := _m.Called(ctx, id)

	if len(ret) == 0 {
		panic("no return value specified for GetPayoutByID")
	}

	var r0 *Payout
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, string) (*Payout, error)); ok {
		return rf(ctx, id)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string) *Payout); ok {
		r0 = rf(ctx, id)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*Payout)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, string) error); ok {
		r1 = rf(ctx, id)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetRecipientByID provides a mock function with given fields: ctx, id
func (_m *MockService) GetRecipientByID(ctx context.Context, id string) (*Recipient, error) {
	ret := _m.Called(ctx, id)

	if len(ret) == 0 {
		panic("no return value specified for GetRecipientByID")
	}

	var r0 *Recipient
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, string) (*Recipient, error)); ok {
		return rf(ctx, id)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string) *Recipient); ok {
		r0 = rf(ctx, id)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*Recipient)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, string) error); ok {
		r1 = rf(ctx, id)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetTransferByID provides a mock function with given fields: ctx, id
func (_m *MockService) GetTransferByID(ctx context.Context, id string) (*Transfer, error) {
	ret := _m.Called(ctx, id)

	if len(ret) == 0 {
		panic("no return value specified for GetTransferByID")
	}

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

// Ping provides a mock function with given fields: ctx
func (_m *MockService) Ping(ctx context.Context) (bool, error) {
	ret := _m.Called(ctx)

	if len(ret) == 0 {
		panic("no return value specified for Ping")
	}

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

// PostPayout provides a mock function with given fields: ctx, payoutRequest
func (_m *MockService) PostPayout(ctx context.Context, payoutRequest PayoutRequest) (*Payout, error) {
	ret := _m.Called(ctx, payoutRequest)

	if len(ret) == 0 {
		panic("no return value specified for PostPayout")
	}

	var r0 *Payout
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, PayoutRequest) (*Payout, error)); ok {
		return rf(ctx, payoutRequest)
	}
	if rf, ok := ret.Get(0).(func(context.Context, PayoutRequest) *Payout); ok {
		r0 = rf(ctx, payoutRequest)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*Payout)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, PayoutRequest) error); ok {
		r1 = rf(ctx, payoutRequest)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// PostRecipient provides a mock function with given fields: ctx, recipientRequest
func (_m *MockService) PostRecipient(ctx context.Context, recipientRequest RecipientRequest) (*Recipient, error) {
	ret := _m.Called(ctx, recipientRequest)

	if len(ret) == 0 {
		panic("no return value specified for PostRecipient")
	}

	var r0 *Recipient
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, RecipientRequest) (*Recipient, error)); ok {
		return rf(ctx, recipientRequest)
	}
	if rf, ok := ret.Get(0).(func(context.Context, RecipientRequest) *Recipient); ok {
		r0 = rf(ctx, recipientRequest)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*Recipient)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, RecipientRequest) error); ok {
		r1 = rf(ctx, recipientRequest)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// PostTransfer provides a mock function with given fields: ctx, transferRequest
func (_m *MockService) PostTransfer(ctx context.Context, transferRequest TransferRequest) (*Transfer, error) {
	ret := _m.Called(ctx, transferRequest)

	if len(ret) == 0 {
		panic("no return value specified for PostTransfer")
	}

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

// SendPayout provides a mock function with given fields: ctx, paymentRequest
func (_m *MockService) SendPayout(ctx context.Context, paymentRequest PaymentRequest) (*Payout, error) {
	ret := _m.Called(ctx, paymentRequest)

	if len(ret) == 0 {
		panic("no return value specified for SendPayout")
	}

	var r0 *Payout
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, PaymentRequest) (*Payout, error)); ok {
		return rf(ctx, paymentRequest)
	}
	if rf, ok := ret.Get(0).(func(context.Context, PaymentRequest) *Payout); ok {
		r0 = rf(ctx, paymentRequest)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*Payout)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, PaymentRequest) error); ok {
		r1 = rf(ctx, paymentRequest)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// SendTransfer provides a mock function with given fields: ctx, paymentRequest
func (_m *MockService) SendTransfer(ctx context.Context, paymentRequest PaymentRequest) (*Transfer, error) {
	ret := _m.Called(ctx, paymentRequest)

	if len(ret) == 0 {
		panic("no return value specified for SendTransfer")
	}

	var r0 *Transfer
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, PaymentRequest) (*Transfer, error)); ok {
		return rf(ctx, paymentRequest)
	}
	if rf, ok := ret.Get(0).(func(context.Context, PaymentRequest) *Transfer); ok {
		r0 = rf(ctx, paymentRequest)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*Transfer)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, PaymentRequest) error); ok {
		r1 = rf(ctx, paymentRequest)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// NewMockService creates a new instance of MockService. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewMockService(t interface {
	mock.TestingT
	Cleanup(func())
}) *MockService {
	mock := &MockService{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
