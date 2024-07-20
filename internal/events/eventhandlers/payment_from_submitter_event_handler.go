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

type PaymentFromSubmitterEventHandlerOptions struct {
	AdminDBConnectionPool db.DBConnectionPool
	MtnDBConnectionPool   db.DBConnectionPool
	TSSDBConnectionPool   db.DBConnectionPool
}

type PaymentFromSubmitterEventHandler struct {
	tenantManager tenant.ManagerInterface
	service       services.PaymentFromSubmitterServiceInterface
}

var _ events.EventHandler = new(PaymentFromSubmitterEventHandler)

func NewPaymentFromSubmitterEventHandler(options PaymentFromSubmitterEventHandlerOptions) *PaymentFromSubmitterEventHandler {
	tm := tenant.NewManager(tenant.WithDatabase(options.AdminDBConnectionPool))

	models, err := data.NewModels(options.MtnDBConnectionPool)
	if err != nil {
		log.Fatalf("error getting models: %s", err.Error())
	}

	s := services.NewPaymentFromSubmitterService(models, options.TSSDBConnectionPool)

	return &PaymentFromSubmitterEventHandler{
		tenantManager: tm,
		service:       s,
	}
}

func (h *PaymentFromSubmitterEventHandler) Name() string {
	return utils.GetTypeName(h)
}

func (h *PaymentFromSubmitterEventHandler) CanHandleMessage(ctx context.Context, message *events.Message) bool {
	return message.Topic == events.PaymentCompletedTopic
}

func (h *PaymentFromSubmitterEventHandler) Handle(ctx context.Context, message *events.Message) error {
	tx, err := utils.ConvertType[any, schemas.EventPaymentCompletedData](message.Data)
	if err != nil {
		return fmt.Errorf("could not convert message data to %T: %w", schemas.EventPaymentCompletedData{}, err)
	}

	t, err := h.tenantManager.GetTenantByID(ctx, message.TenantID)
	if err != nil {
		return fmt.Errorf("getting tenant by id %s: %w", message.TenantID, err)
	}

	ctx = tenant.SaveTenantInContext(ctx, t)

	if syncErr := h.service.SyncTransaction(ctx, &tx); syncErr != nil {
		return fmt.Errorf("syncing transaction completion for transaction ID %q: %w", tx.TransactionID, syncErr)
	}

	return nil
}
