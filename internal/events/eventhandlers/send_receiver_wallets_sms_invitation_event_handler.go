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
	UseExternalID                  bool
}

type SendReceiverWalletsSMSInvitationEventHandler struct {
	tenantManager       tenant.ManagerInterface
	mtnDBConnectionPool db.DBConnectionPool
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
		options.UseExternalID,
	)
	if err != nil {
		log.Fatalf("error instantiating service: %s", err.Error())
	}

	return &SendReceiverWalletsSMSInvitationEventHandler{
		tenantManager:       tm,
		mtnDBConnectionPool: options.MtnDBConnectionPool,
		service:             s,
	}
}

func (h *SendReceiverWalletsSMSInvitationEventHandler) Name() string {
	return utils.GetTypeName(h)
}

func (h *SendReceiverWalletsSMSInvitationEventHandler) CanHandleMessage(ctx context.Context, message *events.Message) bool {
	return message.Topic == events.ReceiverWalletNewInvitationTopic
}

func (h *SendReceiverWalletsSMSInvitationEventHandler) Handle(ctx context.Context, message *events.Message) error {
	receiverWalletInvitationData, err := utils.ConvertType[any, []schemas.EventReceiverWalletSMSInvitationData](message.Data)
	if err != nil {
		return fmt.Errorf("could not convert message data to %T: %w", []schemas.EventReceiverWalletSMSInvitationData{}, err)
	}

	t, err := h.tenantManager.GetTenantByID(ctx, message.TenantID)
	if err != nil {
		return fmt.Errorf("getting tenant by id %s: %w", message.TenantID, err)
	}

	ctx = tenant.SaveTenantInContext(ctx, t)

	if sendErr := h.service.SendInvite(ctx, receiverWalletInvitationData...); sendErr != nil {
		return fmt.Errorf("sending receiver wallets invitation: %w", sendErr)
	}

	return nil
}
