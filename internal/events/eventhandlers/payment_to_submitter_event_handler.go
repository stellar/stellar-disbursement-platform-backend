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

type PaymentToSubmitterEventHandlerOptions struct {
	AdminDBConnectionPool db.DBConnectionPool
	MtnDBConnectionPool   db.DBConnectionPool
	TSSDBConnectionPool   db.DBConnectionPool
	CrashTrackerClient    crashtracker.CrashTrackerClient
}

type PaymentToSubmitterEventHandler struct {
	tenantManager      tenant.ManagerInterface
	crashTrackerClient crashtracker.CrashTrackerClient
	service            services.PaymentToSubmitterServiceInterface
}

var _ events.EventHandler = new(PaymentToSubmitterEventHandler)

func NewPaymentToSubmitterEventHandler(options PaymentToSubmitterEventHandlerOptions) *PaymentToSubmitterEventHandler {
	tm := tenant.NewManager(tenant.WithDatabase(options.AdminDBConnectionPool))

	models, err := data.NewModels(options.MtnDBConnectionPool)
	if err != nil {
		log.Fatalf("error getting models: %s", err.Error())
	}

	s := services.NewPaymentToSubmitterService(models, options.TSSDBConnectionPool)

	return &PaymentToSubmitterEventHandler{
		tenantManager:      tm,
		service:            s,
		crashTrackerClient: options.CrashTrackerClient,
	}
}

func (h *PaymentToSubmitterEventHandler) Name() string {
	return "PaymentToSubmitterEventHandler"
}

func (h *PaymentToSubmitterEventHandler) CanHandleMessage(ctx context.Context, message *events.Message) bool {
	return message.Topic == events.PaymentReadyToPayTopic
}

func (h *PaymentToSubmitterEventHandler) Handle(ctx context.Context, message *events.Message) {
	paymentsReadyToPay, err := utils.ConvertType[any, schemas.EventPaymentsReadyToPayData](message.Data)
	if err != nil {
		h.crashTrackerClient.LogAndReportErrors(ctx, err, fmt.Sprintf("[%s] could not convert data to %T: %v", h.Name(), schemas.EventPaymentsReadyToPayData{}, message.Data))
		return
	}

	t, err := h.tenantManager.GetTenantByID(ctx, message.TenantID)
	if err != nil {
		h.crashTrackerClient.LogAndReportErrors(ctx, err, fmt.Sprintf("[%s] error getting tenant by id", h.Name()))
		return
	}

	ctx = tenant.SaveTenantInContext(ctx, t)

	if err := h.service.SendPaymentsReadyToPay(ctx, paymentsReadyToPay); err != nil {
		h.crashTrackerClient.LogAndReportErrors(ctx, err, fmt.Sprintf("[%s] send payments ready to pay: %s", h.Name(), paymentsReadyToPay.Payments))
		return
	}
}
