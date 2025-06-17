package eventhandlers

import (
	"context"
	"fmt"

	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events/schemas"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

type EmbeddedWalletFromSubmitterEventHandlerOptions struct {
	AdminDBConnectionPool db.DBConnectionPool
	MtnDBConnectionPool   db.DBConnectionPool
	TSSDBConnectionPool   db.DBConnectionPool
	NetworkPassphrase     string
}

type EmbeddedWalletFromSubmitterEventHandler struct {
	tenantManager tenant.ManagerInterface
	service       services.EmbeddedWalletFromSubmitterServiceInterface
}

var _ events.EventHandler = new(EmbeddedWalletFromSubmitterEventHandler)

func NewEmbeddedWalletFromSubmitterEventHandler(options EmbeddedWalletFromSubmitterEventHandlerOptions) *EmbeddedWalletFromSubmitterEventHandler {
	models, err := data.NewModels(options.MtnDBConnectionPool)
	if err != nil {
		log.Fatalf("error getting models: %s", err.Error())
	}

	return &EmbeddedWalletFromSubmitterEventHandler{
		tenantManager: tenant.NewManager(tenant.WithDatabase(options.AdminDBConnectionPool)),
		service:       services.NewEmbeddedWalletFromSubmitterService(models, options.TSSDBConnectionPool, options.NetworkPassphrase),
	}
}

func (h *EmbeddedWalletFromSubmitterEventHandler) Handle(ctx context.Context, message *events.Message) error {
	tx, err := utils.ConvertType[any, schemas.EventWalletCreationCompletedData](message.Data)
	if err != nil {
		return fmt.Errorf("could not convert message data to %T: %w", schemas.EventWalletCreationCompletedData{}, err)
	}

	t, err := h.tenantManager.GetTenantByID(ctx, message.TenantID)
	if err != nil {
		return fmt.Errorf("getting tenant by id %s: %w", message.TenantID, err)
	}

	ctx = tenant.SaveTenantInContext(ctx, t)

	if err := h.service.SyncTransaction(ctx, tx.TransactionID); err != nil {
		return fmt.Errorf("syncing transaction completion for transaction ID %q: %w", tx.TransactionID, err)
	}

	return nil
}

func (h *EmbeddedWalletFromSubmitterEventHandler) Name() string {
	return utils.GetTypeName(h)
}

func (h *EmbeddedWalletFromSubmitterEventHandler) CanHandleMessage(ctx context.Context, message *events.Message) bool {
	return message.Topic == events.WalletCreationCompletedTopic
}
