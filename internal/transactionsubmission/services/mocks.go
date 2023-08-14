package services

import (
	"context"

	"github.com/stretchr/testify/mock"
)

type ChannelAccountsServiceMock struct {
	mock.Mock
}

func (cas *ChannelAccountsServiceMock) CreateChannelAccountsOnChain(ctx context.Context, opts ChannelAccountServiceOptions) error {
	args := cas.Called(ctx, opts)
	return args.Error(0)
}

func (cas *ChannelAccountsServiceMock) VerifyChannelAccounts(ctx context.Context, opts ChannelAccountServiceOptions) error {
	args := cas.Called(ctx)
	return args.Error(0)
}

func (cas *ChannelAccountsServiceMock) DeleteChannelAccounts(ctx context.Context) error {
	args := cas.Called(ctx)
	return args.Error(0)
}

func (cas *ChannelAccountsServiceMock) DeleteChannelAccount(ctx context.Context, opts ChannelAccountServiceOptions) error {
	args := cas.Called(ctx, opts)
	return args.Error(0)
}

func (cas *ChannelAccountsServiceMock) EnsureChannelAccountsCount(ctx context.Context, opts ChannelAccountServiceOptions) error {
	args := cas.Called(ctx, opts)
	return args.Error(0)
}

func (cas *ChannelAccountsServiceMock) ViewChannelAccounts(ctx context.Context) error {
	args := cas.Called(ctx)
	return args.Error(0)
}

var _ ChannelAccountsServiceInterface = (*ChannelAccountsServiceMock)(nil)
