package httphandler

import (
	"context"
	crand "crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/strkey"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
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
	ctx := sdpcontext.SetTenantInContext(context.Background(), &tnt)

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

	t.Run("successfuly retry invitation", func(t *testing.T) {
		handler := ReceiverWalletsHandler{
			Models: models,
		}
		r := chi.NewRouter()
		r.Patch("/receivers/wallets/{receiver_wallet_id}", handler.RetryInvitation)

		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
		wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "wallet", "https://www.wallet.com", "www.wallet.com", "wallet://")
		rw := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.ReadyReceiversWalletStatus)

		route := fmt.Sprintf("/receivers/wallets/%s", rw.ID)
		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, route, nil)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		wantJSON := fmt.Sprintf(`{
			"id": %q,
			"receiver_id": %q,
			"wallet_id": %q,
			"created_at": %q,
			"invitation_sent_at": null
		}`, rw.ID, receiver.ID, wallet.ID, rw.CreatedAt.Format(time.RFC3339Nano))

		resp := rr.Result()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, wantJSON, rr.Body.String())
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

func Test_ReceiverWalletsHandler_PatchReceiverWallet_DuplicateStellarAddress(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)
	tnt := schema.Tenant{ID: "tenant-id"}
	ctx := sdpcontext.SetTenantInContext(context.Background(), &tnt)

	receiver1 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	receiver2 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})

	// user managed wallet for receiver1
	userManagedWallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "User Managed Wallet", "stellar.org", "stellar.org", "stellar://")
	data.MakeWalletUserManaged(t, ctx, dbConnectionPool, userManagedWallet.ID)

	// wallet for receiver2
	wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "Wallet", "https://www.wallet.com", "www.wallet.com", "wallet://")

	// Create receiver wallets
	rw1 := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver1.ID, userManagedWallet.ID, data.DraftReceiversWalletStatus)
	rw2 := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver2.ID, wallet.ID, data.RegisteredReceiversWalletStatus)

	handler := ReceiverWalletsHandler{Models: models}
	router := chi.NewRouter()
	router.Patch("/receivers/{receiver_id}/wallets/{receiver_wallet_id}", handler.PatchReceiverWallet)

	t.Run("patch receiver1 with new stellar address succeeds", func(t *testing.T) {
		newStellarAddress := "GDQP2KPQGKIHYJGXNUIYOMHARUARCA7DJT5FO2FFOOKY3B2WSQHG4W37"
		reqBody := fmt.Sprintf(`{"stellar_address": "%s"}`, newStellarAddress)
		route := fmt.Sprintf("/receivers/%s/wallets/%s", receiver1.ID, rw1.ID)

		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, route, strings.NewReader(reqBody))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		resp := rr.Result()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var responseData map[string]interface{}
		err = json.Unmarshal(respBody, &responseData)
		require.NoError(t, err)

		assert.Equal(t, newStellarAddress, responseData["stellar_address"])
	})

	t.Run("patch receiver1 with receiver2's stellar address triggers conflict", func(t *testing.T) {
		reqBody := fmt.Sprintf(`{"stellar_address": "%s"}`, rw2.StellarAddress)
		route := fmt.Sprintf("/receivers/%s/wallets/%s", receiver1.ID, rw1.ID)

		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, route, strings.NewReader(reqBody))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		resp := rr.Result()
		assert.Equal(t, http.StatusConflict, resp.StatusCode)

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		expectedJSON := `{
			"error": "The provided wallet address is already associated with another user.",
			"extras": {
				"wallet_address": "wallet address must be unique"
			}
		}`

		assert.JSONEq(t, expectedJSON, string(respBody))
	})

	t.Run("receiver_wallet_id doesn't belong to receiver_id returns error", func(t *testing.T) {
		reqBody := fmt.Sprintf(`{"stellar_address": "%s"}`, rw2.StellarAddress)
		route := fmt.Sprintf("/receivers/%s/wallets/%s", receiver1.ID, rw2.ID)

		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, route, strings.NewReader(reqBody))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		resp := rr.Result()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Contains(t, string(respBody), "Receiver wallet does not belong to the specified receiver")
	})
}

