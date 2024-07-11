package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/circle"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

//go:generate mockery --name=CircleReconciliationServiceInterface --case=underscore --structname=MockCircleReconciliationService --filename=circle_reconciliation_service.go
type CircleReconciliationServiceInterface interface {
	Reconcile(ctx context.Context) error
}

type CircleReconciliationService struct {
	Models              *data.Models
	CircleService       circle.ServiceInterface
	DistAccountResolver signing.DistributionAccountResolver
}

// Reconcile reconciles the pending Circle transfer requests for the tenant in the context. It fetches the rows from
// circte_transfer_request where status is set to pending, and then fetches the transfer details from Circle API. It
// updates the status of the transfer request in the DB based on the status of the transfer in Circle. If the transfer
// reached a successful/failure status, it updates the payment status in the DB as well to reflect that.
func (s *CircleReconciliationService) Reconcile(ctx context.Context) error {
	// Step 1: Get the tenant from the context.
	tnt, outerErr := tenant.GetTenantFromContext(ctx)
	if outerErr != nil {
		return fmt.Errorf("getting tenant from context: %w", outerErr)
	}

	// Step 2: check if the tenant distribution account is of type Circle, and if it is Active.
	distAcc, outerErr := s.DistAccountResolver.DistributionAccountFromContext(ctx)
	if outerErr != nil {
		return fmt.Errorf("getting distribution account from context: %w", outerErr)
	}
	if !distAcc.IsCircle() {
		log.Ctx(ctx).Debugf("Distribution account for tenant %q is not of type %q, skipping reconciliation...", tnt.Name, schema.CirclePlatform)
		return nil
	}
	if distAcc.Status != schema.AccountStatusActive {
		log.Ctx(ctx).Debugf("Distribution account for tenant %q is not %q, skipping reconciliation...", tnt.Name, schema.AccountStatusActive)
		return nil
	}

	var reconciliationErrors []error
	var reconciliationCount int
	outerErr = db.RunInTransaction(ctx, s.Models.DBConnectionPool, nil, func(dbTx db.DBTransaction) error {
		// Step 3: Get pending Circle transfer requests.
		circleRequests, err := s.Models.CircleTransferRequests.GetPendingReconciliation(ctx, dbTx)
		if err != nil {
			return fmt.Errorf("getting pending Circle transfer requests: %w", err)
		}

		log.Ctx(ctx).Debugf("Found %d pending Circle transfer requests in tenant %q", len(circleRequests), tnt.Name)
		if len(circleRequests) == 0 {
			return nil
		}

		// Step 4: Reconcile the pending Circle transfer requests.
		reconciliationCount = len(circleRequests)
		for _, circleRequest := range circleRequests {
			err = s.reconcileTransferRequest(ctx, dbTx, tnt, circleRequest)
			if err != nil {
				err = fmt.Errorf("reconciling Circle transfer request: %w", err)
				reconciliationErrors = append(reconciliationErrors, err)
			}
		}

		return nil
	})
	if outerErr != nil {
		return fmt.Errorf("running Circle reconciliation for tenant %q: %w", tnt.Name, outerErr)
	}

	if len(reconciliationErrors) > 0 {
		return fmt.Errorf("attempted to reconcyle %d circle requests but failed on %d reconciliations: %v", reconciliationCount, len(reconciliationErrors), reconciliationErrors)
	}

	return nil
}

// reconcileTransferRequest reconciles a Circle transfer request and updates the payment status in the DB. It returns an
// error if the reconciliation fails.
func (s *CircleReconciliationService) reconcileTransferRequest(ctx context.Context, dbTx db.DBTransaction, tnt *tenant.Tenant, circleRequest *data.CircleTransferRequest) error {
	// 4.1. get the Circle transfer by ID
	transfer, err := s.CircleService.GetTransferByID(ctx, *circleRequest.CircleTransferID)
	if err != nil {
		var cAPIErr *circle.APIError
		if errors.As(err, &cAPIErr) && cAPIErr.StatusCode == http.StatusBadRequest {
			// if the the Circle API returns a 400, increment the sync attempts and update the last sync
			errJSONBody, marshalErr := json.Marshal(cAPIErr)
			if marshalErr != nil {
				log.Ctx(ctx).Errorf("marshalling Circle APIError: %v", marshalErr)
			}

			// increment the sync attempts and update the last sync attempt time.
			var updateErr error
			circleRequest, updateErr = s.Models.CircleTransferRequests.Update(ctx, dbTx, circleRequest.IdempotencyKey, data.CircleTransferRequestUpdate{
				LastSyncAttemptAt: utils.TimePtr(time.Now()),
				SyncAttempts:      circleRequest.SyncAttempts + 1,
				ResponseBody:      errJSONBody,
			})
			if updateErr != nil {
				return fmt.Errorf("updating Circle transfer request sync attempts: %w", updateErr)
			}
		}
		return fmt.Errorf("getting Circle transfer by ID %q: %w", *circleRequest.CircleTransferID, err)
	}
	jsonBody, err := json.Marshal(transfer)
	if err != nil {
		return fmt.Errorf("converting transfer body to json: %w", err)
	}

	// 4.2. update the circle transfer request entry in the DB.
	newStatus := data.CircleTransferStatus(transfer.Status)
	if *circleRequest.Status == newStatus {
		// this condition should be unrechable, but we're adding this log just in case...
		log.Ctx(ctx).Debugf("[tenant=%s] Circle transfer request %q is already in status %q, skipping reconciliation...", tnt.Name, circleRequest.IdempotencyKey, newStatus)
		return nil
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

	// 4.3. update the payment status in the DB.
	newPaymentStatus, err := transfer.Status.ToPaymentStatus()
	if err != nil {
		return fmt.Errorf("converting Circle transfer status to Payment status: %w", err)
	}
	var statusMsg string
	switch newStatus {
	case data.CircleTransferStatusSuccess:
		statusMsg = fmt.Sprintf("Circle transfer completed successfully with the Stellar transaction hash: %q", transfer.TransactionHash)
	case data.CircleTransferStatusFailed:
		statusMsg = fmt.Sprintf("Circle transfer failed with error: %q", transfer.ErrorCode)
	default:
		return fmt.Errorf("unexpected Circle transfer status: %q", newStatus)
	}

	err = s.Models.Payment.UpdateStatus(ctx, dbTx, circleRequest.PaymentID, newPaymentStatus, &statusMsg, transfer.TransactionHash)
	if err != nil {
		return fmt.Errorf("updating payment status: %w", err)
	}

	log.Ctx(ctx).Infof("[tenant=%s] Reconciled Circle transfer request %q with status %q", tnt.Name, *circleRequest.CircleTransferID, newStatus)

	return nil
}
