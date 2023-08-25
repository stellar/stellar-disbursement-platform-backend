package mocks

import (
	"context"

	anchorplatform "github.com/stellar/stellar-disbursement-platform-backend/internal/anchorplatform"
	"github.com/stretchr/testify/mock"
)

type AnchorPlatformAPIServiceMock struct {
	mock.Mock
}

func (a *AnchorPlatformAPIServiceMock) UpdateAnchorTransactions(ctx context.Context, transactions []anchorplatform.Transaction) error {
	args := a.Called(ctx, transactions)
	return args.Error(0)
}

func (a *AnchorPlatformAPIServiceMock) IsAnchorProtectedByAuth(ctx context.Context) (bool, error) {
	args := a.Called(ctx)
	return args.Bool(0), args.Error(1)
}

var _ anchorplatform.AnchorPlatformAPIServiceInterface = (*AnchorPlatformAPIServiceMock)(nil)
