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
	"github.com/stellar/go/strkey"
	"github.com/stellar/go/support/render/httpjson"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events/schemas"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
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

		if errors.Is(err, sdpcontext.ErrTenantNotFoundInContext) {
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

type PatchReceiverWalletRequest struct {
	StellarAddress string `json:"stellar_address"`
	StellarMemo    string `json:"stellar_memo,omitempty"`
}

// PatchReceiverWallet updates a receiver wallet's Stellar address and memo for user-managed wallets
func (h ReceiverWalletsHandler) PatchReceiverWallet(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	receiverWalletID := chi.URLParam(req, "receiver_wallet_id")
	if strings.TrimSpace(receiverWalletID) == "" {
		httperror.BadRequest("receiver_wallet_id is required", nil, nil).Render(rw)
		return
	}

	receiverID := chi.URLParam(req, "receiver_id")
	if strings.TrimSpace(receiverID) == "" {
		httperror.BadRequest("receiver_id is required", nil, nil).Render(rw)
		return
	}

	// Parse the request body into our DTO structure
	var patchRequest PatchReceiverWalletRequest
	err := json.NewDecoder(req.Body).Decode(&patchRequest)
	if err != nil {
		httperror.BadRequest("invalid request body", err, nil).Render(rw)
		return
	}

	// Validate required fields in the request body
	if strings.TrimSpace(patchRequest.StellarAddress) == "" {
		httperror.BadRequest("stellar_address is required", nil, nil).Render(rw)
		return
	}

	// Validate that stellar_address is a valid Stellar public key
	if !strkey.IsValidEd25519PublicKey(patchRequest.StellarAddress) {
		httperror.BadRequest("stellar_address must be a valid Stellar public key", nil, nil).Render(rw)
		return
	}

	updatedReceiverWallet, err := db.RunInTransactionWithResult(ctx, h.Models.DBConnectionPool, nil, func(dbTx db.DBTransaction) (*data.ReceiverWallet, error) {
		// 1: Validate existing receiver wallet
		currentReceiverWallet, txErr := h.Models.ReceiverWallet.GetByID(ctx, dbTx, receiverWalletID)
		if txErr != nil {
			return nil, fmt.Errorf("getting receiver wallet by ID %s: %w", receiverWalletID, txErr)
		}

		if currentReceiverWallet.Receiver.ID != receiverID {
			return nil, httperror.BadRequest("Receiver wallet does not belong to the specified receiver", nil, nil)
		}

		if !currentReceiverWallet.Wallet.UserManaged {
			return nil, httperror.BadRequest("Cannot edit stellar address for non-user-managed wallet", nil, nil)
		}

		// 2: Prepare the wallet update with new address and optional memo
		walletUpdate := data.ReceiverWalletUpdate{
			StellarAddress: patchRequest.StellarAddress,
		}

		if strings.TrimSpace(patchRequest.StellarMemo) != "" {
			memoType := schema.MemoTypeID
			walletUpdate.StellarMemo = &patchRequest.StellarMemo
			walletUpdate.StellarMemoType = &memoType
		} else {
			walletUpdate.StellarMemo = nil
			walletUpdate.StellarMemoType = nil
		}

		// 3: Update the receiver wallet
		if txErr = h.Models.ReceiverWallet.Update(ctx, receiverWalletID, walletUpdate, dbTx); txErr != nil {
			return nil, fmt.Errorf("updating receiver wallet %s: %w", receiverWalletID, txErr)
		}

		// 4: Retrieve the updated receiver wallet
		updatedWallet, txErr := h.Models.ReceiverWallet.GetByID(ctx, dbTx, receiverWalletID)
		if txErr != nil {
			return nil, fmt.Errorf("getting updated receiver wallet %s: %w", receiverWalletID, txErr)
		}

		return updatedWallet, nil
	})

	if err != nil {
		var httpErr *httperror.HTTPError
		if errors.As(err, &httpErr) {
			httpErr.Render(rw)
			return
		}

		// Handle duplicate wallet addresses
		if httpErr := httperror.HandlePostgreSQLConflictErrors(err); httpErr != nil {
			httpErr.Render(rw)
			return
		}

		httperror.InternalError(ctx, "Error updating receiver wallet", err, nil).Render(rw)
		return
	}

	httpjson.RenderStatus(rw, http.StatusOK, updatedReceiverWallet, httpjson.JSON)
}
