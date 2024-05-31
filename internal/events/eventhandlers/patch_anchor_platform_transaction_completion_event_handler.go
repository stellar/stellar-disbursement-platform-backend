package eventhandlers

import (
	"context"
	"fmt"

	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/anchorplatform"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events/schemas"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

type PatchAnchorPlatformTransactionCompletionEventHandlerOptions struct {
	AdminDBConnectionPool db.DBConnectionPool
	MtnDBConnectionPool   db.DBConnectionPool
	APapiSvc              anchorplatform.AnchorPlatformAPIServiceInterface
}

type PatchAnchorPlatformTransactionCompletionEventHandler struct {
	tenantManager tenant.ManagerInterface
	service       services.PatchAnchorPlatformTransactionCompletionServiceInterface
}

var _ events.EventHandler = new(PatchAnchorPlatformTransactionCompletionEventHandler)

func NewPatchAnchorPlatformTransactionCompletionEventHandler(options PatchAnchorPlatformTransactionCompletionEventHandlerOptions) *PatchAnchorPlatformTransactionCompletionEventHandler {
	tm := tenant.NewManager(tenant.WithDatabase(options.AdminDBConnectionPool))

	models, err := data.NewModels(options.MtnDBConnectionPool)
	if err != nil {
		log.Fatalf("error getting models: %s", err.Error())
	}

	s, err := services.NewPatchAnchorPlatformTransactionCompletionService(options.APapiSvc, models)
	if err != nil {
		log.Fatalf("error instantiating service: %s", err.Error())
	}

	return &PatchAnchorPlatformTransactionCompletionEventHandler{
		tenantManager: tm,
		service:       s,
	}
}

func (h *PatchAnchorPlatformTransactionCompletionEventHandler) Name() string {
	return "PatchAnchorPlatformTransactionCompletionEventHandler"
}

func (h *PatchAnchorPlatformTransactionCompletionEventHandler) CanHandleMessage(ctx context.Context, message *events.Message) bool {
	return message.Topic == events.PaymentCompletedTopic
}

func (h *PatchAnchorPlatformTransactionCompletionEventHandler) Handle(ctx context.Context, message *events.Message) error {
	payment, err := utils.ConvertType[any, schemas.EventPaymentCompletedData](message.Data)
	if err != nil {
		return fmt.Errorf("could not convert data to %T: %w", schemas.EventPaymentCompletedData{}, err)
	}

	tnt, err := h.tenantManager.GetTenantByID(ctx, message.TenantID)
	if err != nil {
		return fmt.Errorf("getting tenant by id: %w", err)
	}

	ctx = tenant.SaveTenantInContext(ctx, tnt)

	err = h.service.PatchAPTransactionForPaymentEvent(ctx, payment)
	if err != nil {
		return fmt.Errorf("patching anchor platform transaction for payment event: %w", err)
	}
	return nil
}
