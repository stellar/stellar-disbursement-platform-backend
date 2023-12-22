package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
)

// PaymentManagementService is a service for managing disbursements.
type PaymentManagementService struct {
	models           *data.Models
	dbConnectionPool db.DBConnectionPool
}

var (
	ErrPaymentNotFound            = errors.New("payment not found")
	ErrPaymentNotReadyToCancel    = errors.New("payment is not ready to be canceled")
	ErrPaymentStatusCantBeChanged = errors.New("payment status can't be changed to the requested status")
)

// NewPaymentManagementService is a factory function for creating a new PaymentManagementService.
func NewPaymentManagementService(models *data.Models, dbConnectionPool db.DBConnectionPool) *PaymentManagementService {
	return &PaymentManagementService{
		models:           models,
		dbConnectionPool: dbConnectionPool,
	}
}

// CancelPayment update payment to status 'canceled'
func (s *PaymentManagementService) CancelPayment(ctx context.Context, paymentID string) error {
	return db.RunInTransaction(ctx, s.dbConnectionPool, nil, func(dbTx db.DBTransaction) error {
		payment, err := s.models.Payment.Get(ctx, paymentID, dbTx)
		if err != nil {
			if errors.Is(err, data.ErrRecordNotFound) {
				return ErrPaymentNotFound
			}
			return fmt.Errorf("error getting payment with id %s: %w", paymentID, err)
		}

		// 1. Verify Transition is Possible
		err = payment.Status.TransitionTo(data.PaymentStatus(data.CanceledPaymentStatus))
		if err != nil {
			return ErrPaymentNotReadyToCancel
		}

		// 2. Update payment status to `canceled`
		numRowsAffected, err := s.models.Payment.UpdateStatuses(ctx, dbTx, []*data.Payment{payment}, data.CanceledPaymentStatus)
		if err != nil {
			return fmt.Errorf("error updating payment status with id %s to 'canceled': %w", paymentID, err)
		}

		if numRowsAffected == 0 {
			return ErrPaymentStatusCantBeChanged
		}

		return nil
	})
}
