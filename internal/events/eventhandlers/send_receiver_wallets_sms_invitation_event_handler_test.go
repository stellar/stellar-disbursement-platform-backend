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

	service := servicesMocks.MockSendReceiverWalletInviteService{}

	handler := SendReceiverWalletsSMSInvitationEventHandler{
		tenantManager:       tenantManager,
		mtnDBConnectionPool: mtnDBConnectionPool,
		service:             &service,
	}

	ctx := context.Background()
	t.Run("logs and report error when message Data is invalid", func(t *testing.T) {
		handleErr := handler.Handle(ctx, &events.Message{Data: "invalid"})
		assert.ErrorContains(t, handleErr, "could not convert message data to []schemas.EventReceiverWalletSMSInvitationData")
	})

	t.Run("logs and report error when fails getting tenant by ID", func(t *testing.T) {
		handleErr := handler.Handle(ctx, &events.Message{
			TenantID: "tenant-id",
			Data: []schemas.EventReceiverWalletSMSInvitationData{
				{ReceiverWalletID: "rw-id-1"},
				{ReceiverWalletID: "rw-id-2"},
			},
		})
		assert.ErrorIs(t, handleErr, tenant.ErrTenantDoesNotExist)
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

		handleErr := handler.Handle(ctx, &events.Message{
			TenantID: tnt.ID,
			Data:     reqs,
		})
		assert.ErrorContains(t, handleErr, "sending receiver wallets invitation")
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

		handleErr := handler.Handle(ctx, &events.Message{
			TenantID: tnt.ID,
			Data:     reqs,
		})
		assert.NoError(t, handleErr)
	})

	service.AssertExpectations(t)
}

func Test_SendReceiverWalletsSMSInvitationEventHandler_CanHandleMessage(t *testing.T) {
	ctx := context.Background()
	handler := SendReceiverWalletsSMSInvitationEventHandler{}

	assert.False(t, handler.CanHandleMessage(ctx, &events.Message{Topic: "some-topic"}))
	assert.True(t, handler.CanHandleMessage(ctx, &events.Message{Topic: events.ReceiverWalletNewInvitationTopic}))
}
