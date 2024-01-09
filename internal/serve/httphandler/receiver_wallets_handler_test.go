package httphandler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events/schemas"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_RetryInvitation(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)
	eventProducerMock := events.MockProducer{}
	tnt := tenant.Tenant{ID: "tenant-id"}

	handler := &ReceiverWalletsHandler{
		Models:        models,
		EventProducer: &eventProducerMock,
	}

	ctx := context.Background()

	r := chi.NewRouter()
	r.Patch("/receivers/wallets/{receiver_wallet_id}", handler.RetryInvitation)

	t.Run("returns error when tenant is not in the context", func(t *testing.T) {
		route := "/receivers/wallets/invalid_id"
		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, route, nil)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
		assert.JSONEq(t, `{ "error": "You don't have permission to perform this action." }`, rr.Body.String())
	})

	ctx = tenant.SaveTenantInContext(ctx, &tnt)

	t.Run("returns error when receiver wallet does not exist", func(t *testing.T) {
		route := "/receivers/wallets/invalid_id"
		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, route, nil)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		assert.JSONEq(t, `{ "error": "Resource not found." }`, rr.Body.String())
	})

	t.Run("successfuly retry invitation", func(t *testing.T) {
		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
		wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "wallet", "https://www.wallet.com", "www.wallet.com", "wallet://")
		rw := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.ReadyReceiversWalletStatus)

		eventProducerMock.
			On("WriteMessages", mock.Anything, []events.Message{
				{
					Topic:    events.ReceiverWalletSMSInvitationTopic,
					Key:      rw.ID,
					TenantID: tnt.ID,
					Type:     events.RetryReceiverWalletSMSInvitationType,
					Data: []schemas.EventReceiverWalletSMSInvitationData{
						{
							ReceiverWalletID: rw.ID,
						},
					},
				},
			}).
			Return(nil).
			Once()

		route := fmt.Sprintf("/receivers/wallets/%s", rw.ID)
		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, route, nil)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		wantJson := fmt.Sprintf(`{
			"id": %q,
			"receiver_id": %q,
			"wallet_id": %q,
			"created_at": %q,
			"invitation_sent_at": null
		}`, rw.ID, receiver.ID, wallet.ID, rw.CreatedAt.Format(time.RFC3339Nano))

		resp := rr.Result()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, wantJson, rr.Body.String())
	})

	t.Run("returns error when fails writing message on message broker", func(t *testing.T) {
		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
		wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "wallet", "https://www.wallet.com", "www.wallet.com", "wallet://")
		rw := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.ReadyReceiversWalletStatus)

		eventProducerMock.
			On("WriteMessages", mock.Anything, []events.Message{
				{
					Topic:    events.ReceiverWalletSMSInvitationTopic,
					Key:      rw.ID,
					TenantID: tnt.ID,
					Type:     events.RetryReceiverWalletSMSInvitationType,
					Data: []schemas.EventReceiverWalletSMSInvitationData{
						{
							ReceiverWalletID: rw.ID,
						},
					},
				},
			}).
			Return(errors.New("unexpected error")).
			Once()

		route := fmt.Sprintf("/receivers/wallets/%s", rw.ID)
		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, route, nil)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		assert.JSONEq(t, `{"error":"An internal error occurred while processing this request."}`, rr.Body.String())
	})
}
