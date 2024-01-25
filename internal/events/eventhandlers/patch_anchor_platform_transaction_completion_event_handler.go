package eventhandlers

import (
	"context"
	"fmt"

	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/anchorplatform"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events/schemas"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/router"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

type PatchAnchorPlatformTransactionCompletionEventHandlerOptions struct {
	DBConnectionPool   db.DBConnectionPool
	APapiSvc           anchorplatform.AnchorPlatformAPIServiceInterface
	CrashTrackerClient crashtracker.CrashTrackerClient
}

type PatchAnchorPlatformTransactionCompletionEventHandler struct {
	tenantManager      tenant.ManagerInterface
	service            services.PatchAnchorPlatformTransactionCompletionServiceInterface
	crashTrackerClient crashtracker.CrashTrackerClient
}

var _ events.EventHandler = new(PatchAnchorPlatformTransactionCompletionEventHandler)

func NewPatchAnchorPlatformTransactionCompletionEventHandler(options PatchAnchorPlatformTransactionCompletionEventHandlerOptions) *PatchAnchorPlatformTransactionCompletionEventHandler {
	tm := tenant.NewManager(tenant.WithDatabase(options.DBConnectionPool))
	tr := router.NewMultiTenantDataSourceRouter(tm)
	mtnDBConnectionPool, err := db.NewConnectionPoolWithRouter(tr)
	if err != nil {
		log.Fatalf("error getting tenant DB Connection Pool: %s", err.Error())
	}

	models, err := data.NewModels(mtnDBConnectionPool)
	if err != nil {
		log.Fatalf("error getting models: %s", err.Error())
	}

	s, err := services.NewPatchAnchorPlatformTransactionCompletionService(options.APapiSvc, models)
	if err != nil {
		log.Fatalf("error instantiating service: %s", err.Error())
	}

	return &PatchAnchorPlatformTransactionCompletionEventHandler{
		tenantManager:      tenant.NewManager(tenant.WithDatabase(options.DBConnectionPool)),
		service:            s,
		crashTrackerClient: options.CrashTrackerClient,
	}
}

func (h *PatchAnchorPlatformTransactionCompletionEventHandler) Name() string {
	return "PatchAnchorPlatformTransactionCompletionEventHandler"
}

func (h *PatchAnchorPlatformTransactionCompletionEventHandler) CanHandleMessage(ctx context.Context, message *events.Message) bool {
	return message.Topic == events.PaymentCompletedTopic
}

func (h *PatchAnchorPlatformTransactionCompletionEventHandler) Handle(ctx context.Context, message *events.Message) {
	payment, err := utils.ConvertType[any, schemas.EventPaymentCompletedData](message.Data)
	if err != nil {
		h.crashTrackerClient.LogAndReportErrors(ctx, err, fmt.Sprintf("[PatchAnchorPlatformTransactionCompletionEventHandler] could not convert data to %T: %v", schemas.EventPaymentCompletedData{}, message.Data))
		return
	}

	tnt, err := h.tenantManager.GetTenantByID(ctx, message.TenantID)
	if err != nil {
		h.crashTrackerClient.LogAndReportErrors(ctx, err, "[PatchAnchorPlatformTransactionCompletionEventHandler] error getting tenant by id")
		return
	}

	ctx = tenant.SaveTenantInContext(ctx, tnt)

	if err := h.service.PatchTransactionCompletion(ctx, payment); err != nil {
		h.crashTrackerClient.LogAndReportErrors(ctx, err, "[PatchAnchorPlatformTransactionCompletionEventHandler] patching anchor platform transaction")
		return
	}
}
