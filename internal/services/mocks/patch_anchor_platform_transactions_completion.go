package mocks

import (
	"context"

	"github.com/stretchr/testify/mock"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/events/schemas"
)

type MockPatchAnchorPlatformTransactionCompletionService struct {
	mock.Mock
}

func (s *MockPatchAnchorPlatformTransactionCompletionService) PatchAPTransactionsForPayments(ctx context.Context) error {
	args := s.Called(ctx)
	return args.Error(0)
}

func (s *MockPatchAnchorPlatformTransactionCompletionService) PatchAPTransactionForPaymentEvent(ctx context.Context, tx schemas.EventPaymentCompletedData) error {
	args := s.Called(ctx, tx)
	return args.Error(0)
}
