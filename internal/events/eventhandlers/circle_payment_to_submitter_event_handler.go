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
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/paymentdispatchers"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

type CirclePaymentToSubmitterEventHandlerOptions struct {
	AdminDBConnectionPool db.DBConnectionPool
	MtnDBConnectionPool   db.DBConnectionPool
	DistAccountResolver   signing.DistributionAccountResolver
	CircleService         circle.ServiceInterface
	CircleAPYType         circle.APIType
}

type CirclePaymentToSubmitterEventHandler struct {
	tenantManager       tenant.ManagerInterface
	service             services.PaymentToSubmitterServiceInterface
	distAccountResolver signing.DistributionAccountResolver
}

var _ events.EventHandler = new(CirclePaymentToSubmitterEventHandler)

func NewCirclePaymentToSubmitterEventHandler(opts CirclePaymentToSubmitterEventHandlerOptions) *CirclePaymentToSubmitterEventHandler {
	tm := tenant.NewManager(tenant.WithDatabase(opts.AdminDBConnectionPool))

	models, err := data.NewModels(opts.MtnDBConnectionPool)
	if err != nil {
		log.Fatalf("error getting models: %s", err.Error())
	}

	var circlePaymentDispatcher paymentdispatchers.PaymentDispatcherInterface
	if opts.CircleAPYType == circle.APITypePayouts {
		circlePaymentDispatcher = paymentdispatchers.NewCirclePaymentDispatcher(models, opts.CircleService, opts.DistAccountResolver)
	} else {
		circlePaymentDispatcher = paymentdispatchers.NewCirclePaymentTransferDispatcher(models, opts.CircleService, opts.DistAccountResolver)
	}

	s := services.NewPaymentToSubmitterService(services.PaymentToSubmitterServiceOptions{
		Models:              models,
		DistAccountResolver: opts.DistAccountResolver,
		PaymentDispatcher:   circlePaymentDispatcher,
	})

	return &CirclePaymentToSubmitterEventHandler{
		tenantManager:       tm,
		service:             s,
		distAccountResolver: opts.DistAccountResolver,
	}
}

func (h *CirclePaymentToSubmitterEventHandler) Name() string {
	return utils.GetTypeName(h)
}

func (h *CirclePaymentToSubmitterEventHandler) CanHandleMessage(ctx context.Context, message *events.Message) bool {
	return message.Topic == events.CirclePaymentReadyToPayTopic
}

func (h *CirclePaymentToSubmitterEventHandler) Handle(ctx context.Context, message *events.Message) error {
	paymentsReadyToPay, err := utils.ConvertType[any, schemas.EventPaymentsReadyToPayData](message.Data)
	if err != nil {
		return fmt.Errorf("could not convert message data to %T: %w", schemas.EventPaymentsReadyToPayData{}, err)
	}

	t, err := h.tenantManager.GetTenantByID(ctx, message.TenantID)
	if err != nil {
		return fmt.Errorf("getting tenant by id %s: %w", message.TenantID, err)
	}

	ctx = tenant.SaveTenantInContext(ctx, t)

	distAccount, err := h.distAccountResolver.DistributionAccountFromContext(ctx)
	if err != nil {
		return fmt.Errorf("getting distribution account: %w", err)
	}

	if !distAccount.Type.IsCircle() {
		log.Ctx(ctx).Debugf("distribution account is not a Circle account. Skipping for tenant %s", message.TenantID)
		return nil
	}

	if sendErr := h.service.SendPaymentsReadyToPay(ctx, paymentsReadyToPay); sendErr != nil {
		return fmt.Errorf("sending payments ready to pay: %w", sendErr)
	}

	return nil
}
