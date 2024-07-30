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
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/paymentdispatchers"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	txSubStore "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

type StellarPaymentToSubmitterEventHandlerOptions struct {
	AdminDBConnectionPool db.DBConnectionPool
	MtnDBConnectionPool   db.DBConnectionPool
	TSSDBConnectionPool   db.DBConnectionPool
	DistAccountResolver   signing.DistributionAccountResolver
}

type StellarPaymentToSubmitterEventHandler struct {
	tenantManager       tenant.ManagerInterface
	service             services.PaymentToSubmitterServiceInterface
	distAccountResolver signing.DistributionAccountResolver
}

var _ events.EventHandler = new(StellarPaymentToSubmitterEventHandler)

func NewStellarPaymentToSubmitterEventHandler(opts StellarPaymentToSubmitterEventHandlerOptions) *StellarPaymentToSubmitterEventHandler {
	tm := tenant.NewManager(tenant.WithDatabase(opts.AdminDBConnectionPool))

	models, err := data.NewModels(opts.MtnDBConnectionPool)
	if err != nil {
		log.Fatalf("error getting models: %s", err.Error())
	}

	stellarPaymentDispatcher := paymentdispatchers.NewStellarPaymentDispatcher(
		models,
		txSubStore.NewTransactionModel(opts.TSSDBConnectionPool),
		opts.DistAccountResolver)

	s := services.NewPaymentToSubmitterService(services.PaymentToSubmitterServiceOptions{
		Models:              models,
		DistAccountResolver: opts.DistAccountResolver,
		PaymentDispatcher:   stellarPaymentDispatcher,
	})

	return &StellarPaymentToSubmitterEventHandler{
		tenantManager:       tm,
		service:             s,
		distAccountResolver: opts.DistAccountResolver,
	}
}

func (h *StellarPaymentToSubmitterEventHandler) Name() string {
	return utils.GetTypeName(h)
}

func (h *StellarPaymentToSubmitterEventHandler) CanHandleMessage(ctx context.Context, message *events.Message) bool {
	return message.Topic == events.PaymentReadyToPayTopic
}

func (h *StellarPaymentToSubmitterEventHandler) Handle(ctx context.Context, message *events.Message) error {
	paymentsReadyToPay, err := utils.ConvertType[any, schemas.EventPaymentsReadyToPayData](message.Data)
	if err != nil {
		return fmt.Errorf("could not convert message data to %T: %w", schemas.EventPaymentsReadyToPayData{}, err)
	}

	// Save tenant in context
	t, err := h.tenantManager.GetTenantByID(ctx, message.TenantID)
	if err != nil {
		return fmt.Errorf("getting tenant by id %s: %w", message.TenantID, err)
	}
	ctx = tenant.SaveTenantInContext(ctx, t)

	// Bypass Sending payments if tenant doesn't have a Stellar account
	distAccount, err := h.distAccountResolver.DistributionAccountFromContext(ctx)
	if err != nil {
		return fmt.Errorf("getting distribution account: %w", err)
	}

	if !distAccount.Type.IsStellar() {
		log.Ctx(ctx).Debugf("distribution account is not a Stellar account. Skipping for tenant %s", message.TenantID)
		return nil
	}

	if sendErr := h.service.SendPaymentsReadyToPay(ctx, paymentsReadyToPay); sendErr != nil {
		return fmt.Errorf("sending payments ready to pay: %w", sendErr)
	}

	return nil
}
