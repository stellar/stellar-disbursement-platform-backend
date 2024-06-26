// Code generated by mockery v2.40.1. DO NOT EDIT.

package mocks

import (
	context "context"

	mock "github.com/stretchr/testify/mock"

	txnbuild "github.com/stellar/go/txnbuild"
)

// MockSignatureClient is an autogenerated mock type for the SignatureClient type
type MockSignatureClient struct {
	mock.Mock
}

// BatchInsert provides a mock function with given fields: ctx, number
func (_m *MockSignatureClient) BatchInsert(ctx context.Context, number int) ([]string, error) {
	ret := _m.Called(ctx, number)

	if len(ret) == 0 {
		panic("no return value specified for BatchInsert")
	}

	var r0 []string
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, int) ([]string, error)); ok {
		return rf(ctx, number)
	}
	if rf, ok := ret.Get(0).(func(context.Context, int) []string); ok {
		r0 = rf(ctx, number)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]string)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, int) error); ok {
		r1 = rf(ctx, number)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Delete provides a mock function with given fields: ctx, publicKey
func (_m *MockSignatureClient) Delete(ctx context.Context, publicKey string) error {
	ret := _m.Called(ctx, publicKey)

	if len(ret) == 0 {
		panic("no return value specified for Delete")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, string) error); ok {
		r0 = rf(ctx, publicKey)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// NetworkPassphrase provides a mock function with given fields:
func (_m *MockSignatureClient) NetworkPassphrase() string {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for NetworkPassphrase")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// SignFeeBumpStellarTransaction provides a mock function with given fields: ctx, feeBumpStellarTx, stellarAccounts
func (_m *MockSignatureClient) SignFeeBumpStellarTransaction(ctx context.Context, feeBumpStellarTx *txnbuild.FeeBumpTransaction, stellarAccounts ...string) (*txnbuild.FeeBumpTransaction, error) {
	_va := make([]interface{}, len(stellarAccounts))
	for _i := range stellarAccounts {
		_va[_i] = stellarAccounts[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, feeBumpStellarTx)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	if len(ret) == 0 {
		panic("no return value specified for SignFeeBumpStellarTransaction")
	}

	var r0 *txnbuild.FeeBumpTransaction
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *txnbuild.FeeBumpTransaction, ...string) (*txnbuild.FeeBumpTransaction, error)); ok {
		return rf(ctx, feeBumpStellarTx, stellarAccounts...)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *txnbuild.FeeBumpTransaction, ...string) *txnbuild.FeeBumpTransaction); ok {
		r0 = rf(ctx, feeBumpStellarTx, stellarAccounts...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*txnbuild.FeeBumpTransaction)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *txnbuild.FeeBumpTransaction, ...string) error); ok {
		r1 = rf(ctx, feeBumpStellarTx, stellarAccounts...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// SignStellarTransaction provides a mock function with given fields: ctx, stellarTx, stellarAccounts
func (_m *MockSignatureClient) SignStellarTransaction(ctx context.Context, stellarTx *txnbuild.Transaction, stellarAccounts ...string) (*txnbuild.Transaction, error) {
	_va := make([]interface{}, len(stellarAccounts))
	for _i := range stellarAccounts {
		_va[_i] = stellarAccounts[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, stellarTx)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	if len(ret) == 0 {
		panic("no return value specified for SignStellarTransaction")
	}

	var r0 *txnbuild.Transaction
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *txnbuild.Transaction, ...string) (*txnbuild.Transaction, error)); ok {
		return rf(ctx, stellarTx, stellarAccounts...)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *txnbuild.Transaction, ...string) *txnbuild.Transaction); ok {
		r0 = rf(ctx, stellarTx, stellarAccounts...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*txnbuild.Transaction)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *txnbuild.Transaction, ...string) error); ok {
		r1 = rf(ctx, stellarTx, stellarAccounts...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// NewMockSignatureClient creates a new instance of MockSignatureClient. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewMockSignatureClient(t interface {
	mock.TestingT
	Cleanup(func())
}) *MockSignatureClient {
	mock := &MockSignatureClient{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
