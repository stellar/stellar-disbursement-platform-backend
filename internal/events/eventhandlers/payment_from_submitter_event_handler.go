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
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

type PaymentFromSubmitterEventHandlerOptions struct {
	AdminDBConnectionPool db.DBConnectionPool
	MtnDBConnectionPool   db.DBConnectionPool
	TSSDBConnectionPool   db.DBConnectionPool
	CrashTrackerClient    crashtracker.CrashTrackerClient
}

type PaymentFromSubmitterEventHandler struct {
	tenantManager      tenant.ManagerInterface
	crashTrackerClient crashtracker.CrashTrackerClient
	service            services.PaymentFromSubmitterServiceInterface
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
		tenantManager:      tm,
		service:            s,
		crashTrackerClient: options.CrashTrackerClient,
	}
}

func (h *PaymentFromSubmitterEventHandler) Name() string {
	return "PaymentFromSubmitterEventHandler"
}

func (h *PaymentFromSubmitterEventHandler) CanHandleMessage(ctx context.Context, message *events.Message) bool {
	return message.Topic == events.PaymentCompletedTopic
}

func (h *PaymentFromSubmitterEventHandler) Handle(ctx context.Context, message *events.Message) {
	tx, err := utils.ConvertType[any, schemas.EventPaymentCompletedData](message.Data)
	if err != nil {
		h.crashTrackerClient.LogAndReportErrors(ctx, err, fmt.Sprintf("[%s] could not convert data to %T: %v", h.Name(), schemas.EventPaymentCompletedData{}, message.Data))
		return
	}

	t, err := h.tenantManager.GetTenantByID(ctx, message.TenantID)
	if err != nil {
		h.crashTrackerClient.LogAndReportErrors(ctx, err, fmt.Sprintf("[%s] error getting tenant by id", h.Name()))
		return
	}

	ctx = tenant.SaveTenantInContext(ctx, t)

	if err := h.service.SyncTransaction(ctx, &tx); err != nil {
		h.crashTrackerClient.LogAndReportErrors(ctx, err, fmt.Sprintf("[%s] synching transaction completion for transaction ID %q", h.Name(), tx.TransactionID))
		return
	}
}
