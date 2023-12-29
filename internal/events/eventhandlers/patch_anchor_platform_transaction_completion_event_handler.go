package eventhandlers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/anchorplatform"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
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
	s, err := services.NewPatchAnchorPlatformTransactionCompletionService(options.APapiSvc, nil)
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
	return message.Topic == events.PatchAnchorPlatformTransactionCompletionTopic
}

func (h *PatchAnchorPlatformTransactionCompletionEventHandler) Handle(ctx context.Context, message *events.Message) {
	dataJSON, err := json.Marshal(message.Data)
	if err != nil {
		h.crashTrackerClient.LogAndReportErrors(ctx, err, fmt.Sprintf("[PatchAnchorPlatformTransactionCompletionEventHandler] could not marshal data: %v", message.Data))
		return
	}

	var req services.PatchAnchorPlatformTransactionCompletionReq
	err = json.Unmarshal(dataJSON, &req)
	if err != nil {
		h.crashTrackerClient.LogAndReportErrors(ctx, err, fmt.Sprintf("[PatchAnchorPlatformTransactionCompletionEventHandler] could not unmarshal data: %v", message.Data))
		return
	}

	t, err := h.tenantManager.GetTenantByID(ctx, message.TenantID)
	if err != nil {
		h.crashTrackerClient.LogAndReportErrors(ctx, err, "[PatchAnchorPlatformTransactionCompletionEventHandler] error getting tenant by id")
		return
	}

	dsn, err := h.tenantManager.GetDSNForTenant(ctx, t.Name)
	if err != nil {
		h.crashTrackerClient.LogAndReportErrors(ctx, err, fmt.Sprintf("[PatchAnchorPlatformTransactionCompletionEventHandler] error getting DSN for tenant %s", t.Name))
		return
	}

	dbConnectionPool, err := db.OpenDBConnectionPool(dsn)
	if err != nil {
		h.crashTrackerClient.LogAndReportErrors(ctx, err, fmt.Sprintf("[PatchAnchorPlatformTransactionCompletionEventHandler] error opening DB Connection pool for tenant %s", t.Name))
		return
	}
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	if err != nil {
		h.crashTrackerClient.LogAndReportErrors(ctx, err, "[PatchAnchorPlatformTransactionCompletionEventHandler] error getting models")
		return
	}

	h.service.SetModels(models)
	if err := h.service.PatchTransactionCompletion(ctx, services.PatchAnchorPlatformTransactionCompletionReq{PaymentID: req.PaymentID}); err != nil {
		h.crashTrackerClient.LogAndReportErrors(ctx, err, "[PatchAnchorPlatformTransactionCompletionEventHandler] patching anchor platform transaction")
		return
	}
}
