package mocks

import (
	"context"

	"github.com/stretchr/testify/mock"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/events/schemas"
)

// MockPaymentToSubmitterService mocks PaymentToSubmitterService.
type MockPaymentToSubmitterService struct {
	mock.Mock
}

func (m *MockPaymentToSubmitterService) SendBatchPayments(ctx context.Context, batchSize int) error {
	args := m.Called(ctx, batchSize)
	return args.Error(0)
}

func (m *MockPaymentToSubmitterService) SendPaymentsReadyToPay(ctx context.Context, paymentsReadyToPay schemas.EventPaymentsReadyToPayData) error {
	args := m.Called(ctx, paymentsReadyToPay)
	return args.Error(0)
}