func Test_ReceiverwalletsHandler_PatchReceiverWallet_MemoValidation(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)
	tnt := schema.Tenant{ID: "tenant-id"}
	ctx := sdpcontext.SetTenantInContext(context.Background(), &tnt)

	handler := ReceiverWalletsHandler{Models: models}
	router := chi.NewRouter()
	router.Patch("/receivers/{receiver_id}/wallets/{receiver_wallet_id}", handler.PatchReceiverWallet)

	createUserManagedReceiverWallet := func(t *testing.T, status data.ReceiversWalletStatus) (*data.Receiver, *data.ReceiverWallet) {
		t.Helper()

		wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "User Managed Wallet", "stellar.org", "stellar.org", "stellar://")
		data.MakeWalletUserManaged(t, ctx, dbConnectionPool, wallet.ID)
		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
		rw := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, status)

		return receiver, rw
	}

	doPatch := func(body string, receiverID string, receiverWalletID string) (*http.Response, []byte) {
		req, requestErr := http.NewRequestWithContext(ctx, http.MethodPatch,
			fmt.Sprintf("/receivers/%s/wallets/%s", receiverID, receiverWalletID), strings.NewReader(body))
		require.NoError(t, requestErr)
		req.Header.Set("Content-Type", "application/json")

		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		resp := rr.Result()
		payload, readErr := io.ReadAll(resp.Body)
		require.NoError(t, readErr)

		return resp, payload
	}

	generateAccountAddress := func(t *testing.T) string {
		t.Helper()
		return keypair.MustRandom().Address()
	}

	generateContractAddress := func(t *testing.T) string {
		t.Helper()

		payload := make([]byte, 32)
		_, randErr := crand.Read(payload)
		require.NoError(t, randErr)

		addr, encodeErr := strkey.Encode(strkey.VersionByteContract, payload)
		require.NoError(t, encodeErr)

		return addr
	}

	t.Run("accepts contract address without memo", func(t *testing.T) {
		receiver, rw := createUserManagedReceiverWallet(t, data.DraftReceiversWalletStatus)
		contractAddress := generateContractAddress(t)

		resp, payload := doPatch(fmt.Sprintf(`{"stellar_address": "%s"}`, contractAddress), receiver.ID, rw.ID)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var responseData map[string]interface{}
		unmarshalErr := json.Unmarshal(payload, &responseData)
		require.NoError(t, unmarshalErr)
		assert.Equal(t, contractAddress, responseData["stellar_address"])
	})

	t.Run("allows switching from contract address to account address with memo", func(t *testing.T) {
		receiver, rw := createUserManagedReceiverWallet(t, data.DraftReceiversWalletStatus)
		currentContract := generateContractAddress(t)

		resp, payload := doPatch(fmt.Sprintf(`{"stellar_address": "%s"}`, currentContract), receiver.ID, rw.ID)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(payload))

		newAccountAddress := generateAccountAddress(t)
		memo := "987654321"

		resp, payload = doPatch(fmt.Sprintf(`{"stellar_address": "%s","stellar_memo":"%s"}`, newAccountAddress, memo), receiver.ID, rw.ID)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var responseData map[string]interface{}
		unmarshalErr := json.Unmarshal(payload, &responseData)
		require.NoError(t, unmarshalErr)
		assert.Equal(t, newAccountAddress, responseData["stellar_address"])
		assert.Equal(t, memo, responseData["stellar_memo"])
		assert.Equal(t, string(schema.MemoTypeID), responseData["stellar_memo_type"])
	})

	t.Run("requires clearing memo before switching to contract address", func(t *testing.T) {
		receiver, rw := createUserManagedReceiverWallet(t, data.RegisteredReceiversWalletStatus)
		require.NotEmpty(t, rw.StellarMemo)

		contractAddress := generateContractAddress(t)

		resp, payload := doPatch(fmt.Sprintf(`{"stellar_address": "%s"}`, contractAddress), receiver.ID, rw.ID)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, `{"error":"Clear memo before assigning a contract address"}`, string(payload))
	})

	t.Run("rejects memo payload when assigning contract address", func(t *testing.T) {
		receiver, rw := createUserManagedReceiverWallet(t, data.DraftReceiversWalletStatus)

		contractAddress := generateContractAddress(t)

		resp, payload := doPatch(fmt.Sprintf(`{"stellar_address": "%s","stellar_memo":"memo-value"}`, contractAddress), receiver.ID, rw.ID)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, `{"error":"Memos are not supported for contract addresses"}`, string(payload))
	})

	t.Run("allows clearing memo when switching to contract address", func(t *testing.T) {
		receiver, rw := createUserManagedReceiverWallet(t, data.RegisteredReceiversWalletStatus)
		require.NotEmpty(t, rw.StellarMemo)

		contractAddress := generateContractAddress(t)

		resp, payload := doPatch(fmt.Sprintf(`{"stellar_address": "%s","stellar_memo":""}`, contractAddress), receiver.ID, rw.ID)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var responseData map[string]interface{}
		unmarshalErr := json.Unmarshal(payload, &responseData)
		require.NoError(t, unmarshalErr)
		assert.Equal(t, contractAddress, responseData["stellar_address"])
		_, memoPresent := responseData["stellar_memo"]
		assert.False(t, memoPresent)
		_, memoTypePresent := responseData["stellar_memo_type"]
		assert.False(t, memoTypePresent)
	})
}
