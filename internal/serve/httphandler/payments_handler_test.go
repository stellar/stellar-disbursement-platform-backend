package httphandler

import (
	"bytes"
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
	"github.com/google/uuid"
	"github.com/stellar/go/support/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events/schemas"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpresponse"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/middleware"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/testutils"
	sigMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

func Test_PaymentsHandlerGet(t *testing.T) {
	dbConnectionPool := testutils.GetDBConnectionPool(t)

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	mDistributionAccountResolver := sigMocks.NewMockDistributionAccountResolver(t)

	handler := &PaymentsHandler{
		Models:                      models,
		DBConnectionPool:            dbConnectionPool,
		DistributionAccountResolver: mDistributionAccountResolver,
	}

	mDistributionAccountResolver.
		On("DistributionAccountFromContext", mock.Anything).
		Return(schema.TransactionAccount{Type: schema.DistributionAccountStellarEnv}, nil).
		Maybe()

	r := chi.NewRouter()
	r.Get("/payments/{id}", handler.GetPayment)

	ctx := context.Background()

	asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
	wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")

	receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.DraftReceiversWalletStatus)

	disbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Name:   "disbursement 1",
		Status: data.DraftDisbursementStatus,
		Asset:  asset,
		Wallet: wallet,
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
		Disbursement:      disbursement,
		Asset:             *asset,
		ReceiverWallet:    receiverWallet,
		ExternalPaymentID: "mockID",
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

		wantJson := `{
			"id": "` + payment.ID + `",
			"amount": "50.0000000",
			"stellar_transaction_id": "` + payment.StellarTransactionID + `",
			"stellar_operation_id": "` + payment.StellarOperationID + `",
			"status": "DRAFT",
			"payment_type": "DISBURSEMENT",
			"status_history": [
				{
					"status": "DRAFT",
					"status_message": "",
					"timestamp": "` + payment.StatusHistory[0].Timestamp.Format(time.RFC3339Nano) + `"
				}
			],
			"disbursement": {
				"id": "` + disbursement.ID + `",
				"name": "disbursement 1",
				"status": "DRAFT",
				"status_history": [
					{
						"status": "DRAFT",
						"user_id": "",
						"timestamp": "` + disbursement.StatusHistory[0].Timestamp.Format(time.RFC3339Nano) + `"
					}
				],
				"created_at": "` + disbursement.CreatedAt.Format(time.RFC3339Nano) + `",
				"updated_at": "` + disbursement.UpdatedAt.Format(time.RFC3339Nano) + `",
				"registration_contact_type": "` + disbursement.RegistrationContactType.String() + `",
				"verification_field": "` + string(disbursement.VerificationField) + `",
				"receiver_registration_message_template":""
			},
			"asset": {
				"id": "` + asset.ID + `",
				"code": "USDC",
				"issuer": "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV",
				"deleted_at": null
			},
			"receiver_wallet": {
				"id": "` + receiverWallet.ID + `",
				"receiver": {
					"id": "` + receiver.ID + `",
					"created_at": "` + receiver.CreatedAt.Format(time.RFC3339Nano) + `",
					"email": "` + receiver.Email + `", 
					"external_id": "` + receiver.ExternalID + `",
					"phone_number": "` + receiver.PhoneNumber + `",
					"updated_at": "` + receiver.UpdatedAt.Format(time.RFC3339Nano) + `"
				},
				"wallet": {
					"id": "` + wallet.ID + `",
					"name": "wallet1",
					"deep_link_schema": "` + wallet.DeepLinkSchema + `",
					"homepage": "` + wallet.Homepage + `",
					"sep_10_client_domain": "` + wallet.SEP10ClientDomain + `",
					"enabled": true
				},
				"status": "DRAFT",
				"status_history": [
					{
						"status": "DRAFT",
						"timestamp": "` + receiverWallet.StatusHistory[0].Timestamp.Format(time.RFC3339Nano) + `"
					}
				],
				"created_at": "` + receiverWallet.CreatedAt.Format(time.RFC3339Nano) + `",
				"updated_at": "` + receiverWallet.UpdatedAt.Format(time.RFC3339Nano) + `",
				"invitation_sent_at": null
			},
			"created_at": "` + payment.CreatedAt.Format(time.RFC3339Nano) + `",
			"updated_at": "` + payment.UpdatedAt.Format(time.RFC3339Nano) + `",
			"external_payment_id": "` + payment.ExternalPaymentID + `"
		}`

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

func Test_PaymentHandler_GetPayments_CirclePayments(t *testing.T) {
	ctx := context.Background()

	dbConnectionPool := testutils.GetDBConnectionPool(t)

	models, outerErr := data.NewModels(dbConnectionPool)
	require.NoError(t, outerErr)

	// Create fixtures
	asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
	wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")
	disbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Wallet: wallet,
		Status: data.ReadyDisbursementStatus,
		Asset:  asset,
	})
	receiverReady := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	rwReady := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiverReady.ID, wallet.ID, data.ReadyReceiversWalletStatus)
	payment1 := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		ReceiverWallet: rwReady,
		Disbursement:   disbursement,
		Asset:          *asset,
		Amount:         "100",
		Status:         data.DraftPaymentStatus,
	})
	payment2 := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		ReceiverWallet: rwReady,
		Disbursement:   disbursement,
		Asset:          *asset,
		Amount:         "200",
		Status:         data.DraftPaymentStatus,
	})
	data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		ReceiverWallet: rwReady,
		Disbursement:   disbursement,
		Asset:          *asset,
		Amount:         "300",
		Status:         data.DraftPaymentStatus,
	})

	data.CreateCircleTransferRequestFixture(t, ctx, dbConnectionPool, data.CircleTransferRequest{
		IdempotencyKey:   "idempotency-key-1",
		PaymentID:        payment1.ID,
		CircleTransferID: utils.StringPtr("circle-transfer-id-1"),
	})

	data.CreateCircleTransferRequestFixture(t, ctx, dbConnectionPool, data.CircleTransferRequest{
		IdempotencyKey:   "idempotency-key-2",
		PaymentID:        payment2.ID,
		CircleTransferID: utils.StringPtr("circle-transfer-id-2"),
	})

	testCases := []struct {
		name          string
		prepareMocks  func(t *testing.T, mDistributionAccountResolver *sigMocks.MockDistributionAccountResolver)
		runAssertions func(t *testing.T, responseStatus int, response string)
	}{
		{
			name: "returns error when distribution account resolver fails",
			prepareMocks: func(t *testing.T, mDistributionAccountResolver *sigMocks.MockDistributionAccountResolver) {
				t.Helper()

				mDistributionAccountResolver.
					On("DistributionAccountFromContext", mock.Anything).
					Return(schema.TransactionAccount{}, errors.New("unexpected error")).
					Once()
			},
			runAssertions: func(t *testing.T, responseStatus int, response string) {
				t.Helper()

				assert.Equal(t, http.StatusInternalServerError, responseStatus)
				assert.JSONEq(t, `{"error":"Cannot retrieve payments"}`, string(response))
			},
		},
		{
			name: "successfully returns payments with circle transaction IDs",
			prepareMocks: func(t *testing.T, mDistributionAccountResolver *sigMocks.MockDistributionAccountResolver) {
				t.Helper()

				mDistributionAccountResolver.
					On("DistributionAccountFromContext", mock.Anything).
					Return(schema.TransactionAccount{Type: schema.DistributionAccountCircleDBVault}, nil).
					Maybe()
			},
			runAssertions: func(t *testing.T, responseStatus int, response string) {
				t.Helper()

				assert.Equal(t, http.StatusOK, responseStatus)

				var actualResponse httpresponse.PaginatedResponse
				err := json.Unmarshal([]byte(response), &actualResponse)
				require.NoError(t, err)

				assert.Equal(t, 3, actualResponse.Pagination.Total)

				var payments []data.Payment
				err = json.Unmarshal(actualResponse.Data, &payments)
				require.NoError(t, err)

				assert.Len(t, payments, 3)
				for _, payment := range payments {
					if payment.ID == payment1.ID {
						assert.Equal(t, "circle-transfer-id-1", *payment.CircleTransferRequestID)
					}
					if payment.ID == payment2.ID {
						assert.Equal(t, "circle-transfer-id-2", *payment.CircleTransferRequestID)
					}
					if payment.ID != payment1.ID && payment.ID != payment2.ID {
						assert.Nil(t, payment.CircleTransferRequestID)
					}
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mDistributionAccountResolver := sigMocks.NewMockDistributionAccountResolver(t)

			tc.prepareMocks(t, mDistributionAccountResolver)

			h := &PaymentsHandler{
				Models:                      models,
				DBConnectionPool:            dbConnectionPool,
				DistributionAccountResolver: mDistributionAccountResolver,
			}

			rr := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/payments", nil)
			require.NoError(t, err)
			http.HandlerFunc(h.GetPayments).ServeHTTP(rr, req)
			resp := rr.Result()
			defer resp.Body.Close()
			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			tc.runAssertions(t, resp.StatusCode, string(respBody))
		})
	}
}

