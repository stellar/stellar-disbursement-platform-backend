package eventhandlers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events/schemas"
	servicesMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/services/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

func Test_PatchAnchorPlatformTransactionCompletionEventHandler(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	tenantManager := tenant.NewManager(tenant.WithDatabase(dbConnectionPool))

	service := servicesMocks.MockPatchAnchorPlatformTransactionCompletionService{}

	handler := PatchAnchorPlatformTransactionCompletionEventHandler{
		tenantManager: tenantManager,
		service:       &service,
	}

	ctx := context.Background()
	t.Run("logs and report error when message Data is invalid", func(t *testing.T) {
		handleErr := handler.Handle(ctx, &events.Message{Data: "invalid"})
		assert.ErrorContains(t, handleErr, "could not convert data to schemas.EventPaymentCompletedData")
	})

	t.Run("logs and report error when fails getting tenant by ID", func(t *testing.T) {
		handleErr := handler.Handle(ctx, &events.Message{
			TenantID: "tenant-id",
			Data: schemas.EventPaymentCompletedData{
				PaymentID:            "payment-ID",
				PaymentStatus:        "SUCCESS",
				PaymentStatusMessage: "",
				StellarTransactionID: "tx-hash",
			},
		})
		assert.ErrorIs(t, handleErr, tenant.ErrTenantDoesNotExist)
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
			On("PatchAPTransactionForPaymentEvent", mock.Anything, tx).
			Return(errors.New("unexpected error")).
			Once()

		handleErr := handler.Handle(ctx, &events.Message{
			TenantID: tnt.ID,
			Data:     tx,
		})
		assert.EqualError(t, handleErr, "patching anchor platform transaction for payment event: unexpected error")
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
			On("PatchAPTransactionForPaymentEvent", mock.Anything, tx).
			Return(nil).
			Once()

		handleErr := handler.Handle(ctx, &events.Message{
			TenantID: tnt.ID,
			Data:     tx,
		})
		require.NoError(t, handleErr)
	})

	service.AssertExpectations(t)
}
