package httphandler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpresponse"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/middleware"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_PaymentsHandlerGet(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	handler := &PaymentsHandler{
		Models:           models,
		DBConnectionPool: dbConnectionPool,
	}

	// setup
	r := chi.NewRouter()
	r.Get("/payments/{id}", handler.GetPayment)

	ctx := context.Background()

	asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
	country := data.CreateCountryFixture(t, ctx, dbConnectionPool, "FRA", "France")
	wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")

	receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.DraftReceiversWalletStatus)

	disbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Name:    "disbursement 1",
		Status:  data.DraftDisbursementStatus,
		Asset:   asset,
		Wallet:  wallet,
		Country: country,
	})

	stellarTransactionID, err := utils.RandomString(64)
	require.NoError(t, err)
	stellarOperationID, err := utils.RandomString(32)
	require.NoError(t, err)

	payment := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		Amount:               "50",
		StellarTransactionID: stellarTransactionID,
		StellarOperationID:   stellarOperationID,
		Status:               data.DraftPaymentStatus,
		StatusHistory: []data.PaymentStatusHistoryEntry{
			{
				Status:        data.DraftPaymentStatus,
				StatusMessage: "",
				Timestamp:     time.Now(),
			},
		},
		Disbursement:   disbursement,
		Asset:          *asset,
		ReceiverWallet: receiverWallet,
	})

	t.Run("successfully returns payment details for given ID", func(t *testing.T) {
		// test
		route := fmt.Sprintf("/payments/%s", payment.ID)
		req, err := http.NewRequest("GET", route, nil)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		// assert response
		assert.Equal(t, http.StatusOK, rr.Code)

		wantJson := fmt.Sprintf(`{
			"id": %q,
			"amount": "50.0000000",
			"stellar_transaction_id": %q,
			"stellar_operation_id": %q,
			"status": "DRAFT",
			"status_history": [
				{
					"status": "DRAFT",
					"status_message": "",
					"timestamp": %q
				}
			],
			"disbursement": {
				"id": %q,
				"name": "disbursement 1",
				"status": "DRAFT",
				"created_at": %q,
				"updated_at": %q
			},
			"asset": {
				"id": %q,
				"code": "USDC",
				"issuer": "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV",
				"deleted_at": null
			},
			"receiver_wallet": {
				"id": %q,
				"receiver": {
					"id": %q
				},
				"wallet": {
					"id": %q,
					"name": "wallet1"
				},
				"stellar_address": %q,
				"stellar_memo": %q,
				"stellar_memo_type": %q,
				"status": "DRAFT",
				"created_at": %q,
				"updated_at": %q
			},
			"created_at": %q,
            "updated_at": %q
		}`, payment.ID, payment.StellarTransactionID, payment.StellarOperationID, payment.StatusHistory[0].Timestamp.Format(time.RFC3339Nano),
			disbursement.ID, disbursement.CreatedAt.Format(time.RFC3339Nano), disbursement.UpdatedAt.Format(time.RFC3339Nano),
			asset.ID, receiverWallet.ID, receiver.ID, wallet.ID, receiverWallet.StellarAddress, receiverWallet.StellarMemo,
			receiverWallet.StellarMemoType, receiverWallet.CreatedAt.Format(time.RFC3339Nano), receiverWallet.UpdatedAt.Format(time.RFC3339Nano),
			payment.CreatedAt.Format(time.RFC3339Nano), payment.UpdatedAt.Format(time.RFC3339Nano))

		assert.JSONEq(t, wantJson, rr.Body.String())
	})

	t.Run("error payment not found for given ID", func(t *testing.T) {
		// test
		req, err := http.NewRequest("GET", "/payments/invalid_id", nil)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		// assert response
		assert.Equal(t, http.StatusNotFound, rr.Code)

		wantJson := `{
			"error": "Cannot retrieve payment with ID: invalid_id"
		}`
		assert.JSONEq(t, wantJson, rr.Body.String())
	})
}