func Test_PaymentHandler_GetPayments_Errors(t *testing.T) {
	dbConnectionPool := testutils.GetDBConnectionPool(t)

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
			expectedResponse:   fmt.Sprintf(`{"error":"request invalid", "extras":{"status":"invalid parameter. valid values are: %v"}}`, data.PaymentStatuses()),
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
	dbConnectionPool := testutils.GetDBConnectionPool(t)

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	mDistributionAccountResolver := sigMocks.NewMockDistributionAccountResolver(t)
	mDistributionAccountResolver.
		On("DistributionAccountFromContext", mock.Anything).
		Return(schema.TransactionAccount{Type: schema.DistributionAccountStellarEnv}, nil).
		Maybe()

	handler := &PaymentsHandler{
		Models:                      models,
		DBConnectionPool:            dbConnectionPool,
		DistributionAccountResolver: mDistributionAccountResolver,
	}

	ts := httptest.NewServer(http.HandlerFunc(handler.GetPayments))
	defer ts.Close()

	ctx := context.Background()

	// create fixtures
	asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
	wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")

	// create receivers
	receiver1 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	receiverWallet1 := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver1.ID, wallet.ID, data.RegisteredReceiversWalletStatus)

	receiver2 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	receiverWallet2 := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver2.ID, wallet.ID, data.RegisteredReceiversWalletStatus)

	// create disbursements
	disbursement1 := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Name:   "disbursement 1",
		Status: data.DraftDisbursementStatus,
		Asset:  asset,
		Wallet: wallet,
	})

	disbursement2 := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Name:   "disbursement 2",
		Status: data.ReadyDisbursementStatus,
		Asset:  asset,
		Wallet: wallet,
	})

	stellarTransactionID, err := utils.RandomString(64)
	require.NoError(t, err)
	stellarOperationID, err := utils.RandomString(32)
	require.NoError(t, err)

	// create payments
	paymentDraft := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		Amount:               "50",
		ExternalPaymentID:    uuid.NewString(),
		StellarTransactionID: stellarTransactionID,
		StellarOperationID:   stellarOperationID,
		Status:               data.DraftPaymentStatus,
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

	paymentReady := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		Amount:               "150",
		ExternalPaymentID:    uuid.NewString(),
		StellarTransactionID: stellarTransactionID,
		StellarOperationID:   stellarOperationID,
		Status:               data.ReadyPaymentStatus,
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

	paymentPending := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		Amount:               "200.50",
		ExternalPaymentID:    uuid.NewString(),
		StellarTransactionID: stellarTransactionID,
		StellarOperationID:   stellarOperationID,
		Status:               data.PendingPaymentStatus,
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

	paymentPaused := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		Amount:               "20",
		ExternalPaymentID:    uuid.NewString(),
		StellarTransactionID: stellarTransactionID,
		StellarOperationID:   stellarOperationID,
		Status:               data.PausedPaymentStatus,
		Disbursement:         disbursement2,
		Asset:                *asset,
		ReceiverWallet:       receiverWallet2,
		CreatedAt:            time.Date(2023, 3, 10, 23, 40, 20, 1431, time.UTC),
		UpdatedAt:            time.Date(2023, 4, 10, 23, 40, 20, 1431, time.UTC),
	})

	var paymentSuccess *data.Payment
	var paymentFailed *data.Payment
	var paymentCanceled *data.Payment

	for i, paymentStatus := range []data.PaymentStatus{data.SuccessPaymentStatus, data.FailedPaymentStatus, data.CanceledPaymentStatus} {
		payment := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			Amount:               "50",
			ExternalPaymentID:    uuid.NewString(),
			StellarTransactionID: stellarTransactionID,
			StellarOperationID:   stellarOperationID,
			Status:               paymentStatus,
			Disbursement:         disbursement1,
			Asset:                *asset,
			ReceiverWallet:       receiverWallet1,
			CreatedAt:            time.Date(2024, time.Month(i+1), 10, 23, 40, 20, 1431, time.UTC),
			UpdatedAt:            time.Date(2024, time.Month(i+1), 10, 23, 40, 20, 1431, time.UTC),
		})

		switch paymentStatus {
		case data.SuccessPaymentStatus:
			paymentSuccess = payment
		case data.FailedPaymentStatus:
			paymentFailed = payment
		case data.CanceledPaymentStatus:
			paymentCanceled = payment
		default:
			panic(fmt.Sprintf("invalid payment status: %s", paymentStatus))
		}
	}

	type TestCase struct {
		name               string
		queryParams        map[string]string
		expectedStatusCode int
		expectedPagination httpresponse.PaginationInfo
		expectedPayments   []data.Payment
	}

	tests := []TestCase{
		{
			name:               "fetch all payments without filters will use the default sorter (updated_at DESC)",
			queryParams:        map[string]string{},
			expectedStatusCode: http.StatusOK,
			expectedPagination: httpresponse.PaginationInfo{
				Next:  "",
				Prev:  "",
				Pages: 1,
				Total: 7,
			},
			expectedPayments: []data.Payment{*paymentCanceled, *paymentFailed, *paymentSuccess, *paymentPaused, *paymentDraft, *paymentPending, *paymentReady}, // default sorter: (updated_at DESC)
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
				Pages: 7,
				Total: 7,
			},
			expectedPayments: []data.Payment{*paymentDraft},
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
				Pages: 7,
				Total: 7,
			},
			expectedPayments: []data.Payment{*paymentReady},
		},
		{
			name: "fetch last page of payments with limit 1 and sort by created_at",
			queryParams: map[string]string{
				"page":       "7",
				"page_limit": "1",
				"sort":       "created_at",
				"direction":  "asc",
			},
			expectedStatusCode: http.StatusOK,
			expectedPagination: httpresponse.PaginationInfo{
				Next:  "",
				Prev:  "/payments?direction=asc&page=6&page_limit=1&sort=created_at",
				Pages: 7,
				Total: 7,
			},
			expectedPayments: []data.Payment{*paymentCanceled},
		},
		{
			name: "fetch payments for receiver1 with default sorter (updated_at DESC)",
			queryParams: map[string]string{
				"receiver_id": receiver1.ID,
			},
			expectedStatusCode: http.StatusOK,
			expectedPagination: httpresponse.PaginationInfo{
				Next:  "",
				Prev:  "",
				Pages: 1,
				Total: 5,
			},
			expectedPayments: []data.Payment{*paymentCanceled, *paymentFailed, *paymentSuccess, *paymentDraft, *paymentPending}, // default sorter: (updated_at DESC)
		},
		{
			name: "fetch payments for receiver2 with default sorter (updated_at DESC)",
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
			expectedPayments: []data.Payment{*paymentPaused, *paymentReady}, // default sorter: (updated_at DESC)
		},
		{
			name: "returns empty list when receiver_id is not found",
			queryParams: map[string]string{
				"receiver_id": "non_existing_id",
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
			expectedPayments: []data.Payment{*paymentDraft},
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
				Total: 4,
			},
			expectedPayments: []data.Payment{*paymentCanceled, *paymentFailed, *paymentSuccess, *paymentPaused}, // default sorter: (updated_at DESC)
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
			expectedPayments: []data.Payment{*paymentPending, *paymentReady}, // default sorter: (updated_at DESC)
		},
		{
			name: "query[p.id]",
			queryParams: map[string]string{
				"q": paymentDraft.ID[:5],
			},
			expectedStatusCode: http.StatusOK,
			expectedPagination: httpresponse.PaginationInfo{
				Next: "", Prev: "",
				Pages: 1, Total: 1,
			},
			expectedPayments: []data.Payment{*paymentDraft},
		},
		{
			name: "query[p.external_payment_id]",
			queryParams: map[string]string{
				"q": paymentReady.ExternalPaymentID[:5],
			},
			expectedStatusCode: http.StatusOK,
			expectedPagination: httpresponse.PaginationInfo{
				Next: "", Prev: "",
				Pages: 1, Total: 1,
			},
			expectedPayments: []data.Payment{*paymentReady},
		},
		{
			name: "query[rw.stellar_address]",
			queryParams: map[string]string{
				"q": receiverWallet1.StellarAddress[:5],
			},
			expectedStatusCode: http.StatusOK,
			expectedPagination: httpresponse.PaginationInfo{
				Next: "", Prev: "",
				Pages: 1, Total: 5,
			},
			expectedPayments: []data.Payment{*paymentCanceled, *paymentFailed, *paymentSuccess, *paymentDraft, *paymentPending}, // default sorter: (updated_at DESC)
		},
		{
			name: "query[d.name]",
			queryParams: map[string]string{
				"q": disbursement2.Name[5:],
			},
			expectedStatusCode: http.StatusOK,
			expectedPagination: httpresponse.PaginationInfo{
				Next: "", Prev: "",
				Pages: 1, Total: 2,
			},
			expectedPayments: []data.Payment{*paymentPaused, *paymentPending}, // default sorter: (updated_at DESC)
		},
	}

	for _, payment := range []data.Payment{*paymentDraft, *paymentPending, *paymentReady, *paymentPaused, *paymentSuccess, *paymentFailed, *paymentCanceled} {
		tests = append(tests, TestCase{
			name: "fetch payments with status=" + string(payment.Status),
			queryParams: map[string]string{
				"status": string(payment.Status),
			},
			expectedStatusCode: http.StatusOK,
			expectedPagination: httpresponse.PaginationInfo{
				Next:  "",
				Prev:  "",
				Pages: 1,
				Total: 1,
			},
			expectedPayments: []data.Payment{payment},
		})
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
			expectedJson, err := json.Marshal(tc.expectedPayments)
			require.NoError(t, err)
			assert.JSONEq(t, string(expectedJson), string(actualResponse.Data))
		})
	}
}

