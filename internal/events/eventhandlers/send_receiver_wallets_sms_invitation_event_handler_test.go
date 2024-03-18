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
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events/schemas"
	servicesMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/services/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

func Test_SendReceiverWalletsSMSInvitationEventHandler_Handle(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	tenantManager := tenant.NewManager(tenant.WithDatabase(dbConnectionPool))
	tenantRouter := tenant.NewMultiTenantDataSourceRouter(tenantManager)
	mtnDBConnectionPool, err := db.NewConnectionPoolWithRouter(tenantRouter)
	require.NoError(t, err)

	crashTrackerClient := crashtracker.MockCrashTrackerClient{}
	service := servicesMocks.MockSendReceiverWalletInviteService{}

	handler := SendReceiverWalletsSMSInvitationEventHandler{
		tenantManager:       tenantManager,
		mtnDBConnectionPool: mtnDBConnectionPool,
		crashTrackerClient:  &crashTrackerClient,
		service:             &service,
	}

	ctx := context.Background()
	t.Run("logs and report error when message Data is invalid", func(t *testing.T) {
		crashTrackerClient.
			On("LogAndReportErrors", ctx, mock.Anything, "[SendReceiverWalletsSMSInvitationEventHandler] could not convert data to []schemas.EventReceiverWalletSMSInvitationData: invalid").
			Return().
			Once()

		handler.Handle(ctx, &events.Message{Data: "invalid"})
	})

	t.Run("logs and report error when fails getting tenant by ID", func(t *testing.T) {
		crashTrackerClient.
			On("LogAndReportErrors", ctx, tenant.ErrTenantDoesNotExist, "[SendReceiverWalletsSMSInvitationEventHandler] error getting tenant by id").
			Return().
			Once()

		handler.Handle(ctx, &events.Message{
			TenantID: "tenant-id",
			Data: []schemas.EventReceiverWalletSMSInvitationData{
				{ReceiverWalletID: "rw-id-1"},
				{ReceiverWalletID: "rw-id-2"},
			},
		})
	})

	t.Run("logs and report error when service returns error", func(t *testing.T) {
		tenant.DeleteAllTenantsFixture(t, ctx, dbConnectionPool)

		tnt, err := tenantManager.AddTenant(ctx, "myorg1")
		require.NoError(t, err)

		reqs := []schemas.EventReceiverWalletSMSInvitationData{
			{ReceiverWalletID: "rw-id-1"},
			{ReceiverWalletID: "rw-id-2"},
		}

		ctxWithTenant := tenant.SaveTenantInContext(ctx, tnt)

		service.
			On("SendInvite", ctxWithTenant, reqs).
			Return(errors.New("unexpected error")).
			Once()

		crashTrackerClient.
			On("LogAndReportErrors", ctxWithTenant, errors.New("unexpected error"), "[SendReceiverWalletsSMSInvitationEventHandler] sending receiver wallets invitation").
			Return().
			Once()

		handler.Handle(ctx, &events.Message{
			TenantID: tnt.ID,
			Data:     reqs,
		})
	})

	t.Run("successfully send invitation to the receivers", func(t *testing.T) {
		tenant.DeleteAllTenantsFixture(t, ctx, dbConnectionPool)

		tnt, err := tenantManager.AddTenant(ctx, "myorg1")
		require.NoError(t, err)

		reqs := []schemas.EventReceiverWalletSMSInvitationData{
			{ReceiverWalletID: "rw-id-1"},
			{ReceiverWalletID: "rw-id-2"},
		}

		ctxWithTenant := tenant.SaveTenantInContext(ctx, tnt)

		service.
			On("SendInvite", ctxWithTenant, reqs).
			Return(nil).
			Once()

		handler.Handle(ctx, &events.Message{
			TenantID: tnt.ID,
			Data:     reqs,
		})
	})

	crashTrackerClient.AssertExpectations(t)
	service.AssertExpectations(t)
}

func Test_SendReceiverWalletsSMSInvitationEventHandler_CanHandleMessage(t *testing.T) {
	ctx := context.Background()
	handler := SendReceiverWalletsSMSInvitationEventHandler{}

	assert.False(t, handler.CanHandleMessage(ctx, &events.Message{Topic: "some-topic"}))
	assert.True(t, handler.CanHandleMessage(ctx, &events.Message{Topic: events.ReceiverWalletNewInvitationTopic}))
}