func Test_PaymentHandler_GetPayments_Errors(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	handler := &PaymentsHandler{
		Models:           models,
		DBConnectionPool: dbConnectionPool,
	}

	ts := httptest.NewServer(http.HandlerFunc(handler.GetPayments))
	defer ts.Close()

	tests := []struct {
		name               string
		queryParams        map[string]string
		expectedStatusCode int
		expectedResponse   string
	}{
		{
			name: "returns error when sort parameter is invalid",
			queryParams: map[string]string{
				"sort": "invalid_sort",
			},
			expectedStatusCode: http.StatusBadRequest,
			expectedResponse:   `{"error":"request invalid", "extras":{"sort":"invalid sort field name"}}`,
		},
		{
			name: "returns error when direction is invalid",
			queryParams: map[string]string{
				"direction": "invalid_direction",
			},
			expectedStatusCode: http.StatusBadRequest,
			expectedResponse:   `{"error":"request invalid", "extras":{"direction":"invalid sort order. valid values are 'asc' and 'desc'"}}`,
		},
		{
			name: "returns error when page is invalid",
			queryParams: map[string]string{
				"page": "invalid_page",
			},
			expectedStatusCode: http.StatusBadRequest,
			expectedResponse:   `{"error":"request invalid", "extras":{"page":"parameter must be an integer"}}`,
		},
		{
			name: "returns error when page_limit is invalid",
			queryParams: map[string]string{
				"page_limit": "invalid_page_limit",
			},
			expectedStatusCode: http.StatusBadRequest,
			expectedResponse:   `{"error":"request invalid", "extras":{"page_limit":"parameter must be an integer"}}`,
		},
		{
			name: "returns error when status is invalid",
			queryParams: map[string]string{
				"status": "invalid_status",
			},
			expectedStatusCode: http.StatusBadRequest,
			expectedResponse:   `{"error":"request invalid", "extras":{"status":"invalid parameter. valid values are: draft, ready, pending, paused, success, failed"}}`,
		},
		{
			name: "returns error when created_at_after is invalid",
			queryParams: map[string]string{
				"created_at_after": "invalid_created_at_after",
			},
			expectedStatusCode: http.StatusBadRequest,
			expectedResponse:   `{"error":"request invalid", "extras":{"created_at_after":"invalid date format. valid format is 'YYYY-MM-DD'"}}`,
		},
		{
			name: "returns error when created_at_before is invalid",
			queryParams: map[string]string{
				"created_at_before": "invalid_created_at_before",
			},
			expectedStatusCode: http.StatusBadRequest,
			expectedResponse:   `{"error":"request invalid", "extras":{"created_at_before":"invalid date format. valid format is 'YYYY-MM-DD'"}}`,
		},
		{
			name:               "returns empty list when no expectedPayments are found",
			queryParams:        map[string]string{},
			expectedStatusCode: http.StatusOK,
			expectedResponse:   `{"data":[], "pagination":{"pages":0, "total":0}}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Build the URL for the test request
			url := buildURLWithQueryParams(ts.URL, "/payments", tc.queryParams)
			resp, err := http.Get(url)
			require.NoError(t, err)
			defer resp.Body.Close()

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, tc.expectedStatusCode, resp.StatusCode)
			assert.JSONEq(t, tc.expectedResponse, string(respBody))
		})
	}
}

func Test_PaymentHandler_GetPayments_Success(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	handler := &PaymentsHandler{
		Models:           models,
		DBConnectionPool: dbConnectionPool,
	}

	ts := httptest.NewServer(http.HandlerFunc(handler.GetPayments))
	defer ts.Close()

	ctx := context.Background()

	// create fixtures
	asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
	country := data.CreateCountryFixture(t, ctx, dbConnectionPool, "FRA", "France")
	wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")

	// create receivers
	receiver1 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	receiverWallet1 := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver1.ID, wallet.ID, data.DraftReceiversWalletStatus)

	receiver2 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	receiverWallet2 := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver2.ID, wallet.ID, data.DraftReceiversWalletStatus)

	// create disbursements
	disbursement1 := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Name:    "disbursement 1",
		Status:  data.DraftDisbursementStatus,
		Asset:   asset,
		Wallet:  wallet,
		Country: country,
	})

	disbursement2 := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Name:    "disbursement 2",
		Status:  data.ReadyDisbursementStatus,
		Asset:   asset,
		Wallet:  wallet,
		Country: country,
	})

	stellarTransactionID, err := utils.RandomString(64)
	require.NoError(t, err)
	stellarOperationID, err := utils.RandomString(32)
	require.NoError(t, err)

	// create payments
	payment1 := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		Amount:               "50",
		StellarTransactionID: stellarTransactionID,
		StellarOperationID:   stellarOperationID,
		Status:               data.PendingPaymentStatus,
		Disbursement:         disbursement1,
		Asset:                *asset,
		ReceiverWallet:       receiverWallet1,
		CreatedAt:            time.Date(2022, 12, 10, 23, 40, 20, 1431, time.UTC),
		UpdatedAt:            time.Date(2023, 3, 10, 23, 40, 20, 1431, time.UTC),
	})

	stellarTransactionID, err = utils.RandomString(64)
	require.NoError(t, err)
	stellarOperationID, err = utils.RandomString(32)
	require.NoError(t, err)

	payment2 := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		Amount:               "150",
		StellarTransactionID: stellarTransactionID,
		StellarOperationID:   stellarOperationID,
		Status:               data.DraftPaymentStatus,
		Disbursement:         disbursement1,
		Asset:                *asset,
		ReceiverWallet:       receiverWallet2,
		CreatedAt:            time.Date(2023, 1, 10, 23, 40, 20, 1431, time.UTC),
		UpdatedAt:            time.Date(2023, 1, 10, 23, 40, 20, 1431, time.UTC),
	})

	stellarTransactionID, err = utils.RandomString(64)
	require.NoError(t, err)
	stellarOperationID, err = utils.RandomString(32)
	require.NoError(t, err)

	payment3 := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		Amount:               "200.50",
		StellarTransactionID: stellarTransactionID,
		StellarOperationID:   stellarOperationID,
		Status:               data.DraftPaymentStatus,
		Disbursement:         disbursement2,
		Asset:                *asset,
		ReceiverWallet:       receiverWallet1,
		CreatedAt:            time.Date(2023, 2, 10, 23, 40, 20, 1431, time.UTC),
		UpdatedAt:            time.Date(2023, 2, 10, 23, 40, 20, 1431, time.UTC),
	})

	stellarTransactionID, err = utils.RandomString(64)
	require.NoError(t, err)
	stellarOperationID, err = utils.RandomString(32)
	require.NoError(t, err)

	payment4 := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		Amount:               "20",
		StellarTransactionID: stellarTransactionID,
		StellarOperationID:   stellarOperationID,
		Status:               data.PendingPaymentStatus,
		Disbursement:         disbursement2,
		Asset:                *asset,
		ReceiverWallet:       receiverWallet2,
		CreatedAt:            time.Date(2023, 3, 10, 23, 40, 20, 1431, time.UTC),
		UpdatedAt:            time.Date(2023, 4, 10, 23, 40, 20, 1431, time.UTC),
	})

	tests := []struct {
		name               string
		queryParams        map[string]string
		expectedStatusCode int
		expectedPagination httpresponse.PaginationInfo
		expectedPayments   []data.Payment
	}{
		{
			name:               "fetch all payments without filters",
			queryParams:        map[string]string{},
			expectedStatusCode: http.StatusOK,
			expectedPagination: httpresponse.PaginationInfo{
				Next:  "",
				Prev:  "",
				Pages: 1,
				Total: 4,
			},
			expectedPayments: []data.Payment{*payment4, *payment1, *payment3, *payment2},
		},
		{
			name: "fetch first page of payments with limit 1 and sort by created_at",
			queryParams: map[string]string{
				"page":       "1",
				"page_limit": "1",
				"sort":       "created_at",
				"direction":  "asc",
			},
			expectedStatusCode: http.StatusOK,
			expectedPagination: httpresponse.PaginationInfo{
				Next:  "/payments?direction=asc&page=2&page_limit=1&sort=created_at",
				Prev:  "",
				Pages: 4,
				Total: 4,
			},
			expectedPayments: []data.Payment{*payment1},
		},
		{
			name: "fetch second page of payments with limit 1 and sort by created_at",
			queryParams: map[string]string{
				"page":       "2",
				"page_limit": "1",
				"sort":       "created_at",
				"direction":  "asc",
			},
			expectedStatusCode: http.StatusOK,
			expectedPagination: httpresponse.PaginationInfo{
				Next:  "/payments?direction=asc&page=3&page_limit=1&sort=created_at",
				Prev:  "/payments?direction=asc&page=1&page_limit=1&sort=created_at",
				Pages: 4,
				Total: 4,
			},
			expectedPayments: []data.Payment{*payment2},
		},
		{
			name: "fetch last page of payments with limit 1 and sort by created_at",
			queryParams: map[string]string{
				"page":       "4",
				"page_limit": "1",
				"sort":       "created_at",
				"direction":  "asc",
			},
			expectedStatusCode: http.StatusOK,
			expectedPagination: httpresponse.PaginationInfo{
				Next:  "",
				Prev:  "/payments?direction=asc&page=3&page_limit=1&sort=created_at",
				Pages: 4,
				Total: 4,
			},
			expectedPayments: []data.Payment{*payment4},
		},
		{
			name: "fetch payments with status draft",
			queryParams: map[string]string{
				"status": "dRaFt",
			},
			expectedStatusCode: http.StatusOK,
			expectedPagination: httpresponse.PaginationInfo{
				Next:  "",
				Prev:  "",
				Pages: 1,
				Total: 2,
			},
			expectedPayments: []data.Payment{*payment3, *payment2},
		},
		{
			name: "fetch payments for receiver1",
			queryParams: map[string]string{
				"receiver_id": receiver1.ID,
			},
			expectedStatusCode: http.StatusOK,
			expectedPagination: httpresponse.PaginationInfo{
				Next:  "",
				Prev:  "",
				Pages: 1,
				Total: 2,
			},
			expectedPayments: []data.Payment{*payment1, *payment3},
		},
		{
			name: "fetch payments for receiver2",
			queryParams: map[string]string{
				"receiver_id": receiver2.ID,
			},
			expectedStatusCode: http.StatusOK,
			expectedPagination: httpresponse.PaginationInfo{
				Next:  "",
				Prev:  "",
				Pages: 1,
				Total: 2,
			},
			expectedPayments: []data.Payment{*payment4, *payment2},
		},
		{
			name: "returns empty list when receiver_id is not found",
			queryParams: map[string]string{
				"receiver_id": "invalid_id",
			},
			expectedStatusCode: http.StatusOK,
			expectedPagination: httpresponse.PaginationInfo{
				Next:  "",
				Prev:  "",
				Pages: 0,
				Total: 0,
			},
			expectedPayments: []data.Payment{},
		},
		{
			name: "fetch payments created at before 2023-01-01",
			queryParams: map[string]string{
				"created_at_before": "2023-01-01",
			},
			expectedStatusCode: http.StatusOK,
			expectedPagination: httpresponse.PaginationInfo{
				Next:  "",
				Prev:  "",
				Pages: 1,
				Total: 1,
			},
			expectedPayments: []data.Payment{*payment1},
		},
		{
			name: "fetch payments after 2023-03-01",
			queryParams: map[string]string{
				"created_at_after": "2023-03-01",
			},
			expectedStatusCode: http.StatusOK,
			expectedPagination: httpresponse.PaginationInfo{
				Next:  "",
				Prev:  "",
				Pages: 1,
				Total: 1,
			},
			expectedPayments: []data.Payment{*payment4},
		},
		{
			name: "fetch payment created at after 2023-01-01 and before 2023-03-01",
			queryParams: map[string]string{
				"created_at_after":  "2023-01-01",
				"created_at_before": "2023-03-01",
			},
			expectedStatusCode: http.StatusOK,
			expectedPagination: httpresponse.PaginationInfo{
				Next:  "",
				Prev:  "",
				Pages: 1,
				Total: 2,
			},
			expectedPayments: []data.Payment{*payment3, *payment2},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Build the URL for the test request
			url := buildURLWithQueryParams(ts.URL, "/payments", tc.queryParams)
			resp, err := http.Get(url)
			require.NoError(t, err)
			defer resp.Body.Close()

			// Parse the response body
			var actualResponse httpresponse.PaginatedResponse
			err = json.NewDecoder(resp.Body).Decode(&actualResponse)
			require.NoError(t, err)

			// Assert on the pagination data
			assert.Equal(t, tc.expectedPagination, actualResponse.Pagination)

			// Parse the response data
			var actualPayments []data.Payment
			err = json.Unmarshal(actualResponse.Data, &actualPayments)
			require.NoError(t, err)

			// Assert on the payments data
			assert.Equal(t, tc.expectedPayments, actualPayments)
		})
	}
}

func Test_PaymentHandler_RetryPayments(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	authManagerMock := &auth.AuthManagerMock{}

	handler := PaymentsHandler{
		Models:           models,
		DBConnectionPool: dbConnectionPool,
		AuthManager:      authManagerMock,
	}

	ctx := context.Background()

	data.DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
	data.DeleteAllDisbursementFixtures(t, ctx, dbConnectionPool)
	data.DeleteAllCountryFixtures(t, ctx, dbConnectionPool)
	data.DeleteAllAssetFixtures(t, ctx, dbConnectionPool)
	data.DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
	data.DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)
	data.DeleteAllWalletFixtures(t, ctx, dbConnectionPool)

	country := data.CreateCountryFixture(t, ctx, dbConnectionPool, "BRA", "Brazil")
	wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "Wallet", "https://www.wallet.com", "www.wallet.com", "wallet://")
	asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")

	receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.ReadyReceiversWalletStatus)

	disbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Country:           country,
		Wallet:            wallet,
		Asset:             asset,
		Status:            data.ReadyDisbursementStatus,
		VerificationField: data.VerificationFieldDateOfBirth,
	})

	t.Run("returns Unauthorized when no token in the context", func(t *testing.T) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, "/retry", strings.NewReader("{}"))
		require.NoError(t, err)

		rw := httptest.NewRecorder()
		http.HandlerFunc(handler.RetryPayments).ServeHTTP(rw, req)

		resp := rw.Result()
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		assert.JSONEq(t, `{"error": "Not authorized."}`, string(respBody))
	})

	t.Run("returns InternalServerError when fails getting user from token", func(t *testing.T) {
		ctx = context.WithValue(ctx, middleware.TokenContextKey, "mytoken")

		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, "/retry", strings.NewReader("{}"))
		require.NoError(t, err)

		authManagerMock.
			On("GetUser", ctx, "mytoken").
			Return(nil, errors.New("unexpected error")).
			Once()

		rw := httptest.NewRecorder()
		http.HandlerFunc(handler.RetryPayments).ServeHTTP(rw, req)

		resp := rw.Result()
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		assert.JSONEq(t, `{"error": "An internal error occurred while processing this request."}`, string(respBody))
	})

	t.Run("returns BadRequest when fails decoding body request", func(t *testing.T) {
		ctx = context.WithValue(ctx, middleware.TokenContextKey, "mytoken")

		payload := strings.NewReader("invalid")
		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, "/retry", payload)
		require.NoError(t, err)

		authManagerMock.
			On("GetUser", ctx, "mytoken").
			Return(&auth.User{
				Email: "email@test.com",
			}, nil).
			Once()

		rw := httptest.NewRecorder()
		http.HandlerFunc(handler.RetryPayments).ServeHTTP(rw, req)

		resp := rw.Result()
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, `{"error": "The request was invalid in some way."}`, string(respBody))
	})

	t.Run("returns BadRequest when fails when payload is invalid", func(t *testing.T) {
		ctx = context.WithValue(ctx, middleware.TokenContextKey, "mytoken")

		payload := strings.NewReader("{}")
		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, "/retry", payload)
		require.NoError(t, err)

		authManagerMock.
			On("GetUser", ctx, "mytoken").
			Return(&auth.User{
				Email: "email@test.com",
			}, nil).
			Once()

		rw := httptest.NewRecorder()
		http.HandlerFunc(handler.RetryPayments).ServeHTTP(rw, req)

		resp := rw.Result()
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, `{"error": "The request was invalid in some way.", "extras": {"payment_ids": "payment_ids should not be empty"}}`, string(respBody))
	})

	t.Run("returns BadRequest when some payments IDs are not in the failed state", func(t *testing.T) {
		data.DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)

		payment1 := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-1",
			StellarOperationID:   "operation-id-1",
			Status:               data.PendingPaymentStatus,
			Disbursement:         disbursement,
			ReceiverWallet:       receiverWallet,
			Asset:                *asset,
		})

		payment2 := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-2",
			StellarOperationID:   "operation-id-2",
			Status:               data.ReadyPaymentStatus,
			Disbursement:         disbursement,
			ReceiverWallet:       receiverWallet,
			Asset:                *asset,
		})

		payment3 := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-3",
			StellarOperationID:   "operation-id-3",
			Status:               data.FailedPaymentStatus,
			Disbursement:         disbursement,
			ReceiverWallet:       receiverWallet,
			Asset:                *asset,
		})

		payment4 := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-4",
			StellarOperationID:   "operation-id-4",
			Status:               data.FailedPaymentStatus,
			Disbursement:         disbursement,
			ReceiverWallet:       receiverWallet,
			Asset:                *asset,
		})

		ctx = context.WithValue(ctx, middleware.TokenContextKey, "mytoken")

		payload := strings.NewReader(fmt.Sprintf(`
			{
				"payment_ids": [
					%q,
					%q,
					%q,
					%q
				]
			}
		`, payment1.ID, payment2.ID, payment3.ID, payment4.ID))
		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, "/retry", payload)
		require.NoError(t, err)

		authManagerMock.
			On("GetUser", ctx, "mytoken").
			Return(&auth.User{
				Email: "email@test.com",
			}, nil).
			Once()

		rw := httptest.NewRecorder()
		http.HandlerFunc(handler.RetryPayments).ServeHTTP(rw, req)

		resp := rw.Result()
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, `{"error": "Invalid payment ID(s) provided. All payment IDs must exist and be in the 'FAILED' state."}`, string(respBody))

		payment1DB, err := models.Payment.Get(ctx, payment1.ID, dbConnectionPool)
		require.NoError(t, err)

		payment2DB, err := models.Payment.Get(ctx, payment2.ID, dbConnectionPool)
		require.NoError(t, err)

		payment3DB, err := models.Payment.Get(ctx, payment3.ID, dbConnectionPool)
		require.NoError(t, err)

		payment4DB, err := models.Payment.Get(ctx, payment4.ID, dbConnectionPool)
		require.NoError(t, err)

		// Payment 1
		assert.Equal(t, data.PendingPaymentStatus, payment1DB.Status)
		assert.Equal(t, payment1.StellarTransactionID, payment1DB.StellarTransactionID)
		assert.Equal(t, payment1.StatusHistory, payment1DB.StatusHistory)

		// Payment 2
		assert.Equal(t, data.ReadyPaymentStatus, payment2DB.Status)
		assert.Equal(t, payment2.StellarTransactionID, payment2DB.StellarTransactionID)
		assert.Equal(t, payment2.StatusHistory, payment2DB.StatusHistory)

		// Payment 3
		assert.Equal(t, data.FailedPaymentStatus, payment3DB.Status)
		assert.Equal(t, payment3.StellarTransactionID, payment3DB.StellarTransactionID)
		assert.Equal(t, payment3.StatusHistory, payment3DB.StatusHistory)

		// Payment 4
		assert.Equal(t, data.FailedPaymentStatus, payment4DB.Status)
		assert.Equal(t, payment4.StellarTransactionID, payment4DB.StellarTransactionID)
		assert.Equal(t, payment4.StatusHistory, payment4DB.StatusHistory)
	})

	t.Run("successfully retries failed payments", func(t *testing.T) {
		data.DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)

		payment1 := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-1",
			StellarOperationID:   "operation-id-1",
			Status:               data.FailedPaymentStatus,
			Disbursement:         disbursement,
			ReceiverWallet:       receiverWallet,
			Asset:                *asset,
		})

		payment2 := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-2",
			StellarOperationID:   "operation-id-2",
			Status:               data.FailedPaymentStatus,
			Disbursement:         disbursement,
			ReceiverWallet:       receiverWallet,
			Asset:                *asset,
		})

		ctx = context.WithValue(ctx, middleware.TokenContextKey, "mytoken")

		payload := strings.NewReader(fmt.Sprintf(`
			{
				"payment_ids": [
					%q,
					%q
				]
			}
		`, payment1.ID, payment2.ID))
		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, "/retry", payload)
		require.NoError(t, err)

		authManagerMock.
			On("GetUser", ctx, "mytoken").
			Return(&auth.User{
				Email: "email@test.com",
			}, nil).
			Once()

		rw := httptest.NewRecorder()
		http.HandlerFunc(handler.RetryPayments).ServeHTTP(rw, req)

		resp := rw.Result()
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, `{"message": "Payments retried successfully"}`, string(respBody))

		payment1DB, err := models.Payment.Get(ctx, payment1.ID, dbConnectionPool)
		require.NoError(t, err)

		payment2DB, err := models.Payment.Get(ctx, payment2.ID, dbConnectionPool)
		require.NoError(t, err)

		// Payment 1
		assert.Equal(t, data.ReadyPaymentStatus, payment1DB.Status)
		assert.Empty(t, payment1DB.StellarTransactionID)
		assert.NotEqual(t, payment1.StatusHistory, payment1DB.StatusHistory)
		assert.Len(t, payment1DB.StatusHistory, 2)
		assert.Equal(t, data.ReadyPaymentStatus, payment1DB.StatusHistory[1].Status)
		assert.Equal(t, "User email@test.com has requested to retry the payment - Previous Stellar Transaction ID: stellar-transaction-id-1", payment1DB.StatusHistory[1].StatusMessage)

		// Payment 2
		assert.Equal(t, data.ReadyPaymentStatus, payment2DB.Status)
		assert.Empty(t, payment2DB.StellarTransactionID)
		assert.NotEqual(t, payment2.StatusHistory, payment2DB.StatusHistory)
		assert.Len(t, payment2DB.StatusHistory, 2)
		assert.Equal(t, data.ReadyPaymentStatus, payment2DB.StatusHistory[1].Status)
		assert.Equal(t, "User email@test.com has requested to retry the payment - Previous Stellar Transaction ID: stellar-transaction-id-2", payment2DB.StatusHistory[1].StatusMessage)
	})
}

func Test_PaymentsHandler_getPaymentsWithCount(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)
	handler := &PaymentsHandler{
		Models:           models,
		DBConnectionPool: dbConnectionPool,
	}

	t.Run("0 payments created", func(t *testing.T) {
		response, err := handler.getPaymentsWithCount(ctx, &data.QueryParams{})
		require.NoError(t, err)

		assert.Equal(t, response.Total, 0)
		assert.Equal(t, response.Result, []data.Payment(nil))
	})

	t.Run("error invalid payment status", func(t *testing.T) {
		_, err := handler.getPaymentsWithCount(ctx, &data.QueryParams{
			Filters: map[data.FilterKey]interface{}{
				data.FilterKeyStatus: "INVALID",
			},
		})
		require.EqualError(t, err, `running atomic function in RunInTransactionWithResult: error counting payments: error counting payments: pq: invalid input value for enum payment_status: "INVALID"`)
	})

	asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
	country := data.CreateCountryFixture(t, ctx, dbConnectionPool, "FRA", "France")
	wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")

	receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.DraftReceiversWalletStatus)

	disbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Name:    "disbursement 1",
		Status:  data.DraftDisbursementStatus,
		Asset:   asset,
		Wallet:  wallet,
		Country: country,
	})

	payment := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		Amount: "50",
		Status: data.DraftPaymentStatus,
		StatusHistory: []data.PaymentStatusHistoryEntry{
			{
				Status:        data.DraftPaymentStatus,
				StatusMessage: "",
				Timestamp:     time.Now(),
			},
		},
		Disbursement:   disbursement,
		Asset:          *asset,
		ReceiverWallet: receiverWallet,
	})

	t.Run("return payment", func(t *testing.T) {
		response, err := handler.getPaymentsWithCount(ctx, &data.QueryParams{})
		require.NoError(t, err)

		assert.Equal(t, response.Total, 1)
		assert.Equal(t, response.Result, []data.Payment{*payment})
	})

	t.Run("return multiple payments", func(t *testing.T) {
		payment2 := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			Amount: "50",
			Status: data.DraftPaymentStatus,
			StatusHistory: []data.PaymentStatusHistoryEntry{
				{
					Status:        data.DraftPaymentStatus,
					StatusMessage: "",
					Timestamp:     time.Now(),
				},
			},
			Disbursement:   disbursement,
			Asset:          *asset,
			ReceiverWallet: receiverWallet,
		})

		response, err := handler.getPaymentsWithCount(ctx, &data.QueryParams{})
		require.NoError(t, err)

		assert.Equal(t, response.Total, 2)
		assert.Equal(t, response.Result, []data.Payment{*payment2, *payment})
	})
}
