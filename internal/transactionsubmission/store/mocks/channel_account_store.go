// Code generated by mockery v2.40.1. DO NOT EDIT.

package mocks

import (
	context "context"

	db "github.com/stellar/stellar-disbursement-platform-backend/db"
	mock "github.com/stretchr/testify/mock"

	store "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
)

// MockChannelAccountStore is an autogenerated mock type for the ChannelAccountStore type
type MockChannelAccountStore struct {
	mock.Mock
}

// BatchInsert provides a mock function with given fields: ctx, sqlExec, channelAccounts
func (_m *MockChannelAccountStore) BatchInsert(ctx context.Context, sqlExec db.SQLExecuter, channelAccounts []*store.ChannelAccount) error {
	ret := _m.Called(ctx, sqlExec, channelAccounts)

	if len(ret) == 0 {
		panic("no return value specified for BatchInsert")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, db.SQLExecuter, []*store.ChannelAccount) error); ok {
		r0 = rf(ctx, sqlExec, channelAccounts)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// BatchInsertAndLock provides a mock function with given fields: ctx, channelAccounts, currentLedger, nextLedgerLock
func (_m *MockChannelAccountStore) BatchInsertAndLock(ctx context.Context, channelAccounts []*store.ChannelAccount, currentLedger int, nextLedgerLock int) error {
	ret := _m.Called(ctx, channelAccounts, currentLedger, nextLedgerLock)

	if len(ret) == 0 {
		panic("no return value specified for BatchInsertAndLock")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, []*store.ChannelAccount, int, int) error); ok {
		r0 = rf(ctx, channelAccounts, currentLedger, nextLedgerLock)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Count provides a mock function with given fields: ctx
func (_m *MockChannelAccountStore) Count(ctx context.Context) (int, error) {
	ret := _m.Called(ctx)

	if len(ret) == 0 {
		panic("no return value specified for Count")
	}

	var r0 int
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context) (int, error)); ok {
		return rf(ctx)
	}
	if rf, ok := ret.Get(0).(func(context.Context) int); ok {
		r0 = rf(ctx)
	} else {
		r0 = ret.Get(0).(int)
	}

	if rf, ok := ret.Get(1).(func(context.Context) error); ok {
		r1 = rf(ctx)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Delete provides a mock function with given fields: ctx, sqlExec, publicKey
func (_m *MockChannelAccountStore) Delete(ctx context.Context, sqlExec db.SQLExecuter, publicKey string) error {
	ret := _m.Called(ctx, sqlExec, publicKey)

	if len(ret) == 0 {
		panic("no return value specified for Delete")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, db.SQLExecuter, string) error); ok {
		r0 = rf(ctx, sqlExec, publicKey)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// DeleteIfLockedUntil provides a mock function with given fields: ctx, publicKey, lockedUntilLedgerNumber
func (_m *MockChannelAccountStore) DeleteIfLockedUntil(ctx context.Context, publicKey string, lockedUntilLedgerNumber int) error {
	ret := _m.Called(ctx, publicKey, lockedUntilLedgerNumber)

	if len(ret) == 0 {
		panic("no return value specified for DeleteIfLockedUntil")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, string, int) error); ok {
		r0 = rf(ctx, publicKey, lockedUntilLedgerNumber)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Get provides a mock function with given fields: ctx, sqlExec, publicKey, currentLedgerNumber
func (_m *MockChannelAccountStore) Get(ctx context.Context, sqlExec db.SQLExecuter, publicKey string, currentLedgerNumber int) (*store.ChannelAccount, error) {
	ret := _m.Called(ctx, sqlExec, publicKey, currentLedgerNumber)

	if len(ret) == 0 {
		panic("no return value specified for Get")
	}

	var r0 *store.ChannelAccount
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, db.SQLExecuter, string, int) (*store.ChannelAccount, error)); ok {
		return rf(ctx, sqlExec, publicKey, currentLedgerNumber)
	}
	if rf, ok := ret.Get(0).(func(context.Context, db.SQLExecuter, string, int) *store.ChannelAccount); ok {
		r0 = rf(ctx, sqlExec, publicKey, currentLedgerNumber)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*store.ChannelAccount)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, db.SQLExecuter, string, int) error); ok {
		r1 = rf(ctx, sqlExec, publicKey, currentLedgerNumber)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetAll provides a mock function with given fields: ctx, sqlExec, currentLedger, limit
func (_m *MockChannelAccountStore) GetAll(ctx context.Context, sqlExec db.SQLExecuter, currentLedger int, limit int) ([]*store.ChannelAccount, error) {
	ret := _m.Called(ctx, sqlExec, currentLedger, limit)

	if len(ret) == 0 {
		panic("no return value specified for GetAll")
	}

	var r0 []*store.ChannelAccount
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, db.SQLExecuter, int, int) ([]*store.ChannelAccount, error)); ok {
		return rf(ctx, sqlExec, currentLedger, limit)
	}
	if rf, ok := ret.Get(0).(func(context.Context, db.SQLExecuter, int, int) []*store.ChannelAccount); ok {
		r0 = rf(ctx, sqlExec, currentLedger, limit)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]*store.ChannelAccount)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, db.SQLExecuter, int, int) error); ok {
		r1 = rf(ctx, sqlExec, currentLedger, limit)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetAndLock provides a mock function with given fields: ctx, publicKey, currentLedger, nextLedgerLock
func (_m *MockChannelAccountStore) GetAndLock(ctx context.Context, publicKey string, currentLedger int, nextLedgerLock int) (*store.ChannelAccount, error) {
	ret := _m.Called(ctx, publicKey, currentLedger, nextLedgerLock)

	if len(ret) == 0 {
		panic("no return value specified for GetAndLock")
	}

	var r0 *store.ChannelAccount
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, string, int, int) (*store.ChannelAccount, error)); ok {
		return rf(ctx, publicKey, currentLedger, nextLedgerLock)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string, int, int) *store.ChannelAccount); ok {
		r0 = rf(ctx, publicKey, currentLedger, nextLedgerLock)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*store.ChannelAccount)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, string, int, int) error); ok {
		r1 = rf(ctx, publicKey, currentLedger, nextLedgerLock)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetAndLockAll provides a mock function with given fields: ctx, currentLedger, nextLedgerLock, limit
func (_m *MockChannelAccountStore) GetAndLockAll(ctx context.Context, currentLedger int, nextLedgerLock int, limit int) ([]*store.ChannelAccount, error) {
	ret := _m.Called(ctx, currentLedger, nextLedgerLock, limit)

	if len(ret) == 0 {
		panic("no return value specified for GetAndLockAll")
	}

	var r0 []*store.ChannelAccount
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, int, int, int) ([]*store.ChannelAccount, error)); ok {
		return rf(ctx, currentLedger, nextLedgerLock, limit)
	}
	if rf, ok := ret.Get(0).(func(context.Context, int, int, int) []*store.ChannelAccount); ok {
		r0 = rf(ctx, currentLedger, nextLedgerLock, limit)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]*store.ChannelAccount)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, int, int, int) error); ok {
		r1 = rf(ctx, currentLedger, nextLedgerLock, limit)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Insert provides a mock function with given fields: ctx, sqlExec, publicKey, privateKey
func (_m *MockChannelAccountStore) Insert(ctx context.Context, sqlExec db.SQLExecuter, publicKey string, privateKey string) error {
	ret := _m.Called(ctx, sqlExec, publicKey, privateKey)

	if len(ret) == 0 {
		panic("no return value specified for Insert")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, db.SQLExecuter, string, string) error); ok {
		r0 = rf(ctx, sqlExec, publicKey, privateKey)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// InsertAndLock provides a mock function with given fields: ctx, publicKey, privateKey, currentLedger, nextLedgerLock
func (_m *MockChannelAccountStore) InsertAndLock(ctx context.Context, publicKey string, privateKey string, currentLedger int, nextLedgerLock int) error {
	ret := _m.Called(ctx, publicKey, privateKey, currentLedger, nextLedgerLock)

	if len(ret) == 0 {
		panic("no return value specified for InsertAndLock")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, string, string, int, int) error); ok {
		r0 = rf(ctx, publicKey, privateKey, currentLedger, nextLedgerLock)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Lock provides a mock function with given fields: ctx, sqlExec, publicKey, currentLedger, nextLedgerLock
func (_m *MockChannelAccountStore) Lock(ctx context.Context, sqlExec db.SQLExecuter, publicKey string, currentLedger int32, nextLedgerLock int32) (*store.ChannelAccount, error) {
	ret := _m.Called(ctx, sqlExec, publicKey, currentLedger, nextLedgerLock)

	if len(ret) == 0 {
		panic("no return value specified for Lock")
	}

	var r0 *store.ChannelAccount
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, db.SQLExecuter, string, int32, int32) (*store.ChannelAccount, error)); ok {
		return rf(ctx, sqlExec, publicKey, currentLedger, nextLedgerLock)
	}
	if rf, ok := ret.Get(0).(func(context.Context, db.SQLExecuter, string, int32, int32) *store.ChannelAccount); ok {
		r0 = rf(ctx, sqlExec, publicKey, currentLedger, nextLedgerLock)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*store.ChannelAccount)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, db.SQLExecuter, string, int32, int32) error); ok {
		r1 = rf(ctx, sqlExec, publicKey, currentLedger, nextLedgerLock)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Unlock provides a mock function with given fields: ctx, sqlExec, publicKey
func (_m *MockChannelAccountStore) Unlock(ctx context.Context, sqlExec db.SQLExecuter, publicKey string) (*store.ChannelAccount, error) {
	ret := _m.Called(ctx, sqlExec, publicKey)

	if len(ret) == 0 {
		panic("no return value specified for Unlock")
	}

	var r0 *store.ChannelAccount
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, db.SQLExecuter, string) (*store.ChannelAccount, error)); ok {
		return rf(ctx, sqlExec, publicKey)
	}
	if rf, ok := ret.Get(0).(func(context.Context, db.SQLExecuter, string) *store.ChannelAccount); ok {
		r0 = rf(ctx, sqlExec, publicKey)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*store.ChannelAccount)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, db.SQLExecuter, string) error); ok {
		r1 = rf(ctx, sqlExec, publicKey)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// NewMockChannelAccountStore creates a new instance of MockChannelAccountStore. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewMockChannelAccountStore(t interface {
	mock.TestingT
	Cleanup(func())
}) *MockChannelAccountStore {
	mock := &MockChannelAccountStore{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
