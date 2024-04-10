package eventhandlers

import (
	"context"
	"fmt"

	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events/schemas"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

type SendReceiverWalletsSMSInvitationEventHandlerOptions struct {
	AdminDBConnectionPool          db.DBConnectionPool
	MtnDBConnectionPool            db.DBConnectionPool
	AnchorPlatformBaseSepURL       string
	MessengerClient                message.MessengerClient
	MaxInvitationSMSResendAttempts int64
	Sep10SigningPrivateKey         string
	CrashTrackerClient             crashtracker.CrashTrackerClient
}

type SendReceiverWalletsSMSInvitationEventHandler struct {
	tenantManager       tenant.ManagerInterface
	mtnDBConnectionPool db.DBConnectionPool
	crashTrackerClient  crashtracker.CrashTrackerClient
	service             services.SendReceiverWalletInviteServiceInterface
}

var _ events.EventHandler = new(SendReceiverWalletsSMSInvitationEventHandler)

func NewSendReceiverWalletsSMSInvitationEventHandler(options SendReceiverWalletsSMSInvitationEventHandlerOptions) *SendReceiverWalletsSMSInvitationEventHandler {
	tm := tenant.NewManager(tenant.WithDatabase(options.AdminDBConnectionPool))

	models, err := data.NewModels(options.MtnDBConnectionPool)
	if err != nil {
		log.Fatalf("error getting models: %s", err.Error())
	}

	s, err := services.NewSendReceiverWalletInviteService(
		models,
		options.MessengerClient,
		options.Sep10SigningPrivateKey,
		options.MaxInvitationSMSResendAttempts,
		options.CrashTrackerClient,
	)
	if err != nil {
		log.Fatalf("error instantiating service: %s", err.Error())
	}

	return &SendReceiverWalletsSMSInvitationEventHandler{
		tenantManager:       tm,
		mtnDBConnectionPool: options.MtnDBConnectionPool,
		service:             s,
		crashTrackerClient:  options.CrashTrackerClient,
	}
}

func (h *SendReceiverWalletsSMSInvitationEventHandler) Name() string {
	return "SendReceiverWalletsSMSInvitationEventHandler"
}

func (h *SendReceiverWalletsSMSInvitationEventHandler) CanHandleMessage(ctx context.Context, message *events.Message) bool {
	return message.Topic == events.ReceiverWalletNewInvitationTopic
}

func (h *SendReceiverWalletsSMSInvitationEventHandler) Handle(ctx context.Context, message *events.Message) {
	receiverWalletInvitationData, err := utils.ConvertType[any, []schemas.EventReceiverWalletSMSInvitationData](message.Data)
	if err != nil {
		h.crashTrackerClient.LogAndReportErrors(ctx, err, fmt.Sprintf("[%s] could not convert data to %T: %v", h.Name(), []schemas.EventReceiverWalletSMSInvitationData{}, message.Data))
		return
	}

	t, err := h.tenantManager.GetTenantByID(ctx, message.TenantID, nil)
	if err != nil {
		h.crashTrackerClient.LogAndReportErrors(ctx, err, fmt.Sprintf("[%s] error getting tenant by id", h.Name()))
		return
	}

	ctx = tenant.SaveTenantInContext(ctx, t)

	if err := h.service.SendInvite(ctx, receiverWalletInvitationData...); err != nil {
		h.crashTrackerClient.LogAndReportErrors(ctx, err, fmt.Sprintf("[%s] sending receiver wallets invitation", h.Name()))
		return
	}
}
