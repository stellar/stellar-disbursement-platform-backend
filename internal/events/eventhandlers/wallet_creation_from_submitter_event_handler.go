package eventhandlers

import (
	"context"
	"fmt"

	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events/schemas"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

type WalletCreationFromSubmitterEventHandlerOptions struct {
	AdminDBConnectionPool db.DBConnectionPool
	MtnDBConnectionPool   db.DBConnectionPool
	TSSDBConnectionPool   db.DBConnectionPool
	NetworkPassphrase     string
}

type WalletCreationFromSubmitterEventHandler struct {
	tenantManager tenant.ManagerInterface
	service       services.WalletCreationFromSubmitterServiceInterface
}

var _ events.EventHandler = new(WalletCreationFromSubmitterEventHandler)

func NewWalletCreationFromSubmitterEventHandler(options WalletCreationFromSubmitterEventHandlerOptions) *WalletCreationFromSubmitterEventHandler {
	models, err := data.NewModels(options.MtnDBConnectionPool)
	if err != nil {
		log.Fatalf("error getting models: %s", err.Error())
	}

	return &WalletCreationFromSubmitterEventHandler{
		tenantManager: tenant.NewManager(tenant.WithDatabase(options.AdminDBConnectionPool)),
		service:       services.NewWalletCreationFromSubmitterService(models, options.TSSDBConnectionPool, options.NetworkPassphrase),
	}
}

func (h *WalletCreationFromSubmitterEventHandler) Handle(ctx context.Context, message *events.Message) error {
	tx, err := utils.ConvertType[any, schemas.EventWalletCreationCompletedData](message.Data)
	if err != nil {
		return fmt.Errorf("could not convert message data to %T: %w", schemas.EventWalletCreationCompletedData{}, err)
	}

	t, err := h.tenantManager.GetTenantByID(ctx, message.TenantID)
	if err != nil {
		return fmt.Errorf("getting tenant by id %s: %w", message.TenantID, err)
	}

	ctx = sdpcontext.SetTenantInContext(ctx, t)

	if err := h.service.SyncTransaction(ctx, tx.TransactionID); err != nil {
		return fmt.Errorf("syncing transaction completion for transaction ID %q: %w", tx.TransactionID, err)
	}

	return nil
}

func (h *WalletCreationFromSubmitterEventHandler) Name() string {
	return utils.GetTypeName(h)
}

func (h *WalletCreationFromSubmitterEventHandler) CanHandleMessage(ctx context.Context, message *events.Message) bool {
	return message.Topic == events.WalletCreationCompletedTopic
}
