// Code generated by mockery v2.27.1. DO NOT EDIT.

package mocks

import (
	context "context"

	data "github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	mock "github.com/stretchr/testify/mock"

	schema "github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

// MockDistributionAccountService is an autogenerated mock type for the DistributionAccountServiceInterface type
type MockDistributionAccountService struct {
	mock.Mock
}

// GetBalance provides a mock function with given fields: ctx, account, asset
func (_m *MockDistributionAccountService) GetBalance(ctx context.Context, account *schema.DistributionAccount, asset data.Asset) (float64, error) {
	ret := _m.Called(ctx, account, asset)

	var r0 float64
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *schema.DistributionAccount, data.Asset) (float64, error)); ok {
		return rf(ctx, account, asset)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *schema.DistributionAccount, data.Asset) float64); ok {
		r0 = rf(ctx, account, asset)
	} else {
		r0 = ret.Get(0).(float64)
	}

	if rf, ok := ret.Get(1).(func(context.Context, *schema.DistributionAccount, data.Asset) error); ok {
		r1 = rf(ctx, account, asset)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetBalances provides a mock function with given fields: ctx, account
func (_m *MockDistributionAccountService) GetBalances(ctx context.Context, account *schema.DistributionAccount) (map[string]float64, error) {
	ret := _m.Called(ctx, account)

	var r0 map[string]float64
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *schema.DistributionAccount) (map[string]float64, error)); ok {
		return rf(ctx, account)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *schema.DistributionAccount) map[string]float64); ok {
		r0 = rf(ctx, account)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(map[string]float64)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *schema.DistributionAccount) error); ok {
		r1 = rf(ctx, account)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

type mockConstructorTestingTNewMockDistributionAccountService interface {
	mock.TestingT
	Cleanup(func())
}

// NewMockDistributionAccountService creates a new instance of MockDistributionAccountService. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
func NewMockDistributionAccountService(t mockConstructorTestingTNewMockDistributionAccountService) *MockDistributionAccountService {
	mock := &MockDistributionAccountService{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}