package eventhandlers

import (
	"context"
	"errors"
	"testing"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type PatchAnchorPlatformTransactionCompletionServiceMock struct {
	mock.Mock
}

var _ services.PatchAnchorPlatformTransactionCompletionServiceInterface = new(PatchAnchorPlatformTransactionCompletionServiceMock)

func (s *PatchAnchorPlatformTransactionCompletionServiceMock) PatchTransactionCompletion(ctx context.Context, req services.PatchAnchorPlatformTransactionCompletionReq) error {
	args := s.Called(ctx, req)
	return args.Error(0)
}

func (s *PatchAnchorPlatformTransactionCompletionServiceMock) SetModels(models *data.Models) {
	s.Called(models)
}

func Test_PatchAnchorPlatformTransactionCompletionEventHandler(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	tenantManager := tenant.NewManager(tenant.WithDatabase(dbConnectionPool))
	crashTrackerClient := crashtracker.MockCrashTrackerClient{}
	service := PatchAnchorPlatformTransactionCompletionServiceMock{}

	handler := PatchAnchorPlatformTransactionCompletionEventHandler{
		tenantManager:      tenantManager,
		crashTrackerClient: &crashTrackerClient,
		service:            &service,
	}

	ctx := context.Background()
	t.Run("logs and report error when message Data is invalid", func(t *testing.T) {
		crashTrackerClient.
			On("LogAndReportErrors", ctx, mock.AnythingOfType("*json.UnmarshalTypeError"), "[PatchAnchorPlatformTransactionCompletionEventHandler] could not unmarshal data: invalid").
			Return().
			Once()

		handler.Handle(ctx, &events.Message{Data: "invalid"})
	})

	t.Run("logs and report error when fails getting tenant by ID", func(t *testing.T) {
		crashTrackerClient.
			On("LogAndReportErrors", ctx, tenant.ErrTenantDoesNotExist, "[PatchAnchorPlatformTransactionCompletionEventHandler] error getting tenant by id").
			Return().
			Once()

		handler.Handle(ctx, &events.Message{
			TenantID: "tenant-id",
			Data:     services.PatchAnchorPlatformTransactionCompletionReq{PaymentID: "payment-ID"},
		})
	})

	t.Run("logs and report error when service returns error", func(t *testing.T) {
		tenant.DeleteAllTenantsFixture(t, ctx, dbConnectionPool)

		tnt, err := tenantManager.AddTenant(ctx, "myorg1")
		require.NoError(t, err)

		req := services.PatchAnchorPlatformTransactionCompletionReq{PaymentID: "payment-ID"}

		service.
			On("SetModels", mock.AnythingOfType("*data.Models")).
			On("PatchTransactionCompletion", ctx, req).
			Return(errors.New("unexpected error")).
			Once()

		crashTrackerClient.
			On("LogAndReportErrors", ctx, errors.New("unexpected error"), "[PatchAnchorPlatformTransactionCompletionEventHandler] patching anchor platform transaction").
			Return().
			Once()

		handler.Handle(ctx, &events.Message{
			TenantID: tnt.ID,
			Data:     req,
		})
	})

	t.Run("successfully patch anchor platform transaction completion", func(t *testing.T) {
		tenant.DeleteAllTenantsFixture(t, ctx, dbConnectionPool)

		tnt, err := tenantManager.AddTenant(ctx, "myorg1")
		require.NoError(t, err)

		req := services.PatchAnchorPlatformTransactionCompletionReq{PaymentID: "payment-ID"}

		service.
			On("SetModels", mock.AnythingOfType("*data.Models")).
			On("PatchTransactionCompletion", ctx, req).
			Return(nil).
			Once()

		handler.Handle(ctx, &events.Message{
			TenantID: tnt.ID,
			Data:     req,
		})
	})

	crashTrackerClient.AssertExpectations(t)
	service.AssertExpectations(t)
}
