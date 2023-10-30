package services

import (
	"context"
	"fmt"

	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
)

type ReadyPaymentsCancellationServiceInterface interface {
	CancelReadyPayments(ctx context.Context) error
}

var _ ReadyPaymentsCancellationServiceInterface = (*ReadyPaymentsCancellationService)(nil)

type ReadyPaymentsCancellationService struct {
	sdpModels *data.Models
}

func NewReadyPaymentsCancellationService(models *data.Models) *ReadyPaymentsCancellationService {
	return &ReadyPaymentsCancellationService{
		sdpModels: models,
	}
}

// CancelReadyPayments cancels SDP's ready-to-pay payments that are older than the specified period.
func (s ReadyPaymentsCancellationService) CancelReadyPayments(ctx context.Context) error {
	organization, err := s.sdpModels.Organizations.Get(ctx)
	if err != nil {
		return fmt.Errorf("error getting organization: %w", err)
	}

	if organization.PaymentCancellationPeriodDays == nil {
		log.Debug("automatic ready payment cancellation is deactivated. Set a valid value to the organization's payment_cancellation_period_days to activate it.")
		return nil
	}

	if err := s.sdpModels.Payment.CancelPaymentsWithinPeriodDays(ctx, s.sdpModels.DBConnectionPool, *organization.PaymentCancellationPeriodDays); err != nil {
		return fmt.Errorf("canceling ready payments after %d days: %w", int(*organization.PaymentCancellationPeriodDays), err)
	}
	return nil
}
