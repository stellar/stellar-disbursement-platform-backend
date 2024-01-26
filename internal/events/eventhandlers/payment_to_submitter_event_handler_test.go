package eventhandlers

import (
	"context"
	"errors"
	"testing"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events/schemas"
	servicesmocks "github.com/stellar/stellar-disbursement-platform-backend/internal/services/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_PaymentToSubmitterEventHandler_Handle(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	tenantManager := tenant.NewManager(tenant.WithDatabase(dbConnectionPool))

	crashTrackerClient := crashtracker.MockCrashTrackerClient{}
	service := servicesmocks.MockPaymentToSubmitterService{}

	handler := PaymentToSubmitterEventHandler{
		tenantManager:      tenantManager,
		crashTrackerClient: &crashTrackerClient,
		service:            &service,
	}

	ctx := context.Background()
	t.Run("logs and report error when message Data is invalid", func(t *testing.T) {
		crashTrackerClient.
			On("LogAndReportErrors", ctx, mock.Anything, "[PaymentToSubmitterEventHandler] could not convert data to schemas.EventPaymentsReadyToPayData: invalid").
			Return().
			Once()

		handler.Handle(ctx, &events.Message{Data: "invalid"})
	})

	t.Run("logs and report error when fails getting tenant by ID", func(t *testing.T) {
		crashTrackerClient.
			On("LogAndReportErrors", ctx, tenant.ErrTenantDoesNotExist, "[PaymentToSubmitterEventHandler] error getting tenant by id").
			Return().
			Once()

		handler.Handle(ctx, &events.Message{
			TenantID: "tenant-id",
			Data: schemas.EventPaymentCompletedData{
				TransactionID: "tx-id",
			},
		})
	})

	t.Run("logs and report error when service returns error", func(t *testing.T) {
		tenant.DeleteAllTenantsFixture(t, ctx, dbConnectionPool)

		tnt, err := tenantManager.AddTenant(ctx, "myorg1")
		require.NoError(t, err)

		paymentsReadyToPay := schemas.EventPaymentsReadyToPayData{
			TenantID: tnt.ID,
			Payments: []schemas.PaymentReadyToPay{
				{
					ID: "payment-id",
				},
			},
		}

		ctxWithTenant := tenant.SaveTenantInContext(ctx, tnt)

		service.
			On("SendPaymentsReadyToPay", ctxWithTenant, &paymentsReadyToPay).
			Return(errors.New("unexpected error")).
			Once()

		crashTrackerClient.
			On("LogAndReportErrors", ctxWithTenant, errors.New("unexpected error"), `[PaymentToSubmitterEventHandler] send payments ready to pay: [{payment-id}]`).
			Return().
			Once()

		handler.Handle(ctx, &events.Message{
			TenantID: tnt.ID,
			Data:     paymentsReadyToPay,
		})
	})

	t.Run("successfully sends payments ready to pay to TSS", func(t *testing.T) {
		tenant.DeleteAllTenantsFixture(t, ctx, dbConnectionPool)

		tnt, err := tenantManager.AddTenant(ctx, "myorg1")
		require.NoError(t, err)

		paymentsReadyToPay := schemas.EventPaymentsReadyToPayData{
			TenantID: tnt.ID,
			Payments: []schemas.PaymentReadyToPay{
				{
					ID: "payment-id",
				},
			},
		}

		ctxWithTenant := tenant.SaveTenantInContext(ctx, tnt)

		service.
			On("SendPaymentsReadyToPay", ctxWithTenant, &paymentsReadyToPay).
			Return(nil).
			Once()

		handler.Handle(ctx, &events.Message{
			TenantID: tnt.ID,
			Data:     paymentsReadyToPay,
		})
	})

	crashTrackerClient.AssertExpectations(t)
	service.AssertExpectations(t)
}
