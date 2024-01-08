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
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events/schemas"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type SendReceiverWalletInviteServiceMock struct {
	mock.Mock
}

var _ services.SendReceiverWalletInviteServiceInterface = new(SendReceiverWalletInviteServiceMock)

func (s *SendReceiverWalletInviteServiceMock) SendInvite(ctx context.Context, receiverWalletsReq ...schemas.EventReceiverWalletSMSInvitationData) error {
	args := s.Called(ctx, receiverWalletsReq)
	return args.Error(0)
}

func (s *SendReceiverWalletInviteServiceMock) SetModels(models *data.Models) {
	s.Called(models)
}

func Test_SendReceiverWalletsSMSInvitationEventHandler_Handle(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	tenantManager := tenant.NewManager(tenant.WithDatabase(dbConnectionPool))
	crashTrackerClient := crashtracker.MockCrashTrackerClient{}
	service := SendReceiverWalletInviteServiceMock{}

	handler := SendReceiverWalletsSMSInvitationEventHandler{
		tenantManager:      tenantManager,
		crashTrackerClient: &crashTrackerClient,
		service:            &service,
	}

	ctx := context.Background()
	t.Run("logs and report error when message Data is invalid", func(t *testing.T) {
		crashTrackerClient.
			On("LogAndReportErrors", ctx, mock.AnythingOfType("*json.UnmarshalTypeError"), "[SendReceiverWalletsSMSInvitationEventHandler] could not unmarshal data: invalid").
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

		service.
			On("SetModels", mock.AnythingOfType("*data.Models")).
			Return().
			On("SendInvite", ctx, reqs).
			Return(errors.New("unexpected error")).
			Once()

		crashTrackerClient.
			On("LogAndReportErrors", ctx, errors.New("unexpected error"), "[SendReceiverWalletsSMSInvitationEventHandler] sending receiver wallets invitation").
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

		service.
			On("SetModels", mock.AnythingOfType("*data.Models")).
			Return().
			On("SendInvite", ctx, reqs).
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
	assert.True(t, handler.CanHandleMessage(ctx, &events.Message{Topic: events.ReceiverWalletSMSInvitationTopic}))
}
