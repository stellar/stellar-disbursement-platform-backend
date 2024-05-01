package eventhandlers

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events/schemas"
	servicesMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/services/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

func Test_PaymentFromSubmitterEventHandler_Handle(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	tenantManager := tenant.NewManager(tenant.WithDatabase(dbConnectionPool))

	service := servicesMocks.MockPaymentFromSubmitterService{}

	handler := PaymentFromSubmitterEventHandler{
		tenantManager: tenantManager,
		service:       &service,
	}

	ctx := context.Background()
	t.Run("logs and report error when message Data is invalid", func(t *testing.T) {
		handleErr := handler.Handle(ctx, &events.Message{Data: "invalid"})
		assert.ErrorContains(t, handleErr, "could not convert message data to schemas.EventPaymentCompletedData")
	})

	t.Run("logs and report error when fails getting tenant by ID", func(t *testing.T) {
		handleErr := handler.Handle(ctx, &events.Message{
			TenantID: "tenant-id",
			Data: schemas.EventPaymentCompletedData{
				TransactionID: "tx-id",
			},
		})
		assert.ErrorIs(t, handleErr, tenant.ErrTenantDoesNotExist)
	})

	t.Run("logs and report error when service returns error", func(t *testing.T) {
		tenant.DeleteAllTenantsFixture(t, ctx, dbConnectionPool)

		tnt, err := tenantManager.AddTenant(ctx, "myorg1")
		require.NoError(t, err)

		tx := schemas.EventPaymentCompletedData{
			TransactionID: "tx-id",
		}

		ctxWithTenant := tenant.SaveTenantInContext(ctx, tnt)

		service.
			On("SyncTransaction", ctxWithTenant, &tx).
			Return(errors.New("unexpected error")).
			Once()

		handleErr := handler.Handle(ctx, &events.Message{
			TenantID: tnt.ID,
			Data:     tx,
		})
		assert.ErrorContains(t, handleErr, "syncing transaction completion for transaction ID \"tx-id\"")
	})

	t.Run("successfully syncs the TSS transaction with the SDP's payment", func(t *testing.T) {
		tenant.DeleteAllTenantsFixture(t, ctx, dbConnectionPool)

		tnt, err := tenantManager.AddTenant(ctx, "myorg1")
		require.NoError(t, err)

		tx := schemas.EventPaymentCompletedData{
			TransactionID: "tx-id",
		}

		ctxWithTenant := tenant.SaveTenantInContext(ctx, tnt)

		service.
			On("SyncTransaction", ctxWithTenant, &tx).
			Return(nil).
			Once()

		handleErr := handler.Handle(ctx, &events.Message{
			TenantID: tnt.ID,
			Data:     tx,
		})
		assert.NoError(t, handleErr)
	})

	service.AssertExpectations(t)
}
