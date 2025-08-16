package httphandler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stellar/go/support/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events/schemas"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

func Test_RetryInvitation(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)
	tnt := schema.Tenant{ID: "tenant-id"}
	ctx := tenant.SaveTenantInContext(context.Background(), &tnt)

	t.Run("returns error when receiver wallet does not exist", func(t *testing.T) {
		handler := ReceiverWalletsHandler{Models: models}
		r := chi.NewRouter()
		r.Patch("/receivers/wallets/{receiver_wallet_id}", handler.RetryInvitation)

		route := "/receivers/wallets/invalid_id"
		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, route, nil)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		assert.JSONEq(t, `{ "error": "Resource not found." }`, rr.Body.String())
	})

	t.Run("returns error when tenant is not in the context", func(t *testing.T) {
		handler := ReceiverWalletsHandler{Models: models}
		r := chi.NewRouter()
		r.Patch("/receivers/wallets/{receiver_wallet_id}", handler.RetryInvitation)

		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
		wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "wallet", "https://www.wallet.com", "www.wallet.com", "wallet://")
		rw := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.ReadyReceiversWalletStatus)

		route := fmt.Sprintf("/receivers/wallets/%s", rw.ID)
		req, err := http.NewRequestWithContext(context.Background(), http.MethodPatch, route, nil)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
		assert.JSONEq(t, `{ "error": "You don't have permission to perform this action." }`, rr.Body.String())
	})

	t.Run("successfuly retry invitation", func(t *testing.T) {
		eventProducerMock := events.NewMockProducer(t)
		handler := ReceiverWalletsHandler{
			Models:        models,
			EventProducer: eventProducerMock,
		}
		r := chi.NewRouter()
		r.Patch("/receivers/wallets/{receiver_wallet_id}", handler.RetryInvitation)

		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
		wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "wallet", "https://www.wallet.com", "www.wallet.com", "wallet://")
		rw := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.ReadyReceiversWalletStatus)

		eventProducerMock.
			On("WriteMessages", mock.Anything, []events.Message{
				{
					Topic:    events.ReceiverWalletNewInvitationTopic,
					Key:      rw.ID,
					TenantID: tnt.ID,
					Type:     events.RetryReceiverWalletInvitationType,
					Data: []schemas.EventReceiverWalletInvitationData{
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
		crashTrackerMock := &crashtracker.MockCrashTrackerClient{}
		defer crashTrackerMock.AssertExpectations(t)
		eventProducerMock := events.NewMockProducer(t)
		handler := ReceiverWalletsHandler{
			Models:             models,
			EventProducer:      eventProducerMock,
			CrashTrackerClient: crashTrackerMock,
		}
		r := chi.NewRouter()
		r.Patch("/receivers/wallets/{receiver_wallet_id}", handler.RetryInvitation)

		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
		wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "wallet", "https://www.wallet.com", "www.wallet.com", "wallet://")
		rw := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.ReadyReceiversWalletStatus)

		eventProducerMock.
			On("WriteMessages", mock.Anything, []events.Message{
				{
					Topic:    events.ReceiverWalletNewInvitationTopic,
					Key:      rw.ID,
					TenantID: tnt.ID,
					Type:     events.RetryReceiverWalletInvitationType,
					Data: []schemas.EventReceiverWalletInvitationData{
						{
							ReceiverWalletID: rw.ID,
						},
					},
				},
			}).
			Return(errors.New("unexpected error")).
			Once()

		crashTrackerMock.
			On("LogAndReportErrors", mock.Anything, mock.AnythingOfType("*fmt.wrapError"), "writing retry invitation message on the event producer").
			Return().
			Once()

		route := fmt.Sprintf("/receivers/wallets/%s", rw.ID)
		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, route, nil)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		wantJson := fmt.Sprintf(`{
			"id": %q,
			"receiver_id": %q,
			"wallet_id": %q,
			"created_at": %q,
			"invitation_sent_at": null
		}`, rw.ID, receiver.ID, wallet.ID, rw.CreatedAt.Format(time.RFC3339Nano))
		assert.JSONEq(t, wantJson, rr.Body.String())
	})

	t.Run("logs when couldn't write message because EventProducer is nil", func(t *testing.T) {
		handler := ReceiverWalletsHandler{Models: models}
		r := chi.NewRouter()
		r.Patch("/receivers/wallets/{receiver_wallet_id}", handler.RetryInvitation)

		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
		wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "wallet", "https://www.wallet.com", "www.wallet.com", "wallet://")
		rw := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.ReadyReceiversWalletStatus)

		router := chi.NewRouter()
		router.Patch("/receivers/wallets/{receiver_wallet_id}", handler.RetryInvitation)

		getEntries := log.DefaultLogger.StartTest(log.ErrorLevel)

		// Assert no receivers were registered
		route := fmt.Sprintf("/receivers/wallets/%s", rw.ID)
		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, route, nil)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

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

		msg := events.Message{
			Topic:    events.ReceiverWalletNewInvitationTopic,
			Key:      rw.ID,
			TenantID: tnt.ID,
			Type:     events.RetryReceiverWalletInvitationType,
			Data: []schemas.EventReceiverWalletInvitationData{
				{ReceiverWalletID: rw.ID},
			},
		}

		entries := getEntries()
		require.Len(t, entries, 1)
		assert.Equal(t, fmt.Sprintf("event producer is nil, could not publish messages %+v", []events.Message{msg}), entries[0].Message)
	})
}

