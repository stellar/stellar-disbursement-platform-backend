package httphandler

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stellar/go/support/log"
	"github.com/stellar/go/support/render/httpjson"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events/schemas"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

type RetryInvitationSMSResponse struct {
	ID               string     `json:"id"`
	ReceiverID       string     `json:"receiver_id"`
	WalletID         string     `json:"wallet_id"`
	CreatedAt        time.Time  `json:"created_at"`
	InvitationSentAt *time.Time `json:"invitation_sent_at"`
}

type ReceiverWalletsHandler struct {
	Models        *data.Models
	EventProducer events.Producer
}

func (h ReceiverWalletsHandler) RetryInvitation(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	receiverWalletID := chi.URLParam(req, "receiver_wallet_id")

	receiverWallet, err := db.RunInTransactionWithResult(ctx, h.Models.DBConnectionPool, nil, func(dbTx db.DBTransaction) (*data.ReceiverWallet, error) {
		receiverWallet, err := h.Models.ReceiverWallet.RetryInvitationSMS(ctx, dbTx, receiverWalletID)
		if err != nil {
			return nil, fmt.Errorf("retrying invitation SMS for receiver wallet ID %s: %w", receiverWalletID, err)
		}

		msg, err := events.NewMessage(ctx, events.ReceiverWalletNewInvitationTopic, receiverWalletID, events.RetryReceiverWalletSMSInvitationType, []schemas.EventReceiverWalletSMSInvitationData{
			{
				ReceiverWalletID: receiverWalletID,
			},
		})
		if err != nil {
			return nil, fmt.Errorf("creating event producer message: %w", err)
		}

		if h.EventProducer != nil {
			err = h.EventProducer.WriteMessages(ctx, *msg)
			if err != nil {
				return nil, fmt.Errorf("publishing message %s on event producer: %w", msg, err)
			}
		} else {
			log.Ctx(ctx).Errorf("event producer is nil, could not publish message %s", msg)
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
	}

	response := RetryInvitationSMSResponse{
		ID:               receiverWallet.ID,
		ReceiverID:       receiverWallet.Receiver.ID,
		WalletID:         receiverWallet.Wallet.ID,
		CreatedAt:        receiverWallet.CreatedAt,
		InvitationSentAt: receiverWallet.InvitationSentAt,
	}

	httpjson.RenderStatus(rw, http.StatusOK, response, httpjson.JSON)
}
