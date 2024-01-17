package httphandler

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stellar/go/support/log"
	"github.com/stellar/go/support/render/httpjson"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events/schemas"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
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
	receiverWallet, err := h.Models.ReceiverWallet.RetryInvitationSMS(ctx, h.Models.DBConnectionPool, receiverWalletID)
	if err != nil {
		if errors.Is(err, data.ErrRecordNotFound) {
			httperror.NotFound("", err, nil).Render(rw)
			return
		}
		err = fmt.Errorf("retrying invitation: %w", err)
		httperror.InternalError(ctx, "", err, nil).Render(rw)
		return
	}

	msg, err := events.NewMessage(ctx, events.ReceiverWalletNewInvitationTopic, receiverWalletID, events.RetryReceiverWalletSMSInvitationType, []schemas.EventReceiverWalletSMSInvitationData{
		{
			ReceiverWalletID: receiverWalletID,
		},
	})
	if err != nil {
		err = fmt.Errorf("creating event producer message: %w", err)
		httperror.Forbidden("", err, nil).Render(rw)
		return
	}

	if h.EventProducer != nil {
		err = h.EventProducer.WriteMessages(ctx, *msg)
		if err != nil {
			err = fmt.Errorf("publishing message %s on event producer: %w", msg.String(), err)
			httperror.InternalError(ctx, "", err, nil).Render(rw)
			return
		}
	} else {
		log.Ctx(ctx).Debugf("message %s not published because eventProducer is nil", msg.String())
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
