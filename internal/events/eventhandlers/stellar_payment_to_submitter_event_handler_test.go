package eventhandlers

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events/schemas"
	servicesMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/services/mocks"
	sigMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

func Test_PaymentToSubmitterEventHandler_Handle(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	tenantManager := tenant.NewManager(tenant.WithDatabase(dbConnectionPool))

	service := servicesMocks.MockPaymentToSubmitterService{}
	mDistAccResolver := sigMocks.NewMockDistributionAccountResolver(t)
	mDistAccResolver.
		On("DistributionAccountFromContext", mock.Anything).
		Return(schema.TransactionAccount{Type: schema.DistributionAccountStellarEnv}, nil).
		Maybe()

	handler := StellarPaymentToSubmitterEventHandler{
		tenantManager:       tenantManager,
		service:             &service,
		distAccountResolver: mDistAccResolver,
	}

	ctx := context.Background()
	t.Run("logs and report error when message Data is invalid", func(t *testing.T) {
		handleErr := handler.Handle(ctx, &events.Message{Data: "invalid"})
		assert.ErrorContains(t, handleErr, "could not convert message data to schemas.EventPaymentsReadyToPayData")
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
			On("SendPaymentsReadyToPay", ctxWithTenant, paymentsReadyToPay).
			Return(errors.New("unexpected error")).
			Once()

		handleErr := handler.Handle(ctx, &events.Message{
			TenantID: tnt.ID,
			Data:     paymentsReadyToPay,
		})
		assert.ErrorContains(t, handleErr, "sending payments ready to pay")
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
			On("SendPaymentsReadyToPay", ctxWithTenant, paymentsReadyToPay).
			Return(nil).
			Once()

		handleErr := handler.Handle(ctx, &events.Message{
			TenantID: tnt.ID,
			Data:     paymentsReadyToPay,
		})
		assert.NoError(t, handleErr)
	})

	service.AssertExpectations(t)
}
