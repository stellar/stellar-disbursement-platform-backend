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
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type PaymentFromSubmitterServiceMock struct {
	mock.Mock
}

var _ services.PaymentFromSubmitterServiceInterface = new(PaymentFromSubmitterServiceMock)

func (s *PaymentFromSubmitterServiceMock) SyncTransaction(ctx context.Context, tx *schemas.EventPaymentCompletedData) error {
	args := s.Called(ctx, tx)
	return args.Error(0)
}

func Test_PaymentFromSubmitterEventHandler_Handle(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	tenantManager := tenant.NewManager(tenant.WithDatabase(dbConnectionPool))

	crashTrackerClient := crashtracker.MockCrashTrackerClient{}
	service := PaymentFromSubmitterServiceMock{}

	handler := PaymentFromSubmitterEventHandler{
		tenantManager:      tenantManager,
		crashTrackerClient: &crashTrackerClient,
		service:            &service,
	}

	ctx := context.Background()
	t.Run("logs and report error when message Data is invalid", func(t *testing.T) {
		crashTrackerClient.
			On("LogAndReportErrors", ctx, mock.Anything, "[PaymentFromSubmitterEventHandler] could not convert data to schemas.EventPaymentCompletedData: invalid").
			Return().
			Once()

		handler.Handle(ctx, &events.Message{Data: "invalid"})
	})

	t.Run("logs and report error when fails getting tenant by ID", func(t *testing.T) {
		crashTrackerClient.
			On("LogAndReportErrors", ctx, tenant.ErrTenantDoesNotExist, "[PaymentFromSubmitterEventHandler] error getting tenant by id").
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

		tx := schemas.EventPaymentCompletedData{
			TransactionID: "tx-id",
		}

		ctxWithTenant := tenant.SaveTenantInContext(ctx, tnt)

		service.
			On("SyncTransaction", ctxWithTenant, &tx).
			Return(errors.New("unexpected error")).
			Once()

		crashTrackerClient.
			On("LogAndReportErrors", ctxWithTenant, errors.New("unexpected error"), `[PaymentFromSubmitterEventHandler] synching transaction completion for transaction ID "tx-id"`).
			Return().
			Once()

		handler.Handle(ctx, &events.Message{
			TenantID: tnt.ID,
			Data:     tx,
		})
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

		handler.Handle(ctx, &events.Message{
			TenantID: tnt.ID,
			Data:     tx,
		})
	})

	crashTrackerClient.AssertExpectations(t)
	service.AssertExpectations(t)
}
