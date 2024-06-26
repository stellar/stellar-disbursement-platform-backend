package mocks

import (
	"context"

	"github.com/stretchr/testify/mock"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/events/schemas"
)

type MockPaymentFromSubmitterService struct {
	mock.Mock
}

func (s *MockPaymentFromSubmitterService) SyncTransaction(ctx context.Context, tx *schemas.EventPaymentCompletedData) error {
	args := s.Called(ctx, tx)
	return args.Error(0)
}

func (s *MockPaymentFromSubmitterService) SyncBatchTransactions(ctx context.Context, batchSize int, tenantID string) error {
	args := s.Called(ctx, batchSize, tenantID)
	return args.Error(0)
}
