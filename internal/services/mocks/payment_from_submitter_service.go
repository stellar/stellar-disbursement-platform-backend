package mocks

import (
	"context"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/events/schemas"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stretchr/testify/mock"
)

type MockPaymentFromSubmitterService struct {
	mock.Mock
}

var _ services.PaymentFromSubmitterServiceInterface = new(MockPaymentFromSubmitterService)

func (s *MockPaymentFromSubmitterService) SyncTransaction(ctx context.Context, tx *schemas.EventPaymentCompletedData) error {
	args := s.Called(ctx, tx)
	return args.Error(0)
}

func (s *MockPaymentFromSubmitterService) SyncBatchTransactions(ctx context.Context, batchSize int) error {
	args := s.Called(ctx, batchSize)
	return args.Error(0)
}
