package httphandler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stellar/go/support/render/httpjson"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events/schemas"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

type RetryInvitationMessageResponse struct {
	ID               string     `json:"id"`
	ReceiverID       string     `json:"receiver_id"`
	WalletID         string     `json:"wallet_id"`
	CreatedAt        time.Time  `json:"created_at"`
	InvitationSentAt *time.Time `json:"invitation_sent_at"`
}

type ReceiverWalletsHandler struct {
	Models             *data.Models
	EventProducer      events.Producer
	CrashTrackerClient crashtracker.CrashTrackerClient
}

func (h ReceiverWalletsHandler) RetryInvitation(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	receiverWalletID := chi.URLParam(req, "receiver_wallet_id")

	var msg *events.Message
	receiverWallet, err := db.RunInTransactionWithResult(ctx, h.Models.DBConnectionPool, nil, func(dbTx db.DBTransaction) (*data.ReceiverWallet, error) {
		receiverWallet, err := h.Models.ReceiverWallet.RetryInvitationMessage(ctx, dbTx, receiverWalletID)
		if err != nil {
			return nil, fmt.Errorf("retrying invitation message for receiver wallet ID %s: %w", receiverWalletID, err)
		}

		eventData := []schemas.EventReceiverWalletInvitationData{{ReceiverWalletID: receiverWalletID}}
		msg, err = events.NewMessage(ctx, events.ReceiverWalletNewInvitationTopic, receiverWalletID, events.RetryReceiverWalletInvitationType, eventData)
		if err != nil {
			return nil, fmt.Errorf("creating event producer message: %w", err)
		}
		err = msg.Validate()
		if err != nil {
			return nil, fmt.Errorf("validating event producer message %+v: %w", msg, err)
		}

		return receiverWallet, nil
	})
	if err != nil {
		if errors.Is(err, data.ErrRecordNotFound) {
			httperror.NotFound("", err, nil).Render(rw)
			return
		}

		if errors.Is(err, tenant.ErrTenantNotFoundInContext) {
			httperror.Forbidden("", err, nil).Render(rw)
			return
		}

		err = fmt.Errorf("retrying invitation: %w", err)
		httperror.InternalError(ctx, "", err, nil).Render(rw)
		return
	} else {
		err = events.ProduceEvents(ctx, h.EventProducer, msg)
		if err != nil {
			h.CrashTrackerClient.LogAndReportErrors(ctx, err, "writing retry invitation message on the event producer")
		}
	}

	response := RetryInvitationMessageResponse{
		ID:               receiverWallet.ID,
		ReceiverID:       receiverWallet.Receiver.ID,
		WalletID:         receiverWallet.Wallet.ID,
		CreatedAt:        receiverWallet.CreatedAt,
		InvitationSentAt: receiverWallet.InvitationSentAt,
	}

	httpjson.RenderStatus(rw, http.StatusOK, response, httpjson.JSON)
}

type PatchReceiverWalletStatusRequest struct {
	Status string `json:"status"`
}

var ErrUnsupportedStatusTransition = errors.New("invalid target status")

// PatchReceiverWalletStatus updates a receiver wallet’s status.
func (h ReceiverWalletsHandler) PatchReceiverWalletStatus(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	receiverWalletID := chi.URLParam(req, "receiver_wallet_id")
	if strings.TrimSpace(receiverWalletID) == "" {
		httperror.BadRequest("receiver_wallet_id is required", nil, nil).Render(rw)
		return
	}

	var patchRequest PatchReceiverWalletStatusRequest
	err := json.NewDecoder(req.Body).Decode(&patchRequest)
	if err != nil {
		httperror.BadRequest("invalid request", err, nil).Render(rw)
		return
	}

	// validate request
	toStatus, err := data.ToReceiversWalletStatus(patchRequest.Status)
	if err != nil {
		errMsg := fmt.Sprintf("invalid status %q; valid values %v", patchRequest.Status, data.ReceiversWalletStatuses())
		httperror.BadRequest(errMsg, nil, nil).Render(rw)
		return
	}

	if err = h.validateAndUpdateStatus(ctx, receiverWalletID, toStatus); err != nil {
		if errors.Is(err, data.ErrRecordNotFound) {
			httperror.NotFound("receiver wallet not found", err, nil).Render(rw)
			return
		} else if errors.Is(err, ErrUnsupportedStatusTransition) {
			errMsg := fmt.Sprintf("switching to status %q is not supported", toStatus)
			httperror.BadRequest(errMsg, nil, nil).Render(rw)
			return
		} else if errors.Is(err, data.ErrWalletNotRegistered) {
			httperror.BadRequest("receiver wallet is not registered", err, nil).Render(rw)
			return
		} else if errors.Is(err, data.ErrUnregisterUserManagedWallet) {
			httperror.BadRequest("user managed wallet cannot be unregistered", err, nil).Render(rw)
			return
		} else if errors.Is(err, data.ErrPaymentsInProgressForWallet) {
			httperror.BadRequest("wallet has payments in progress", err, nil).Render(rw)
			return
		}
		httperror.InternalError(ctx, "", err, nil).Render(rw)
		return
	}

	okMessage := fmt.Sprintf("receiver wallet status updated to %q", toStatus)
	httpjson.RenderStatus(rw, http.StatusOK, map[string]string{"message": okMessage}, httpjson.JSON)
}

// validateAndUpdateStatus validates the status transition and updates the receiver wallet status.
func (h ReceiverWalletsHandler) validateAndUpdateStatus(ctx context.Context, receiverWalletID string, toStatus data.ReceiversWalletStatus) error {
	switch toStatus {
	case data.ReadyReceiversWalletStatus:
		err := h.Models.ReceiverWallet.UpdateStatusToReady(ctx, receiverWalletID)
		if err != nil {
			return fmt.Errorf("updating receiver wallet %s: %w", receiverWalletID, err)
		}
		return nil
	default:
		return ErrUnsupportedStatusTransition
	}
}
