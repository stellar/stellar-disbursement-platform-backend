package mocks

import (
	"context"

	services "github.com/stellar/stellar-disbursement-platform-backend/internal/services"
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

// Making sure that ServerService implements ServerServiceInterface:
var _ services.PaymentToSubmitterServiceInterface = (*MockPaymentToSubmitterService)(nil)
