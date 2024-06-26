package services

import (
	"context"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/circle"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
)

type CircleReconciliationServiceInterface interface {
	Reconcile(ctx context.Context) error
}

type CircleReconciliationService struct {
	Models              *data.Models
	CircleService       circle.ServiceInterface
	DistAccountResolver signing.DistributionAccountResolver
}

func NewCircleReconciliationService(models *data.Models, circleService circle.ServiceInterface, distAccountResolver signing.DistributionAccountResolver) CircleReconciliationServiceInterface {
	return &CircleReconciliationService{
		Models:              models,
		CircleService:       circleService,
		DistAccountResolver: distAccountResolver,
	}
}

func (s *CircleReconciliationService) Reconcile(ctx context.Context) error {
	return nil
}
