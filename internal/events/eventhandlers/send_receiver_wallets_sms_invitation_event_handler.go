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
	tenantManager       tenant.ManagerInterface
	mtnDBConnectionPool db.DBConnectionPool
	crashTrackerClient  crashtracker.CrashTrackerClient
	service             services.SendReceiverWalletInviteServiceInterface
}

var _ events.EventHandler = new(SendReceiverWalletsSMSInvitationEventHandler)

func NewSendReceiverWalletsSMSInvitationEventHandler(options SendReceiverWalletsSMSInvitationEventHandlerOptions) *SendReceiverWalletsSMSInvitationEventHandler {
	tm := tenant.NewManager(tenant.WithDatabase(options.DBConnectionPool))
	tr := router.NewMultiTenantDataSourceRouter(tm)
	mtnDBConnectionPool, err := db.NewConnectionPoolWithRouter(tr)
	if err != nil {
		log.Fatalf("error getting tenant DB Connection Pool: %s", err.Error())
	}

	models, err := data.NewModels(mtnDBConnectionPool)
	if err != nil {
		log.Fatalf("error getting models: %s", err.Error())
	}

	s, err := services.NewSendReceiverWalletInviteService(
		models,
		options.MessengerClient,
		options.AnchorPlatformBaseSepURL,
		options.Sep10SigningPrivateKey,
		options.MaxInvitationSMSResendAttempts,
		options.CrashTrackerClient,
	)
	if err != nil {
		log.Fatalf("error instantiating service: %s", err.Error())
	}

	return &SendReceiverWalletsSMSInvitationEventHandler{
		tenantManager:       tm,
		mtnDBConnectionPool: mtnDBConnectionPool,
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
		h.crashTrackerClient.LogAndReportErrors(ctx, err, fmt.Sprintf("[SendReceiverWalletsSMSInvitationEventHandler] could not convert data to %T: %v", []schemas.EventReceiverWalletSMSInvitationData{}, message.Data))
		return
	}

	t, err := h.tenantManager.GetTenantByID(ctx, message.TenantID)
	if err != nil {
		h.crashTrackerClient.LogAndReportErrors(ctx, err, "[SendReceiverWalletsSMSInvitationEventHandler] error getting tenant by id")
		return
	}

	ctx = tenant.SaveTenantInContext(ctx, t)

	if err := h.service.SendInvite(ctx, receiverWalletInvitationData...); err != nil {
		h.crashTrackerClient.LogAndReportErrors(ctx, err, "[SendReceiverWalletsSMSInvitationEventHandler] sending receiver wallets invitation")
		return
	}
}
