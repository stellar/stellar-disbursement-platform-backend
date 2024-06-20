package eventhandlers

import (
	"context"
	"fmt"

	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/circle"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events/schemas"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

type PaymentToSubmitterEventHandlerOptions struct {
	AdminDBConnectionPool db.DBConnectionPool
	MtnDBConnectionPool   db.DBConnectionPool
	TSSDBConnectionPool   db.DBConnectionPool
	DistAccountResolver   signing.DistributionAccountResolver
	CircleService         circle.ServiceInterface
}

type PaymentToSubmitterEventHandler struct {
	tenantManager tenant.ManagerInterface
	service       services.PaymentToSubmitterServiceInterface
}

var _ events.EventHandler = new(PaymentToSubmitterEventHandler)

func NewPaymentToSubmitterEventHandler(options PaymentToSubmitterEventHandlerOptions) *PaymentToSubmitterEventHandler {
	tm := tenant.NewManager(tenant.WithDatabase(options.AdminDBConnectionPool))

	models, err := data.NewModels(options.MtnDBConnectionPool)
	if err != nil {
		log.Fatalf("error getting models: %s", err.Error())
	}

	s := services.NewPaymentToSubmitterService(models, options.TSSDBConnectionPool, options.DistAccountResolver, options.CircleService)

	return &PaymentToSubmitterEventHandler{
		tenantManager: tm,
		service:       s,
	}
}

func (h *PaymentToSubmitterEventHandler) Name() string {
	return utils.GetTypeName(h)
}

func (h *PaymentToSubmitterEventHandler) CanHandleMessage(ctx context.Context, message *events.Message) bool {
	return message.Topic == events.PaymentReadyToPayTopic
}

func (h *PaymentToSubmitterEventHandler) Handle(ctx context.Context, message *events.Message) error {
	paymentsReadyToPay, err := utils.ConvertType[any, schemas.EventPaymentsReadyToPayData](message.Data)
	if err != nil {
		return fmt.Errorf("could not convert message data to %T: %w", schemas.EventPaymentsReadyToPayData{}, err)
	}

	t, err := h.tenantManager.GetTenantByID(ctx, message.TenantID)
	if err != nil {
		return fmt.Errorf("getting tenant by id %s: %w", message.TenantID, err)
	}

	ctx = tenant.SaveTenantInContext(ctx, t)

	if sendErr := h.service.SendPaymentsReadyToPay(ctx, paymentsReadyToPay); sendErr != nil {
		return fmt.Errorf("sending payments ready to pay: %w", sendErr)
	}

	return nil
}
