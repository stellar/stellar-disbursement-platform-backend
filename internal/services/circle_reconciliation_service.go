package services

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/circle"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
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
	// Step 1: Get the tenant from the context.
	tnt, err := tenant.GetTenantFromContext(ctx)
	if err != nil {
		return fmt.Errorf("getting tenant from context: %w", err)
	}

	// Step 2: check if the tenant distribution account is of type Circle, and if it is Active.
	distAcc, err := s.DistAccountResolver.DistributionAccountFromContext(ctx)
	if err != nil {
		return fmt.Errorf("getting distribution account from context: %w", err)
	}
	if !distAcc.IsCircle() {
		return nil
	}
	if distAcc.Status != schema.AccountStatusActive {
		log.Ctx(ctx).Infof("Distribution account for tenant %q is not %s, skipping reconciliation...", tnt.Name, schema.AccountStatusActive)
		return nil
	}

	err = db.RunInTransaction(ctx, s.Models.DBConnectionPool, nil, func(dbTx db.DBTransaction) error {
		// Step 3: Get pending Circle transfer requests.
		circleRequests, err := s.Models.CircleTransferRequests.GetPendingReconciliation(ctx, dbTx)
		if err != nil {
			return fmt.Errorf("getting pending Circle transfer requests: %w", err)
		}

		log.Ctx(ctx).Debugf("Found %d pending Circle transfer requests in tenant %s", len(circleRequests), tnt.Name)
		if len(circleRequests) == 0 {
			return nil
		}

		// Step 4: Reconcile the pending Circle transfer requests.
		for _, circleRequest := range circleRequests {
			// 4.1. get the Circle transfer by ID
			transfer, err := s.CircleService.GetTransferByID(ctx, *circleRequest.CircleTransferID)
			if err != nil {
				return fmt.Errorf("getting Circle transfer by ID %q: %w", *circleRequest.CircleTransferID, err)
			}

			jsonBody, err := json.Marshal(transfer)
			if err != nil {
				return fmt.Errorf("converting transfer body to json: %w", err)
			}

			// 4.2. update the circle transfer request entry in the DB
			newStatus := data.CircleTransferStatus(transfer.Status)
			if *circleRequest.Status == newStatus {
				log.Ctx(ctx).Debugf("[tenant=%s] Circle transfer request %q is already in status %q, skipping reconciliation...", tnt.Name, circleRequest.IdempotencyKey, newStatus)
				continue
			}

			now := time.Now()
			var completedAt *time.Time
			if newStatus.IsCompleted() {
				completedAt = &now
			}
			circleRequest, err = s.Models.CircleTransferRequests.Update(ctx, dbTx, circleRequest.IdempotencyKey, data.CircleTransferRequestUpdate{
				Status:            newStatus,
				CompletedAt:       completedAt,
				LastSyncAttemptAt: &now,
				SyncAttempts:      circleRequest.SyncAttempts + 1,
				ResponseBody:      jsonBody,
			})
			if err != nil {
				return fmt.Errorf("updating Circle transfer request: %w", err)
			}

			// 4.3. update the payment status in the DB
			newPaymentStatus, err := transfer.Status.ToPaymentStatus()
			if err != nil {
				return fmt.Errorf("converting Circle transfer status to Payment status: %w", err)
			}
			var statusMsg string
			if newStatus == data.CircleTransferStatusSuccess {
				statusMsg = fmt.Sprintf("Circle transfer completed successfully with the Stellar transaction hash: %q", transfer.TransactionHash)
			} else if newStatus == data.CircleTransferStatusFailed {
				statusMsg = fmt.Sprintf("Circle transfer failed with error: %q", transfer.ErrorCode)
			}

			err = s.Models.Payment.UpdateStatus(ctx, dbTx, circleRequest.PaymentID, newPaymentStatus, &statusMsg, transfer.TransactionHash)
			if err != nil {
				return fmt.Errorf("updating payment status: %w", err)
			}

			log.Ctx(ctx).Infof("[tenant=%s] Reconciled Circle transfer request %q with status %q", tnt.Name, circleRequest.IdempotencyKey, newStatus)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("running Circle reconciliation for tenant %q: %w", tnt.Name, err)
	}

	return nil
}
