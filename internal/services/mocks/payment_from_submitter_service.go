package mocks

import (
	"context"

	"github.com/stretchr/testify/mock"
)

type MockPaymentFromSubmitterService struct {
	mock.Mock
}

func (s *MockPaymentFromSubmitterService) SyncBatchTransactions(ctx context.Context, batchSize int, tenantID string) error {
	args := s.Called(ctx, batchSize, tenantID)
	return args.Error(0)
}
