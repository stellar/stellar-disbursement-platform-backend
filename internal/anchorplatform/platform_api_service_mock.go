package anchorplatform

import (
	"context"

	"github.com/stretchr/testify/mock"
)

type AnchorPlatformAPIServiceMock struct {
	mock.Mock
}

func (a *AnchorPlatformAPIServiceMock) UpdateAnchorTransactions(ctx context.Context, transactions []Transaction) error {
	args := a.Called(ctx, transactions)
	return args.Error(0)
}

var _ AnchorPlatformAPIServiceInterface = (*AnchorPlatformAPIServiceMock)(nil)
