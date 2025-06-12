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

type SendReceiverWalletsInvitationEventHandlerOptions struct {
	AdminDBConnectionPool       db.DBConnectionPool
	MtnDBConnectionPool         db.DBConnectionPool
	AnchorPlatformBaseSepURL    string
	MessageDispatcher           message.MessageDispatcherInterface
	EmbeddedWalletService       services.EmbeddedWalletServiceInterface
	MaxInvitationResendAttempts int64
	Sep10SigningPrivateKey      string
	CrashTrackerClient          crashtracker.CrashTrackerClient
}

type SendReceiverWalletsInvitationEventHandler struct {
	tenantManager       tenant.ManagerInterface
	mtnDBConnectionPool db.DBConnectionPool
	service             services.SendReceiverWalletInviteServiceInterface
}

var _ events.EventHandler = new(SendReceiverWalletsInvitationEventHandler)

func NewSendReceiverWalletsInvitationEventHandler(options SendReceiverWalletsInvitationEventHandlerOptions) *SendReceiverWalletsInvitationEventHandler {
	tm := tenant.NewManager(tenant.WithDatabase(options.AdminDBConnectionPool))

	models, err := data.NewModels(options.MtnDBConnectionPool)
	if err != nil {
		log.Fatalf("error getting models: %s", err.Error())
	}

	s, err := services.NewSendReceiverWalletInviteService(
		models,
		options.MessageDispatcher,
		options.EmbeddedWalletService,
		options.Sep10SigningPrivateKey,
		options.MaxInvitationResendAttempts,
		options.CrashTrackerClient,
	)
	if err != nil {
		log.Fatalf("error instantiating service: %s", err.Error())
	}

	return &SendReceiverWalletsInvitationEventHandler{
		tenantManager:       tm,
		mtnDBConnectionPool: options.MtnDBConnectionPool,
		service:             s,
	}
}

func (h *SendReceiverWalletsInvitationEventHandler) Name() string {
	return utils.GetTypeName(h)
}

func (h *SendReceiverWalletsInvitationEventHandler) CanHandleMessage(ctx context.Context, message *events.Message) bool {
	return message.Topic == events.ReceiverWalletNewInvitationTopic
}

func (h *SendReceiverWalletsInvitationEventHandler) Handle(ctx context.Context, message *events.Message) error {
	receiverWalletInvitationData, err := utils.ConvertType[any, []schemas.EventReceiverWalletInvitationData](message.Data)
	if err != nil {
		return fmt.Errorf("could not convert message data to %T: %w", []schemas.EventReceiverWalletInvitationData{}, err)
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
