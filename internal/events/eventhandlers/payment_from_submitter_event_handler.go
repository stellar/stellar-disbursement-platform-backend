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
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/router"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

type PaymentFromSubmitterEventHandlerOptions struct {
	DBConnectionPool    db.DBConnectionPool
	TSSDBConnectionPool db.DBConnectionPool
	CrashTrackerClient  crashtracker.CrashTrackerClient
}

type PaymentFromSubmitterEventHandler struct {
	tenantManager       tenant.ManagerInterface
	mtnDBConnectionPool db.DBConnectionPool
	crashTrackerClient  crashtracker.CrashTrackerClient
	service             services.PaymentFromSubmitterServiceInterface
}

var _ events.EventHandler = new(PaymentFromSubmitterEventHandler)

func NewPaymentFromSubmitterEventHandler(options PaymentFromSubmitterEventHandlerOptions) *PaymentFromSubmitterEventHandler {
	s := services.NewPaymentFromSubmitterService(options.TSSDBConnectionPool)

	tm := tenant.NewManager(tenant.WithDatabase(options.DBConnectionPool))
	tr := router.NewMultiTenantDataSourceRouter(tm)
	mtnDBConnectionPool, err := db.NewConnectionPoolWithRouter(tr)
	if err != nil {
		log.Fatalf("error getting tenant DB Connection Pool: %s", err.Error())
	}

	return &PaymentFromSubmitterEventHandler{
		tenantManager:       tm,
		mtnDBConnectionPool: mtnDBConnectionPool,
		service:             s,
		crashTrackerClient:  options.CrashTrackerClient,
	}
}

func (h *PaymentFromSubmitterEventHandler) Name() string {
	return "PaymentFromSubmitterEventHandler"
}

func (h *PaymentFromSubmitterEventHandler) CanHandleMessage(ctx context.Context, message *events.Message) bool {
	return message.Topic == events.PaymentFromSubmitterTopic
}

func (h *PaymentFromSubmitterEventHandler) Handle(ctx context.Context, message *events.Message) {
	tx, err := utils.ConvertType[any, schemas.EventPaymentFromSubmitterData](message.Data)
	if err != nil {
		h.crashTrackerClient.LogAndReportErrors(ctx, err, fmt.Sprintf("[PaymentFromSubmitterEventHandler] could convert data to %T: %v", schemas.EventPaymentFromSubmitterData{}, message.Data))
		return
	}

	t, err := h.tenantManager.GetTenantByID(ctx, message.TenantID)
	if err != nil {
		h.crashTrackerClient.LogAndReportErrors(ctx, err, "[PaymentFromSubmitterEventHandler] error getting tenant by id")
		return
	}

	ctx = tenant.SaveTenantInContext(ctx, t)

	models, err := data.NewModels(h.mtnDBConnectionPool)
	if err != nil {
		h.crashTrackerClient.LogAndReportErrors(ctx, err, "[PaymentFromSubmitterEventHandler] error getting models")
		return
	}

	h.service.SetModels(models)
	if err := h.service.SyncTransaction(ctx, &tx); err != nil {
		h.crashTrackerClient.LogAndReportErrors(ctx, err, fmt.Sprintf("[PaymentFromSubmitterEventHandler] synching transaction completion for transaction ID %q", tx.TransactionID))
		return
	}
}