func Test_PaymentHandler_RetryPayments(t *testing.T) {
	dbConnectionPool := testutils.GetDBConnectionPool(t)

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	tnt := tenant.Tenant{ID: "tenant-id"}

	ctx := tenant.SaveTenantInContext(context.Background(), &tnt)

	wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "Wallet", "https://www.wallet.com", "www.wallet.com", "wallet://")
	asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")

	receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.RegisteredReceiversWalletStatus)

	disbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Wallet:            wallet,
		Asset:             asset,
		Status:            data.StartedDisbursementStatus,
		VerificationField: data.VerificationTypeDateOfBirth,
	})

	t.Run("returns Unauthorized when no token in the context", func(t *testing.T) {
		// Prepare the handler
		handler := PaymentsHandler{
			Models:           models,
			DBConnectionPool: dbConnectionPool,
		}

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

		// Prepare the handler and its mocks
		authManagerMock := auth.NewAuthManagerMock(t)
		authManagerMock.
			On("GetUser", ctx, "mytoken").
			Return(nil, errors.New("unexpected error")).
			Once()
		handler := PaymentsHandler{
			Models:           models,
			DBConnectionPool: dbConnectionPool,
			AuthManager:      authManagerMock,
		}

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

		// Prepare the handler and its mocks
		authManagerMock := auth.NewAuthManagerMock(t)
		authManagerMock.
			On("GetUser", ctx, "mytoken").
			Return(&auth.User{Email: "email@test.com"}, nil).
			Once()
		handler := PaymentsHandler{
			Models:           models,
			DBConnectionPool: dbConnectionPool,
			AuthManager:      authManagerMock,
		}

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

		// Prepare the handler and its mocks
		authManagerMock := auth.NewAuthManagerMock(t)
		authManagerMock.
			On("GetUser", ctx, "mytoken").
			Return(&auth.User{Email: "email@test.com"}, nil).
			Once()
		handler := PaymentsHandler{
			Models:           models,
			DBConnectionPool: dbConnectionPool,
			AuthManager:      authManagerMock,
		}

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
				"payment_ids": [%q, %q, %q, %q]
			}
		`, payment1.ID, payment2.ID, payment3.ID, payment4.ID))
		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, "/retry", payload)
		require.NoError(t, err)

		// Prepare the handler and its mocks
		authManagerMock := auth.NewAuthManagerMock(t)
		authManagerMock.
			On("GetUser", ctx, "mytoken").
			Return(&auth.User{Email: "email@test.com"}, nil).
			Once()
		handler := PaymentsHandler{
			Models:           models,
			DBConnectionPool: dbConnectionPool,
			AuthManager:      authManagerMock,
		}

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
				"payment_ids": [%q, %q]
			}
		`, payment1.ID, payment2.ID))
		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, "/retry", payload)
		require.NoError(t, err)

		// Prepare the handler and its mocks
		authManagerMock := auth.NewAuthManagerMock(t)
		authManagerMock.
			On("GetUser", ctx, "mytoken").
			Return(&auth.User{Email: "email@test.com"}, nil).
			Once()
		eventProducerMock := events.NewMockProducer(t)
		eventProducerMock.
			On("WriteMessages", ctx, []events.Message{
				{
					Topic:    events.PaymentReadyToPayTopic,
					Key:      tnt.ID,
					TenantID: tnt.ID,
					Type:     events.PaymentReadyToPayRetryFailedPayment,
					Data: schemas.EventPaymentsReadyToPayData{
						TenantID: tnt.ID,
						Payments: []schemas.PaymentReadyToPay{
							{ID: payment1.ID},
							{ID: payment2.ID},
						},
					},
				},
			}).
			Return(nil).
			Once()
		distAccountResolverMock := sigMocks.NewMockDistributionAccountResolver(t)
		distAccountResolverMock.
			On("DistributionAccountFromContext", mock.Anything).
			Return(schema.TransactionAccount{Type: schema.DistributionAccountStellarEnv}, nil).
			Once()
		handler := PaymentsHandler{
			Models:                      models,
			DBConnectionPool:            dbConnectionPool,
			AuthManager:                 authManagerMock,
			EventProducer:               eventProducerMock,
			DistributionAccountResolver: distAccountResolverMock,
		}

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

	t.Run("successfully retries failed circle payment", func(t *testing.T) {
		data.DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllCircleRecipientsFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllCircleTransferRequests(t, ctx, dbConnectionPool)

		failedPayment := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-1",
			StellarOperationID:   "operation-id-1",
			Status:               data.FailedPaymentStatus,
			Disbursement:         disbursement,
			ReceiverWallet:       receiverWallet,
			Asset:                *asset,
		})
		circleRecipient := data.CreateCircleRecipientFixture(t, ctx, dbConnectionPool, data.CircleRecipient{
			ReceiverWalletID:  receiverWallet.ID,
			Status:            data.CircleRecipientStatusDenied,
			CircleRecipientID: "circle-recipient-id-1",
			SyncAttempts:      5,
			LastSyncAttemptAt: time.Now(),
		})

		circleTnt := tenant.Tenant{ID: "tenant-id", DistributionAccountType: schema.DistributionAccountCircleDBVault}
		circleCtx := tenant.SaveTenantInContext(context.Background(), &circleTnt)
		circleCtx = context.WithValue(circleCtx, middleware.TokenContextKey, "mytoken")

		payload := strings.NewReader(fmt.Sprintf(`{ "payment_ids": [%q] } `, failedPayment.ID))
		req, err := http.NewRequestWithContext(circleCtx, http.MethodPatch, "/retry", payload)
		require.NoError(t, err)

		// Prepare the handler and its mocks
		authManagerMock := auth.NewAuthManagerMock(t)
		authManagerMock.
			On("GetUser", circleCtx, "mytoken").
			Return(&auth.User{Email: "email@test.com"}, nil).
			Once()
		eventProducerMock := events.NewMockProducer(t)
		eventProducerMock.
			On("WriteMessages", circleCtx, []events.Message{
				{
					Topic:    events.CirclePaymentReadyToPayTopic,
					Key:      tnt.ID,
					TenantID: tnt.ID,
					Type:     events.PaymentReadyToPayRetryFailedPayment,
					Data: schemas.EventPaymentsReadyToPayData{
						TenantID: tnt.ID,
						Payments: []schemas.PaymentReadyToPay{
							{ID: failedPayment.ID},
						},
					},
				},
			}).
			Return(nil).
			Once()
		distAccountResolverMock := sigMocks.NewMockDistributionAccountResolver(t)
		distAccountResolverMock.
			On("DistributionAccountFromContext", mock.Anything).
			Return(schema.TransactionAccount{Type: schema.DistributionAccountCircleDBVault}, nil).
			Once()
		handler := PaymentsHandler{
			Models:                      models,
			DBConnectionPool:            dbConnectionPool,
			AuthManager:                 authManagerMock,
			EventProducer:               eventProducerMock,
			DistributionAccountResolver: distAccountResolverMock,
		}

		rw := httptest.NewRecorder()
		http.HandlerFunc(handler.RetryPayments).ServeHTTP(rw, req)
		resp := rw.Result()
		defer resp.Body.Close()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		// Assert response
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, `{"message": "Payments retried successfully"}`, string(respBody))

		// Assert payment status
		previouslyFailedPayment, err := models.Payment.Get(circleCtx, failedPayment.ID, dbConnectionPool)
		require.NoError(t, err)
		assert.Equal(t, data.ReadyPaymentStatus, previouslyFailedPayment.Status)

		// Assert circle transfer request status
		circleRecipient, err = models.CircleRecipient.GetByReceiverWalletID(circleCtx, circleRecipient.ReceiverWalletID)
		require.NoError(t, err)
		assert.Empty(t, circleRecipient.Status)
		assert.Empty(t, circleRecipient.SyncAttempts)
		assert.Empty(t, circleRecipient.LastSyncAttemptAt)
		assert.Empty(t, circleRecipient.ResponseBody)
	})

	t.Run("returns error when tenant is not in the context", func(t *testing.T) {
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

		ctxWithoutTenant := context.WithValue(context.Background(), middleware.TokenContextKey, "mytoken")

		payload := strings.NewReader(fmt.Sprintf(`
			{
				"payment_ids": [%q, %q]
			}
		`, payment1.ID, payment2.ID))
		req, err := http.NewRequestWithContext(ctxWithoutTenant, http.MethodPatch, "/retry", payload)
		require.NoError(t, err)

		// Prepare the handler and its mocks
		authManagerMock := auth.NewAuthManagerMock(t)
		authManagerMock.
			On("GetUser", ctxWithoutTenant, "mytoken").
			Return(&auth.User{Email: "email@test.com"}, nil).
			Once()
		distAccountResolverMock := sigMocks.NewMockDistributionAccountResolver(t)
		handler := PaymentsHandler{
			Models:                      models,
			DBConnectionPool:            dbConnectionPool,
			AuthManager:                 authManagerMock,
			DistributionAccountResolver: distAccountResolverMock,
		}

		rw := httptest.NewRecorder()
		http.HandlerFunc(handler.RetryPayments).ServeHTTP(rw, req)
		resp := rw.Result()
		defer resp.Body.Close()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		assert.JSONEq(t, `{"error": "An internal error occurred while processing this request."}`, string(respBody))
	})

	t.Run("logs to crashTracker when EventProducer fails to write a message", func(t *testing.T) {
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

		ctx = context.WithValue(ctx, middleware.TokenContextKey, "mytoken")

		payload := strings.NewReader(fmt.Sprintf(`
			{
				"payment_ids": [%q]
			}
		`, payment1.ID))
		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, "/retry", payload)
		require.NoError(t, err)

		// Prepare the handler and its mocks
		authManagerMock := auth.NewAuthManagerMock(t)
		authManagerMock.
			On("GetUser", ctx, "mytoken").
			Return(&auth.User{Email: "email@test.com"}, nil).
			Once()
		eventProducerMock := events.NewMockProducer(t)
		eventProducerMock.
			On("WriteMessages", ctx, []events.Message{
				{
					Topic:    events.PaymentReadyToPayTopic,
					Key:      tnt.ID,
					TenantID: tnt.ID,
					Type:     events.PaymentReadyToPayRetryFailedPayment,
					Data: schemas.EventPaymentsReadyToPayData{
						TenantID: tnt.ID,
						Payments: []schemas.PaymentReadyToPay{
							{ID: payment1.ID},
						},
					},
				},
			}).
			Return(errors.New("unexpected error")).
			Once()
		crashTrackerMock := &crashtracker.MockCrashTrackerClient{}
		crashTrackerMock.
			On("LogAndReportErrors", mock.Anything, mock.Anything, "writing retry payment message on the event producer").
			Once()
		distAccountResolverMock := sigMocks.NewMockDistributionAccountResolver(t)
		distAccountResolverMock.
			On("DistributionAccountFromContext", mock.Anything).
			Return(schema.TransactionAccount{Type: schema.DistributionAccountStellarEnv}, nil).
			Once()
		handler := PaymentsHandler{
			Models:                      models,
			DBConnectionPool:            dbConnectionPool,
			AuthManager:                 authManagerMock,
			EventProducer:               eventProducerMock,
			CrashTrackerClient:          crashTrackerMock,
			DistributionAccountResolver: distAccountResolverMock,
		}

		rw := httptest.NewRecorder()
		http.HandlerFunc(handler.RetryPayments).ServeHTTP(rw, req)

		resp := rw.Result()
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, `{"message":"Payments retried successfully"}`, string(respBody))
	})

	t.Run("logs when couldn't write message because EventProducer is nil", func(t *testing.T) {
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
				"payment_ids": [%q, %q]
			}
		`, payment1.ID, payment2.ID))
		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, "/retry", payload)
		require.NoError(t, err)

		// Prepare the handler and its mocks
		authManagerMock := auth.NewAuthManagerMock(t)
		authManagerMock.
			On("GetUser", ctx, "mytoken").
			Return(&auth.User{Email: "email@test.com"}, nil).
			Once()
		distAccountResolverMock := sigMocks.NewMockDistributionAccountResolver(t)
		distAccountResolverMock.
			On("DistributionAccountFromContext", mock.Anything).
			Return(schema.TransactionAccount{Type: schema.DistributionAccountStellarEnv}, nil).
			Once()
		handler := PaymentsHandler{
			Models:                      models,
			DBConnectionPool:            dbConnectionPool,
			AuthManager:                 authManagerMock,
			DistributionAccountResolver: distAccountResolverMock,
		}

		getEntries := log.DefaultLogger.StartTest(log.DebugLevel)

		handler.EventProducer = nil
		rw := httptest.NewRecorder()
		http.HandlerFunc(handler.RetryPayments).ServeHTTP(rw, req)

		resp := rw.Result()
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, `{"message":"Payments retried successfully"}`, string(respBody))

		msg := events.Message{
			Topic:    events.PaymentReadyToPayTopic,
			Key:      tnt.ID,
			TenantID: tnt.ID,
			Type:     events.PaymentReadyToPayRetryFailedPayment,
			Data: schemas.EventPaymentsReadyToPayData{
				TenantID: tnt.ID,
				Payments: []schemas.PaymentReadyToPay{
					{ID: payment1.ID},
					{ID: payment2.ID},
				},
			},
		}

		entries := getEntries()
		require.Len(t, entries, 1)
		assert.Contains(t, fmt.Sprintf("event producer is nil, could not publish messages %+v", []events.Message{msg}), entries[0].Message)
	})
}

func Test_PaymentsHandler_getPaymentsWithCount(t *testing.T) {
	dbConnectionPool := testutils.GetDBConnectionPool(t)

	ctx := context.Background()
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	mDistributionAccountResolver := sigMocks.NewMockDistributionAccountResolver(t)

	handler := &PaymentsHandler{
		Models:                      models,
		DBConnectionPool:            dbConnectionPool,
		DistributionAccountResolver: mDistributionAccountResolver,
	}

	mDistributionAccountResolver.
		On("DistributionAccountFromContext", mock.Anything).
		Return(schema.TransactionAccount{Type: schema.DistributionAccountStellarEnv}, nil).
		Maybe()

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
		require.ErrorContains(t, err, `error counting payments: error counting payments: pq: invalid input value for enum payment_status: "INVALID"`)
	})

	asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
	wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")

	receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.DraftReceiversWalletStatus)

	disbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Name:   "disbursement 1",
		Status: data.DraftDisbursementStatus,
		Asset:  asset,
		Wallet: wallet,
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

		params := data.QueryParams{SortBy: data.DefaultPaymentSortField, SortOrder: data.DefaultPaymentSortOrder}
		response, err := handler.getPaymentsWithCount(ctx, &params)
		require.NoError(t, err)

		assert.Equal(t, response.Total, 2)
		assert.Equal(t, response.Result, []data.Payment{*payment2, *payment})
	})
}

func Test_PaymentsHandler_PatchPaymentStatus(t *testing.T) {
	dbConnectionPool := testutils.GetDBConnectionPool(t)

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	authManagerMock := &auth.AuthManagerMock{}

	handler := &PaymentsHandler{
		Models:           models,
		DBConnectionPool: models.DBConnectionPool,
		AuthManager:      authManagerMock,
	}

	ctx := context.Background()

	r := chi.NewRouter()
	r.Patch("/payments/{id}/status", handler.PatchPaymentStatus)

	// create fixtures
	wallet := data.CreateDefaultWalletFixture(t, ctx, dbConnectionPool)
	asset := data.GetAssetFixture(t, ctx, dbConnectionPool, data.FixtureAssetUSDC)

	// create disbursements
	startedDisbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Name:   "ready disbursement",
		Status: data.StartedDisbursementStatus,
		Asset:  asset,
		Wallet: wallet,
	})

	// create disbursement receivers
	receiver1 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	receiver2 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})

	rw1 := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver1.ID, wallet.ID, data.RegisteredReceiversWalletStatus)
	rw2 := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver2.ID, wallet.ID, data.RegisteredReceiversWalletStatus)

	readyPayment := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		ReceiverWallet: rw1,
		Disbursement:   startedDisbursement,
		Asset:          *asset,
		Amount:         "100",
		Status:         data.ReadyPaymentStatus,
	})
	draftPayment := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		ReceiverWallet: rw2,
		Disbursement:   startedDisbursement,
		Asset:          *asset,
		Amount:         "200",
		Status:         data.DraftPaymentStatus,
	})

	reqBody := bytes.NewBuffer(nil)
	t.Run("invalid body", func(t *testing.T) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, fmt.Sprintf("/payments/%s/status", readyPayment.ID), reqBody)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		require.Equal(t, http.StatusBadRequest, rr.Code)
		require.Contains(t, rr.Body.String(), "invalid request body")
	})

	t.Run("invalid status", func(t *testing.T) {
		err := json.NewEncoder(reqBody).Encode(PatchPaymentStatusRequest{Status: "INVALID"})
		require.NoError(t, err)

		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, fmt.Sprintf("/payments/%s/status", readyPayment.ID), reqBody)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		require.Equal(t, http.StatusBadRequest, rr.Code)
		require.Contains(t, rr.Body.String(), "invalid status")
	})

	t.Run("payment not found", func(t *testing.T) {
		err := json.NewEncoder(reqBody).Encode(PatchPaymentStatusRequest{Status: "CANCELED"})
		require.NoError(t, err)

		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, fmt.Sprintf("/payments/%s/status", "invalid-id"), reqBody)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		require.Equal(t, http.StatusNotFound, rr.Code)
		require.Contains(t, rr.Body.String(), "payment not found")
	})

	t.Run("payment can't be canceled", func(t *testing.T) {
		err := json.NewEncoder(reqBody).Encode(PatchDisbursementStatusRequest{Status: "CANCELED"})
		require.NoError(t, err)

		req, err := http.NewRequest(http.MethodPatch, fmt.Sprintf("/payments/%s/status", draftPayment.ID), reqBody)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		require.Equal(t, http.StatusBadRequest, rr.Code)
		require.Contains(t, rr.Body.String(), services.ErrPaymentNotReadyToCancel.Error())
	})

	t.Run("payment status can't be changed", func(t *testing.T) {
		err := json.NewEncoder(reqBody).Encode(PatchDisbursementStatusRequest{Status: "READY"})
		require.NoError(t, err)

		req, err := http.NewRequest(http.MethodPatch, fmt.Sprintf("/payments/%s/status", readyPayment.ID), reqBody)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		require.Equal(t, http.StatusBadRequest, rr.Code)
		require.Contains(t, rr.Body.String(), services.ErrPaymentStatusCantBeChanged.Error())
	})

	t.Run("payment canceled successfully", func(t *testing.T) {
		err := json.NewEncoder(reqBody).Encode(PatchDisbursementStatusRequest{Status: "Canceled"})
		require.NoError(t, err)

		req, err := http.NewRequest(http.MethodPatch, fmt.Sprintf("/payments/%s/status", readyPayment.ID), reqBody)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)
		require.Contains(t, rr.Body.String(), "Payment canceled")

		payment, err := handler.Models.Payment.Get(context.Background(), readyPayment.ID, models.DBConnectionPool)
		require.NoError(t, err)
		require.Equal(t, data.CanceledPaymentStatus, payment.Status)
	})

	authManagerMock.AssertExpectations(t)
}

func Test_PaymentsHandler_PostPayment(t *testing.T) {
	dbConnectionPool := testutils.GetDBConnectionPool(t)

	ctx := context.Background()
	tnt := &tenant.Tenant{ID: "thunderhawk-42"}
	ctx = tenant.SaveTenantInContext(ctx, tnt)

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	// Create test data
	asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "CERAMITE", "GISSUER1234567890ABCDEFGHIJKLMNOPQRSTUVWXYZ123456")
	wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "Fortress Monastery", "https://fortress.com", "fortress.com", "fortress://")

	// Associate asset with wallet
	_, err = dbConnectionPool.ExecContext(ctx,
		"INSERT INTO wallets_assets (wallet_id, asset_id) VALUES ($1, $2)",
		wallet.ID, asset.ID)
	require.NoError(t, err)

	receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
		Email: "dante@baal.imperium",
	})

	tests := []struct {
		name             string
		requestBody      string
		setupMocks       func(*testing.T, *auth.AuthManagerMock, *sigMocks.MockDistributionAccountResolver, *mocks.MockDistributionAccountService, *events.MockProducer)
		expectedStatus   int
		expectedResponse string
		validateResponse func(*testing.T, string)
	}{
		{
			name: "successful direct payment creation",
			requestBody: fmt.Sprintf(`{
				"amount": "150.50",
				"asset": {"id": %q},
				"receiver": {"id": %q},
				"wallet": {"id": %q},
				"external_payment_id": "BAAL-CRUSADE-001"
			}`, asset.ID, receiver.ID, wallet.ID),
			setupMocks: func(t *testing.T, authMock *auth.AuthManagerMock, distResolver *sigMocks.MockDistributionAccountResolver, distService *mocks.MockDistributionAccountService, eventProducer *events.MockProducer) {
				authMock.On("GetUser", mock.Anything, "test-token").Return(&auth.User{
					ID:    "user-dante",
					Email: "commander.dante@baal.imperium",
				}, nil)

				distResolver.On("DistributionAccountFromContext", mock.Anything).Return(
					schema.TransactionAccount{Type: schema.DistributionAccountStellarDBVault}, nil)

				distService.On("GetBalance", mock.Anything, mock.Anything, *asset).Return(float64(1000), nil)

				eventProducer.On("WriteMessages", mock.Anything, mock.Anything).Return(nil).Maybe()
			},
			expectedStatus: http.StatusCreated,
			validateResponse: func(t *testing.T, response string) {
				var payment data.Payment
				err := json.Unmarshal([]byte(response), &payment)
				require.NoError(t, err)
				assert.Equal(t, "150.5000000", payment.Amount)
				assert.Equal(t, "BAAL-CRUSADE-001", payment.ExternalPaymentID)
				assert.Equal(t, data.PaymentTypeDirect, payment.PaymentType)
				assert.Nil(t, payment.Disbursement)
			},
		},
		{
			name:        "invalid request body",
			requestBody: `{invalid json`,
			setupMocks: func(t *testing.T, authMock *auth.AuthManagerMock, distResolver *sigMocks.MockDistributionAccountResolver, distService *mocks.MockDistributionAccountService, eventProducer *events.MockProducer) {
				authMock.On("GetUser", mock.Anything, "test-token").Return(&auth.User{
					ID: "user-test",
				}, nil)
			},
			expectedStatus:   http.StatusBadRequest,
			expectedResponse: `{"error": "The request was invalid in some way."}`,
		},
		{
			name: "invalid amount",
			requestBody: `{
				"amount": "not-a-number",
				"asset": {"id": "test"},
				"receiver": {"id": "test"},
				"wallet": {"id": "test"}
			}`,
			setupMocks: func(t *testing.T, authMock *auth.AuthManagerMock, distResolver *sigMocks.MockDistributionAccountResolver, distService *mocks.MockDistributionAccountService, eventProducer *events.MockProducer) {
				authMock.On("GetUser", mock.Anything, "test-token").Return(&auth.User{
					ID: "user-test",
				}, nil)
			},
			expectedStatus:   http.StatusBadRequest,
			expectedResponse: `{"error": "invalid amount"}`,
		},
		{
			name:        "unauthorized - no token",
			requestBody: `{}`,
			setupMocks: func(t *testing.T, authMock *auth.AuthManagerMock, distResolver *sigMocks.MockDistributionAccountResolver, distService *mocks.MockDistributionAccountService, eventProducer *events.MockProducer) {
				// Remove token from context - will be done in test
			},
			expectedStatus:   http.StatusUnauthorized,
			expectedResponse: `{"error": "Not authorized."}`,
		},
		{
			name: "distribution account resolution fails",
			requestBody: fmt.Sprintf(`{
				"amount": "100.00",
				"asset": {"id": %q},
				"receiver": {"id": %q},
				"wallet": {"id": %q}
			}`, asset.ID, receiver.ID, wallet.ID),
			setupMocks: func(t *testing.T, authMock *auth.AuthManagerMock, distResolver *sigMocks.MockDistributionAccountResolver, distService *mocks.MockDistributionAccountService, eventProducer *events.MockProducer) {
				authMock.On("GetUser", mock.Anything, "test-token").Return(&auth.User{
					ID: "user-test",
				}, nil)

				distResolver.On("DistributionAccountFromContext", mock.Anything).Return(
					schema.TransactionAccount{}, errors.New("resolution failed"))
			},
			expectedStatus:   http.StatusInternalServerError,
			expectedResponse: `{"error": "An internal error occurred while processing this request."}`,
		},
		{
			name: "asset not found",
			requestBody: `{
				"amount": "100.00",
				"asset": {"id": "non-existent-asset"},
				"receiver": {"id": "` + receiver.ID + `"},
				"wallet": {"id": "` + wallet.ID + `"}
			}`,
			setupMocks: func(t *testing.T, authMock *auth.AuthManagerMock, distResolver *sigMocks.MockDistributionAccountResolver, distService *mocks.MockDistributionAccountService, eventProducer *events.MockProducer) {
				authMock.On("GetUser", mock.Anything, "test-token").Return(&auth.User{
					ID: "user-test",
				}, nil)

				distResolver.On("DistributionAccountFromContext", mock.Anything).Return(
					schema.TransactionAccount{Type: schema.DistributionAccountStellarDBVault}, nil)
			},
			expectedStatus:   http.StatusNotFound,
			expectedResponse: `{"error": "resource not found"}`,
		},
		{
			name: "insufficient balance",
			requestBody: fmt.Sprintf(`{
				"amount": "10000.00",
				"asset": {"id": %q},
				"receiver": {"id": %q},
				"wallet": {"id": %q}
			}`, asset.ID, receiver.ID, wallet.ID),
			setupMocks: func(t *testing.T, authMock *auth.AuthManagerMock, distResolver *sigMocks.MockDistributionAccountResolver, distService *mocks.MockDistributionAccountService, eventProducer *events.MockProducer) {
				authMock.On("GetUser", mock.Anything, "test-token").Return(&auth.User{
					ID: "user-test",
				}, nil)

				distResolver.On("DistributionAccountFromContext", mock.Anything).Return(
					schema.TransactionAccount{Type: schema.DistributionAccountStellarDBVault}, nil)

				distService.On("GetBalance", mock.Anything, mock.Anything, *asset).Return(float64(100), nil)
			},
			expectedStatus:   http.StatusConflict,
			expectedResponse: `insufficient balance`,
		},
		{
			name: "wallet not enabled",
			requestBody: fmt.Sprintf(`{
				"amount": "100.00",
				"asset": {"id": %q},
				"receiver": {"id": %q},
				"wallet": {"id": %q}
			}`, asset.ID, receiver.ID, wallet.ID),
			setupMocks: func(t *testing.T, authMock *auth.AuthManagerMock, distResolver *sigMocks.MockDistributionAccountResolver, distService *mocks.MockDistributionAccountService, eventProducer *events.MockProducer) {
				// Disable wallet
				_, err := dbConnectionPool.ExecContext(ctx,
					"UPDATE wallets SET enabled = false WHERE id = $1", wallet.ID)
				require.NoError(t, err)
				t.Cleanup(func() {
					_, _ = dbConnectionPool.ExecContext(ctx,
						"UPDATE wallets SET enabled = true WHERE id = $1", wallet.ID)
				})

				authMock.On("GetUser", mock.Anything, "test-token").Return(&auth.User{
					ID: "user-test",
				}, nil)

				distResolver.On("DistributionAccountFromContext", mock.Anything).Return(
					schema.TransactionAccount{Type: schema.DistributionAccountStellarDBVault}, nil)
			},
			expectedStatus:   http.StatusBadRequest,
			expectedResponse: `is not enabled for payments`,
		},
		{
			name: "asset not supported by wallet",
			requestBody: fmt.Sprintf(`{
				"amount": "100.00",
				"asset": {"type": "native"},
				"receiver": {"id": %q},
				"wallet": {"id": %q}
			}`, receiver.ID, wallet.ID),
			setupMocks: func(t *testing.T, authMock *auth.AuthManagerMock, distResolver *sigMocks.MockDistributionAccountResolver, distService *mocks.MockDistributionAccountService, eventProducer *events.MockProducer) {
				authMock.On("GetUser", mock.Anything, "test-token").Return(&auth.User{
					ID: "user-test",
				}, nil)

				distResolver.On("DistributionAccountFromContext", mock.Anything).Return(
					schema.TransactionAccount{Type: schema.DistributionAccountStellarDBVault}, nil)
			},
			expectedStatus:   http.StatusBadRequest,
			expectedResponse: `is not supported by wallet`,
		},
		{
			name: "complex reference - receiver by email, asset by type",
			requestBody: `{
				"amount": "75.25",
				"asset": {
					"type": "classic",
					"code": "CERAMITE",
					"issuer": "GISSUER1234567890ABCDEFGHIJKLMNOPQRSTUVWXYZ123456"
				},
				"receiver": {"email": "dante@baal.imperium"},
				"wallet": {"id": "` + wallet.ID + `"}
			}`,
			setupMocks: func(t *testing.T, authMock *auth.AuthManagerMock, distResolver *sigMocks.MockDistributionAccountResolver, distService *mocks.MockDistributionAccountService, eventProducer *events.MockProducer) {
				authMock.On("GetUser", mock.Anything, "test-token").Return(&auth.User{
					ID:    "user-test",
					Email: "test@imperium.gov",
				}, nil)

				distResolver.On("DistributionAccountFromContext", mock.Anything).Return(
					schema.TransactionAccount{Type: schema.DistributionAccountStellarDBVault}, nil)

				distService.On("GetBalance", mock.Anything, mock.Anything, *asset).Return(float64(1000), nil)

				eventProducer.On("WriteMessages", mock.Anything, mock.Anything).Return(nil).Maybe()
			},
			expectedStatus: http.StatusCreated,
			validateResponse: func(t *testing.T, response string) {
				var payment data.Payment
				err := json.Unmarshal([]byte(response), &payment)
				require.NoError(t, err)
				assert.Equal(t, "75.2500000", payment.Amount)
				assert.Equal(t, data.PaymentTypeDirect, payment.PaymentType)
				assert.Equal(t, asset.ID, payment.Asset.ID)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Clean up any created payments
			t.Cleanup(func() {
				data.DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
				data.DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
			})

			authMock := &auth.AuthManagerMock{}
			distResolverMock := sigMocks.NewMockDistributionAccountResolver(t)
			distServiceMock := &mocks.MockDistributionAccountService{}
			eventProducerMock := events.NewMockProducer(t)

			directPaymentService := services.NewDirectPaymentService(models, dbConnectionPool)
			directPaymentService.DistributionAccountService = distServiceMock
			directPaymentService.EventProducer = eventProducerMock

			handler := &PaymentsHandler{
				Models:                      models,
				DBConnectionPool:            dbConnectionPool,
				AuthManager:                 authMock,
				DistributionAccountResolver: distResolverMock,
				DirectPaymentService:        directPaymentService,
			}

			tc.setupMocks(t, authMock, distResolverMock, distServiceMock, eventProducerMock)

			// Create request
			reqCtx := ctx
			if tc.name != "unauthorized - no token" {
				reqCtx = context.WithValue(ctx, middleware.TokenContextKey, "test-token")
			}

			req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, "/payments",
				strings.NewReader(tc.requestBody))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			// Execute request
			rr := httptest.NewRecorder()
			handler.PostPayment(rr, req)

			// Validate response
			assert.Equal(t, tc.expectedStatus, rr.Code)

			if tc.validateResponse != nil {
				tc.validateResponse(t, rr.Body.String())
			} else if tc.expectedResponse != "" {
				assert.Contains(t, rr.Body.String(), tc.expectedResponse)
			}

			// Verify mocks
			authMock.AssertExpectations(t)
			distResolverMock.AssertExpectations(t)
			distServiceMock.AssertExpectations(t)
			eventProducerMock.AssertExpectations(t)
		})
	}
}
