package mocks

import (
	"context"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/events/schemas"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stretchr/testify/mock"
)

// MockPaymentToSubmitterService mocks PaymentToSubmitterService.
type MockPaymentToSubmitterService struct {
	mock.Mock
}

func (m *MockPaymentToSubmitterService) SendPaymentsReadyToPay(ctx context.Context, paymentsReadyToPay *schemas.EventPaymentsReadyToPayData) error {
	args := m.Called(ctx, paymentsReadyToPay)
	return args.Error(0)
}

// Making sure that ServerService implements ServerServiceInterface:
var _ services.PaymentToSubmitterServiceInterface = (*MockPaymentToSubmitterService)(nil)
