package mocks

import (
	"context"

	"github.com/stretchr/testify/mock"
)

type MockPatchAnchorPlatformTransactionCompletionService struct {
	mock.Mock
}

func (s *MockPatchAnchorPlatformTransactionCompletionService) PatchAPTransactionsForPayments(ctx context.Context) error {
	args := s.Called(ctx)
	return args.Error(0)
}
