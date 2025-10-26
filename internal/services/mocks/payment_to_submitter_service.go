package mocks

import (
	"context"

	"github.com/stretchr/testify/mock"
)

// MockPaymentToSubmitterService mocks PaymentToSubmitterService.
type MockPaymentToSubmitterService struct {
	mock.Mock
}

func (m *MockPaymentToSubmitterService) SendBatchPayments(ctx context.Context, batchSize int) error {
	args := m.Called(ctx, batchSize)
	return args.Error(0)
}
