package services

import (
	"context"
	"fmt"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
)

type PaymentService struct {
	models           *data.Models
	dbConnectionPool db.DBConnectionPool
}

// NewPaymentService creates a new PaymentService
func NewPaymentService(models *data.Models, dbConnectionPool db.DBConnectionPool) *PaymentService {
	return &PaymentService{
		models:           models,
		dbConnectionPool: dbConnectionPool,
	}
}

type PaymentsPaginatedResponse struct {
	TotalPayments int
	Payments      []data.Payment
}

// GetPaymentsWithCount creates a new DB transaction to get payments and total payments filtered by query params.
func (s *PaymentService) GetPaymentsWithCount(ctx context.Context, queryParams *data.QueryParams) (*PaymentsPaginatedResponse, error) {
	return db.RunInTransactionWithResult(ctx, s.dbConnectionPool, nil, func(dbTx db.DBTransaction) (response *PaymentsPaginatedResponse, innerErr error) {
		totalPayments, innerErr := s.models.Payment.Count(ctx, queryParams, dbTx)
		if innerErr != nil {
			return nil, fmt.Errorf("error counting payments: %w", innerErr)
		}

		var payments []data.Payment
		if totalPayments != 0 {
			payments, innerErr = s.models.Payment.GetAll(ctx, queryParams, dbTx)
			if innerErr != nil {
				return nil, fmt.Errorf("error querying payments: %w", innerErr)
			}
		}

		return &PaymentsPaginatedResponse{
			TotalPayments: totalPayments,
			Payments:      payments,
		}, nil
	})
}
