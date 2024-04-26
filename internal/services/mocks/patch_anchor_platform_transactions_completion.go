package mocks

import (
	"context"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/events/schemas"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stretchr/testify/mock"
)

type MockPatchAnchorPlatformTransactionCompletionService struct {
	mock.Mock
}

var _ services.PatchAnchorPlatformTransactionCompletionServiceInterface = new(MockPatchAnchorPlatformTransactionCompletionService)

func (s *MockPatchAnchorPlatformTransactionCompletionService) PatchAPTransactionsForPayments(ctx context.Context) error {
	args := s.Called(ctx)
	return args.Error(0)
}

func (s *MockPatchAnchorPlatformTransactionCompletionService) PatchAPTransactionForPaymentEvent(ctx context.Context, tx schemas.EventPaymentCompletedData) error {
	args := s.Called(ctx, tx)
	return args.Error(0)
}
