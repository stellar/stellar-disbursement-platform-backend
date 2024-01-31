package eventhandlers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events/schemas"
	servicesMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/services/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_PatchAnchorPlatformTransactionCompletionEventHandler(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	tenantManager := tenant.NewManager(tenant.WithDatabase(dbConnectionPool))
	crashTrackerClient := crashtracker.MockCrashTrackerClient{}
	service := servicesMocks.MockPatchAnchorPlatformTransactionCompletionService{}

	handler := PatchAnchorPlatformTransactionCompletionEventHandler{
		tenantManager:      tenantManager,
		crashTrackerClient: &crashTrackerClient,
		service:            &service,
	}

	ctx := context.Background()
	t.Run("logs and report error when message Data is invalid", func(t *testing.T) {
		crashTrackerClient.
			On("LogAndReportErrors", ctx, mock.Anything, "[PatchAnchorPlatformTransactionCompletionEventHandler] could not convert data to schemas.EventPaymentCompletedData: invalid").
			Return().
			Once()

		handler.Handle(ctx, &events.Message{Data: "invalid"})
	})

	t.Run("logs and report error when fails getting tenant by ID", func(t *testing.T) {
		crashTrackerClient.
			On("LogAndReportErrors", mock.Anything, tenant.ErrTenantDoesNotExist, "[PatchAnchorPlatformTransactionCompletionEventHandler] error getting tenant by id").
			Return().
			Once()

		handler.Handle(ctx, &events.Message{
			TenantID: "tenant-id",
			Data: schemas.EventPaymentCompletedData{
				PaymentID:            "payment-ID",
				PaymentStatus:        "SUCCESS",
				PaymentStatusMessage: "",
				StellarTransactionID: "tx-hash",
			},
		})
	})

	t.Run("logs and report error when service returns error", func(t *testing.T) {
		tenant.DeleteAllTenantsFixture(t, ctx, dbConnectionPool)

		tnt, err := tenantManager.AddTenant(ctx, "myorg1")
		require.NoError(t, err)

		tx := schemas.EventPaymentCompletedData{
			PaymentID:            "payment-ID",
			PaymentStatus:        "SUCCESS",
			PaymentStatusMessage: "",
			PaymentCompletedAt:   time.Now().UTC(),
			StellarTransactionID: "tx-hash",
		}

		service.
			On("PatchTransactionCompletion", mock.Anything, tx).
			Return(errors.New("unexpected error")).
			Once()

		crashTrackerClient.
			On("LogAndReportErrors", mock.Anything, errors.New("unexpected error"), "[PatchAnchorPlatformTransactionCompletionEventHandler] patching anchor platform transaction").
			Return().
			Once()

		handler.Handle(ctx, &events.Message{
			TenantID: tnt.ID,
			Data:     tx,
		})
	})

	t.Run("successfully patch anchor platform transaction completion", func(t *testing.T) {
		tenant.DeleteAllTenantsFixture(t, ctx, dbConnectionPool)

		tnt, err := tenantManager.AddTenant(ctx, "myorg1")
		require.NoError(t, err)

		tx := schemas.EventPaymentCompletedData{
			PaymentID:            "payment-ID",
			PaymentStatus:        "SUCCESS",
			PaymentStatusMessage: "",
			PaymentCompletedAt:   time.Now().UTC(),
			StellarTransactionID: "tx-hash",
		}

		service.
			On("PatchTransactionCompletion", mock.Anything, tx).
			Return(nil).
			Once()

		handler.Handle(ctx, &events.Message{
			TenantID: tnt.ID,
			Data:     tx,
		})
	})

	crashTrackerClient.AssertExpectations(t)
	service.AssertExpectations(t)
}
