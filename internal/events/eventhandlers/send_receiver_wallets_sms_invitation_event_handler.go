package eventhandlers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events/schemas"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/router"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

type SendReceiverWalletsSMSInvitationEventHandlerOptions struct {
	DBConnectionPool               db.DBConnectionPool
	AnchorPlatformBaseSepURL       string
	MessengerClient                message.MessengerClient
	MaxInvitationSMSResendAttempts int64
	Sep10SigningPrivateKey         string
	CrashTrackerClient             crashtracker.CrashTrackerClient
}

type SendReceiverWalletsSMSInvitationEventHandler struct {
	tenantManager          tenant.ManagerInterface
	tenantDBConnectionPool db.DBConnectionPool
	crashTrackerClient     crashtracker.CrashTrackerClient
	service                services.SendReceiverWalletInviteServiceInterface
}

var _ events.EventHandler = new(SendReceiverWalletsSMSInvitationEventHandler)

func NewSendReceiverWalletsSMSInvitationEventHandler(options SendReceiverWalletsSMSInvitationEventHandlerOptions) *SendReceiverWalletsSMSInvitationEventHandler {
	s, err := services.NewSendReceiverWalletInviteService(
		nil,
		options.MessengerClient,
		options.AnchorPlatformBaseSepURL,
		options.Sep10SigningPrivateKey,
		options.MaxInvitationSMSResendAttempts,
		options.CrashTrackerClient,
	)
	if err != nil {
		log.Fatalf("error instantiating service: %s", err.Error())
	}

	tm := tenant.NewManager(tenant.WithDatabase(options.DBConnectionPool))
	tr := router.NewMultiTenantDataSourceRouter(tm)
	tenantDBConnectionPool, err := db.NewConnectionPoolWithRouter(tr)
	if err != nil {
		log.Fatalf("error getting tenant DB Connection Pool: %s", err.Error())
	}

	return &SendReceiverWalletsSMSInvitationEventHandler{
		tenantManager:          tm,
		tenantDBConnectionPool: tenantDBConnectionPool,
		service:                s,
		crashTrackerClient:     options.CrashTrackerClient,
	}
}

func (h *SendReceiverWalletsSMSInvitationEventHandler) Name() string {
	return "SendReceiverWalletsSMSInvitationEventHandler"
}

func (h *SendReceiverWalletsSMSInvitationEventHandler) CanHandleMessage(ctx context.Context, message *events.Message) bool {
	return message.Topic == events.ReceiverWalletSMSInvitationTopic
}

func (h *SendReceiverWalletsSMSInvitationEventHandler) Handle(ctx context.Context, message *events.Message) {
	dataJSON, err := json.Marshal(message.Data)
	if err != nil {
		h.crashTrackerClient.LogAndReportErrors(ctx, err, fmt.Sprintf("[SendReceiverWalletsSMSInvitationEventHandler] could not marshal data: %v", message.Data))
		return
	}

	var receiverWalletInvitationData []schemas.EventReceiverWalletSMSInvitationData
	err = json.Unmarshal(dataJSON, &receiverWalletInvitationData)
	if err != nil {
		h.crashTrackerClient.LogAndReportErrors(ctx, err, fmt.Sprintf("[SendReceiverWalletsSMSInvitationEventHandler] could not unmarshal data: %v", message.Data))
		return
	}

	t, err := h.tenantManager.GetTenantByID(ctx, message.TenantID)
	if err != nil {
		h.crashTrackerClient.LogAndReportErrors(ctx, err, "[SendReceiverWalletsSMSInvitationEventHandler] error getting tenant by id")
		return
	}

	ctx = tenant.SaveTenantInContext(ctx, t)

	models, err := data.NewModels(h.tenantDBConnectionPool)
	if err != nil {
		h.crashTrackerClient.LogAndReportErrors(ctx, err, "[SendReceiverWalletsSMSInvitationEventHandler] error getting models")
		return
	}

	h.service.SetModels(models)
	if err := h.service.SendInvite(ctx, receiverWalletInvitationData...); err != nil {
		h.crashTrackerClient.LogAndReportErrors(ctx, err, "[SendReceiverWalletsSMSInvitationEventHandler] sending receiver wallets invitation")
		return
	}
}