func Test_ReceiverWalletsHandler_PatchReceiverWalletStatus(t *testing.T) {
	type TestCase struct {
		name           string
		setup          func(ctx context.Context, t *testing.T) (models *data.Models, receiverWalletID string)
		body           string
		expectedStatus int
		expectedJSON   string
	}

	ctx := context.Background()

	tests := []TestCase{
		{
			name: "400 – missing receiver_wallet_id URL param",
			setup: func(_ context.Context, _ *testing.T) (*data.Models, string) {
				return data.SetupModels(t), ""
			},
			body:           `{"status":"READY"}`,
			expectedStatus: http.StatusBadRequest,
			expectedJSON:   `{"error":"receiver_wallet_id is required"}`,
		},
		{
			name: "400 – invalid JSON in request body",
			setup: func(_ context.Context, _ *testing.T) (*data.Models, string) {
				return data.SetupModels(t), "irrelevant-id"
			},
			body:           `{"status": READY}`, // malformed JSON
			expectedStatus: http.StatusBadRequest,
			expectedJSON:   `{"error":"invalid request"}`,
		},
		{
			name: "400 – unknown status value",
			setup: func(_ context.Context, _ *testing.T) (*data.Models, string) {
				return data.SetupModels(t), "wallet-id"
			},
			body:           `{"status":"UNKNOWN_STATUS"}`,
			expectedStatus: http.StatusBadRequest,
			expectedJSON:   `{"error":"invalid status \"UNKNOWN_STATUS\"; valid values [DRAFT READY REGISTERED FLAGGED]"}`,
		},
		{
			name: "404 – receiver wallet not found",
			setup: func(ctx context.Context, _ *testing.T) (*data.Models, string) {
				return data.SetupModels(t), "non-existent-id"
			},
			body:           `{"status":"READY"}`,
			expectedStatus: http.StatusNotFound,
			expectedJSON:   `{"error":"receiver wallet not found"}`,
		},
		{
			name: "400 – receiver wallet not registered",
			setup: func(ctx context.Context, t *testing.T) (*data.Models, string) {
				models := data.SetupModels(t)
				dbPool := models.DBConnectionPool
				// create wallet & receiver wallet already in READY (≠ REGISTERED)
				wallet := data.CreateDefaultWalletFixture(t, ctx, dbPool)
				recv := data.CreateReceiverFixture(t, ctx, dbPool, &data.Receiver{})
				rw := data.CreateReceiverWalletFixture(t, ctx, dbPool, recv.ID, wallet.ID,
					data.ReadyReceiversWalletStatus)

				return models, rw.ID
			},
			body:           `{"status":"READY"}`,
			expectedStatus: http.StatusBadRequest,
			expectedJSON:   `{"error":"receiver wallet is not registered"}`,
		},
		{
			name: "400 – unsupported status transition to [FLAGGED]",
			setup: func(ctx context.Context, t *testing.T) (*data.Models, string) {
				models := data.SetupModels(t)
				dbPool := models.DBConnectionPool

				wallet := data.CreateDefaultWalletFixture(t, ctx, dbPool)
				recv := data.CreateReceiverFixture(t, ctx, dbPool, &data.Receiver{})
				rw := data.CreateReceiverWalletFixture(t, ctx, dbPool, recv.ID, wallet.ID,
					data.RegisteredReceiversWalletStatus)

				return models, rw.ID
			},
			body:           `{"status":"FLAGGED"}`,
			expectedStatus: http.StatusBadRequest,
			expectedJSON:   `{"error":"switching to status \"FLAGGED\" is not supported"}`,
		},
		{
			name: "400 – user-managed wallet cannot be unregistered",
			setup: func(ctx context.Context, t *testing.T) (*data.Models, string) {
				models := data.SetupModels(t)
				dbPool := models.DBConnectionPool

				wallet := data.CreateDefaultWalletFixture(t, ctx, dbPool)
				_, err := dbPool.ExecContext(ctx,
					`UPDATE wallets SET user_managed = TRUE WHERE id = $1`, wallet.ID)
				require.NoError(t, err)

				recv := data.CreateReceiverFixture(t, ctx, dbPool, &data.Receiver{})
				rw := data.CreateReceiverWalletFixture(t, ctx, dbPool, recv.ID, wallet.ID,
					data.RegisteredReceiversWalletStatus)

				t.Cleanup(func() {
					data.DeleteAllReceiverWalletsFixtures(t, ctx, dbPool)
				})

				return models, rw.ID
			},
			body:           `{"status":"READY"}`,
			expectedStatus: http.StatusBadRequest,
			expectedJSON:   `{"error":"user managed wallet cannot be unregistered"}`,
		},
		{
			name: "400 – wallet has payments in progress",
			setup: func(ctx context.Context, t *testing.T) (*data.Models, string) {
				models := data.SetupModels(t)
				dbPool := models.DBConnectionPool

				wallet := data.CreateDefaultWalletFixture(t, ctx, dbPool)
				recv := data.CreateReceiverFixture(t, ctx, dbPool, &data.Receiver{})
				rw := data.CreateReceiverWalletFixture(t, ctx, dbPool, recv.ID, wallet.ID,
					data.RegisteredReceiversWalletStatus)

				disb := data.CreateDisbursementFixture(t, ctx, dbPool,
					models.Disbursements, &data.Disbursement{})

				data.CreatePaymentFixture(t, ctx, dbPool, models.Payment, &data.Payment{
					Amount:         "42",
					Asset:          *disb.Asset,
					Status:         data.ReadyPaymentStatus,
					ReceiverWallet: rw,
					Disbursement:   disb,
				})

				t.Cleanup(func() {
					data.DeleteAllPaymentsFixtures(t, ctx, dbPool)
					data.DeleteAllReceiverWalletsFixtures(t, ctx, dbPool)
				})

				return models, rw.ID
			},
			body:           `{"status":"READY"}`,
			expectedStatus: http.StatusBadRequest,
			expectedJSON:   `{"error":"wallet has payments in progress"}`,
		},
		{
			name: "500 – unexpected DB error bubbles up as internal error",
			setup: func(ctx context.Context, t *testing.T) (*data.Models, string) {
				models := data.SetupModels(t)
				dbPool := models.DBConnectionPool

				wallet := data.CreateDefaultWalletFixture(t, ctx, dbPool)
				recv := data.CreateReceiverFixture(t, ctx, dbPool, &data.Receiver{})
				rw := data.CreateReceiverWalletFixture(t, ctx, dbPool, recv.ID, wallet.ID,
					data.RegisteredReceiversWalletStatus)

				// close pool so UpdateStatusToReady fails with a generic error
				dbPool.Close()

				return models, rw.ID
			},
			body:           `{"status":"READY"}`,
			expectedStatus: http.StatusInternalServerError,
			expectedJSON:   `{"error":"An internal error occurred while processing this request."}`,
		},
		{
			name: "200 – happy path, status updated to READY",
			setup: func(ctx context.Context, t *testing.T) (*data.Models, string) {
				models := data.SetupModels(t)
				dbPool := models.DBConnectionPool

				wallet := data.CreateDefaultWalletFixture(t, ctx, dbPool)
				recv := data.CreateReceiverFixture(t, ctx, dbPool, &data.Receiver{})
				rw := data.CreateReceiverWalletFixture(t, ctx, dbPool, recv.ID, wallet.ID,
					data.RegisteredReceiversWalletStatus)

				return models, rw.ID
			},
			body:           `{"status":"READY"}`,
			expectedStatus: http.StatusOK,
			expectedJSON:   `{"message":"receiver wallet status updated to \"READY\""}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			models, receiverWalletID := tc.setup(ctx, t)

			handler := ReceiverWalletsHandler{Models: models}
			router := chi.NewRouter()
			router.Patch("/receivers/wallets/{receiver_wallet_id}/status", handler.PatchReceiverWalletStatus)

			url := fmt.Sprintf("/receivers/wallets/%s/status", receiverWalletID)

			req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, strings.NewReader(tc.body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			resp := rr.Result()
			assert.Equal(t, tc.expectedStatus, resp.StatusCode)

			respBody, readErr := io.ReadAll(resp.Body)
			require.NoError(t, readErr)

			if tc.expectedJSON != "" {
				assert.JSONEq(t, tc.expectedJSON, string(respBody))
			}
		})
	}
}
