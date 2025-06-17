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

func Test_EmbeddedWalletFromSubmitterEventHandler_Handle(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	tenantManager := tenant.NewManager(tenant.WithDatabase(dbConnectionPool))

	service := servicesMocks.MockEmbeddedWalletFromSubmitterService{}

	handler := EmbeddedWalletFromSubmitterEventHandler{
		tenantManager: tenantManager,
		service:       &service,
	}

	ctx := context.Background()

	t.Run("logs and report error when message Data is invalid", func(t *testing.T) {
		handleErr := handler.Handle(ctx, &events.Message{Data: "invalid"})
		assert.ErrorContains(t, handleErr, "could not convert message data to schemas.EventWalletCreationCompletedData")
	})

	t.Run("logs and report error when fails getting tenant by ID", func(t *testing.T) {
		handleErr := handler.Handle(ctx, &events.Message{
			TenantID: "non-existent-tenant-id",
			Data: schemas.EventWalletCreationCompletedData{
				TransactionID: "tx-id",
			},
		})
		assert.ErrorIs(t, handleErr, tenant.ErrTenantDoesNotExist)
	})

	t.Run("logs and report error when service returns error", func(t *testing.T) {
		tenant.DeleteAllTenantsFixture(t, ctx, dbConnectionPool)

		tnt, err := tenantManager.AddTenant(ctx, "myorg1")
		require.NoError(t, err)

		txData := schemas.EventWalletCreationCompletedData{
			TransactionID: "tx-id",
		}

		ctxWithTenant := tenant.SaveTenantInContext(ctx, tnt)

		service.
			On("SyncTransaction", ctxWithTenant, txData.TransactionID).
			Return(errors.New("unexpected sync error")).
			Once()

		handleErr := handler.Handle(ctx, &events.Message{
			TenantID: tnt.ID,
			Data:     txData,
		})
		assert.ErrorContains(t, handleErr, "syncing transaction completion for transaction ID \"tx-id\"")
	})

	t.Run("successfully syncs the TSS transaction with the embedded wallet", func(t *testing.T) {
		tenant.DeleteAllTenantsFixture(t, ctx, dbConnectionPool)

		tnt, err := tenantManager.AddTenant(ctx, "myorg1")
		require.NoError(t, err)

		txData := schemas.EventWalletCreationCompletedData{
			TransactionID: "tx-id",
		}

		ctxWithTenant := tenant.SaveTenantInContext(ctx, tnt)

		service.
			On("SyncTransaction", ctxWithTenant, txData.TransactionID).
			Return(nil).
			Once()

		handleErr := handler.Handle(ctx, &events.Message{
			TenantID: tnt.ID,
			Data:     txData,
		})
		assert.NoError(t, handleErr)
	})

	service.AssertExpectations(t)
}

func Test_EmbeddedWalletFromSubmitterEventHandler_Name(t *testing.T) {
	handler := &EmbeddedWalletFromSubmitterEventHandler{}
	name := handler.Name()
	assert.Equal(t, "EmbeddedWalletFromSubmitterEventHandler", name)
}

func Test_EmbeddedWalletFromSubmitterEventHandler_CanHandleMessage(t *testing.T) {
	handler := &EmbeddedWalletFromSubmitterEventHandler{}
	ctx := context.Background()

	tests := []struct {
		name     string
		topic    string
		expected bool
	}{
		{
			name:     "can handle wallet creation completed topic",
			topic:    events.WalletCreationCompletedTopic,
			expected: true,
		},
		{
			name:     "cannot handle payment completed topic",
			topic:    events.PaymentCompletedTopic,
			expected: false,
		},
		{
			name:     "cannot handle receiver wallet invitation topic",
			topic:    events.ReceiverWalletNewInvitationTopic,
			expected: false,
		},
		{
			name:     "cannot handle payments ready to pay topic",
			topic:    events.PaymentReadyToPayTopic,
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			message := &events.Message{
				Topic: tc.topic,
			}
			canHandle := handler.CanHandleMessage(ctx, message)
			assert.Equal(t, tc.expected, canHandle)
		})
	}
}
