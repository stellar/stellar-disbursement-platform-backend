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
	"github.com/shopspring/decimal"
	"github.com/stellar/go-stellar-sdk/clients/horizonclient"
	"github.com/stellar/go-stellar-sdk/protocols/horizon"
	"github.com/stellar/go-stellar-sdk/protocols/horizon/base"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpresponse"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/testutils"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	sigMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
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
		SenderAddress:     "GDOSPKDCGMYZTPHXPFAZSSVIHNKBPEGQXQVEWEJ4JXMKYZNXEVCFGMC2",
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

		wantJSON := `{
			"id": "` + payment.ID + `",
			"amount": "50.0000000",
			"stellar_transaction_id": "` + payment.StellarTransactionID + `",
			"stellar_operation_id": "` + payment.StellarOperationID + `",
			"status": "DRAFT",
			"type": "DISBURSEMENT",
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
					"id": "` + receiver.ID + `"
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
			"external_payment_id": "` + payment.ExternalPaymentID + `",
			"sender_address": "` + payment.SenderAddress + `"
		}`

		assert.JSONEq(t, wantJSON, rr.Body.String())
	})

	t.Run("error payment not found for given ID", func(t *testing.T) {
		// test
		req, err := http.NewRequest("GET", "/payments/invalid_id", nil)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		// assert response
		assert.Equal(t, http.StatusNotFound, rr.Code)

		wantJSON := `{
			"error": "Cannot retrieve payment with ID: invalid_id"
		}`
		assert.JSONEq(t, wantJSON, rr.Body.String())
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
				assert.JSONEq(t, `{"error":"Cannot retrieve payments"}`, response)
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
			name: "returns error when page_limit is zero",
			queryParams: map[string]string{
				"page_limit": "0",
			},
			expectedStatusCode: http.StatusBadRequest,
			expectedResponse:   `{"error":"request invalid", "extras":{"page_limit":"parameter must be a positive integer"}}`,
		},
		{
			name: "returns error when page_limit exceeds max",
			queryParams: map[string]string{
				"page_limit": fmt.Sprintf("%d", validators.MaxPageLimit+1),
			},
			expectedStatusCode: http.StatusBadRequest,
			expectedResponse:   fmt.Sprintf(`{"error":"request invalid", "extras":{"page_limit":"parameter must be less than or equal to %d"}}`, validators.MaxPageLimit),
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

	directPaymentReady := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		Amount:               "150",
		ExternalPaymentID:    uuid.NewString(),
		StellarTransactionID: stellarTransactionID,
		StellarOperationID:   stellarOperationID,
		Status:               data.ReadyPaymentStatus,
		Asset:                *asset,
		ReceiverWallet:       receiverWallet2,
		Type:                 data.PaymentTypeDirect,
		CreatedAt:            time.Date(2023, 1, 10, 23, 40, 20, 1431, time.UTC),
		UpdatedAt:            time.Date(2023, 12, 10, 23, 40, 20, 1431, time.UTC),
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
				Total: 8,
			},
			expectedPayments: []data.Payment{*paymentCanceled, *paymentFailed, *paymentSuccess, *directPaymentReady, *paymentPaused, *paymentDraft, *paymentPending, *paymentReady}, // correct updated_at DESC order
		},
		{
			name:               "fetch all payments with DISBURSEMENT type filter",
			queryParams:        map[string]string{"type": "disbursement"},
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
			name:               "fetch all payments with DIRECT type filter",
			queryParams:        map[string]string{"type": "direct"},
			expectedStatusCode: http.StatusOK,
			expectedPagination: httpresponse.PaginationInfo{
				Next:  "",
				Prev:  "",
				Pages: 1,
				Total: 1,
			},
			expectedPayments: []data.Payment{*directPaymentReady},
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
				Pages: 8,
				Total: 8,
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
				Pages: 8,
				Total: 8,
			},
			expectedPayments: []data.Payment{*directPaymentReady},
		},
		{
			name: "fetch last page of payments with limit 1 and sort by created_at",
			queryParams: map[string]string{
				"page":       "8",
				"page_limit": "1",
				"sort":       "created_at",
				"direction":  "asc",
			},
			expectedStatusCode: http.StatusOK,
			expectedPagination: httpresponse.PaginationInfo{
				Next:  "",
				Prev:  "/payments?direction=asc&page=7&page_limit=1&sort=created_at",
				Pages: 8,
				Total: 8,
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
				Total: 3,
			},
			expectedPayments: []data.Payment{*directPaymentReady, *paymentPaused, *paymentReady}, // updated_at DESC order
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
				Total: 3,
			},
			expectedPayments: []data.Payment{*directPaymentReady, *paymentPending, *paymentReady},
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

	for _, payment := range []data.Payment{*paymentDraft, *paymentPending, *paymentReady, *paymentPaused, *paymentSuccess, *paymentFailed, *paymentCanceled, *directPaymentReady} {
		expectedTotal := 1
		expectedPayments := []data.Payment{payment}

		if payment.Status == data.ReadyPaymentStatus {
			expectedTotal = 2
			expectedPayments = []data.Payment{*directPaymentReady, *paymentReady}
		}

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
				Total: expectedTotal,
			},
			expectedPayments: expectedPayments,
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
			expectedJSON, err := json.Marshal(tc.expectedPayments)
			require.NoError(t, err)
			assert.JSONEq(t, string(expectedJSON), string(actualResponse.Data))
		})
	}
}

func Test_PaymentHandler_RetryPayments(t *testing.T) {
	dbConnectionPool := testutils.GetDBConnectionPool(t)

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	tnt := schema.Tenant{ID: "tenant-id"}

	ctx := sdpcontext.SetTenantInContext(context.Background(), &tnt)

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
		ctx = sdpcontext.SetTokenInContext(ctx, "mytoken")

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
		ctx = sdpcontext.SetTokenInContext(ctx, "mytoken")

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
		ctx = sdpcontext.SetTokenInContext(ctx, "mytoken")

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

		ctx = sdpcontext.SetTokenInContext(ctx, "mytoken")

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

		ctx = sdpcontext.SetTokenInContext(ctx, "mytoken")

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
		// distAccountResolverMock.
		//	On("DistributionAccountFromContext", mock.Anything).
		//	Return(schema.TransactionAccount{Type: schema.DistributionAccountStellarEnv}, nil).
		//	Once()
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

		circleTnt := schema.Tenant{ID: "tenant-id", DistributionAccountType: schema.DistributionAccountCircleDBVault}
		circleCtx := sdpcontext.SetTenantInContext(context.Background(), &circleTnt)
		circleCtx = sdpcontext.SetTokenInContext(circleCtx, "mytoken")

		payload := strings.NewReader(fmt.Sprintf(`{ "payment_ids": [%q] } `, failedPayment.ID))
		req, err := http.NewRequestWithContext(circleCtx, http.MethodPatch, "/retry", payload)
		require.NoError(t, err)

		// Prepare the handler and its mocks
		authManagerMock := auth.NewAuthManagerMock(t)
		authManagerMock.
			On("GetUser", circleCtx, "mytoken").
			Return(&auth.User{Email: "email@test.com"}, nil).
			Once()

		handler := PaymentsHandler{
			Models:                      models,
			DBConnectionPool:            dbConnectionPool,
			AuthManager:                 authManagerMock,
			DistributionAccountResolver: sigMocks.NewMockDistributionAccountResolver(t),
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

		ctxWithoutTenant := sdpcontext.SetTokenInContext(context.Background(), "mytoken")

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
	ctx := sdpcontext.SetUserIDInContext(context.Background(), "user-id")
	ctx = sdpcontext.SetTenantInContext(ctx, &schema.Tenant{ID: "battle-barge-001"})

	dbConnectionPool := testutils.GetDBConnectionPool(t)
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	t.Run("successful direct payment creation", func(t *testing.T) {
		t.Cleanup(func() {
			data.DeleteAllFixtures(t, ctx, dbConnectionPool)
		})

		asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "CERAMITE", "GBXGQJWVLWOYHFLVTKWV5FGHA3LNYY2JQKM7OAJAUEQFU6LPCSEFVXON")
		wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "Fortress Monastery", "https://fortress.com", "fortress.com", "fortress://")

		_, err = dbConnectionPool.ExecContext(ctx,
			"INSERT INTO wallets_assets (wallet_id, asset_id) VALUES ($1, $2)",
			wallet.ID, asset.ID)
		require.NoError(t, err)

		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
			Email: "dante@baal.imperium",
		})
		data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.RegisteredReceiversWalletStatus)

		requestBody := fmt.Sprintf(`{
			"amount": "150.50",
			"asset": {"id": %q},
			"receiver": {"id": %q},
			"wallet": {"id": %q},
			"external_payment_id": "BAAL-CRUSADE-001"
		}`, asset.ID, receiver.ID, wallet.ID)

		authMock := &auth.AuthManagerMock{}
		distResolverMock := sigMocks.NewMockDistributionAccountResolver(t)
		distServiceMock := &mocks.MockDistributionAccountService{}
		horizonClientMock := &horizonclient.MockClient{}

		distributionAccPubKey := "GAAHIL6ZW4QFNLCKALZ3YOIWPP4TXQ7B7J5IU7RLNVGQAV6GFDZHLDTA"
		stellarDistAccount := schema.TransactionAccount{
			Type:    schema.DistributionAccountStellarDBVault,
			Address: distributionAccPubKey,
		}

		authMock.On("GetUserByID", mock.Anything, "user-id").Return(&auth.User{
			ID:    "user-dante",
			Email: "commander.dante@baal.imperium",
		}, nil)

		distResolverMock.On("DistributionAccountFromContext", mock.Anything).Return(stellarDistAccount, nil)

		horizonClientMock.On("AccountDetail", horizonclient.AccountRequest{
			AccountID: distributionAccPubKey,
		}).Return(horizon.Account{
			AccountID: distributionAccPubKey,
			Sequence:  123,
			Balances: []horizon.Balance{
				{
					Balance: "10000000",
					Asset: base.Asset{
						Code:   asset.Code,
						Issuer: asset.Issuer,
					},
				},
			},
		}, nil).Once()

		distServiceMock.On("GetBalance", mock.Anything, &stellarDistAccount, *asset).Return(decimal.NewFromFloat(1000), nil)

		directPaymentService := services.NewDirectPaymentService(
			models,
			distServiceMock,
			engine.SubmitterEngine{HorizonClient: horizonClientMock},
		)

		handler := &PaymentsHandler{
			Models:                      models,
			DBConnectionPool:            dbConnectionPool,
			AuthManager:                 authMock,
			DistributionAccountResolver: distResolverMock,
			DirectPaymentService:        directPaymentService,
		}
		var req *http.Request
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, "/payments",
			strings.NewReader(requestBody))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		rr := httptest.NewRecorder()
		handler.PostDirectPayment(rr, req)

		assert.Equal(t, http.StatusCreated, rr.Code)

		var payment data.Payment
		err = json.Unmarshal(rr.Body.Bytes(), &payment)
		require.NoError(t, err)
		assert.Equal(t, "150.5000000", payment.Amount)
		assert.Equal(t, "BAAL-CRUSADE-001", payment.ExternalPaymentID)
		assert.Equal(t, data.PaymentTypeDirect, payment.Type)
		assert.Nil(t, payment.Disbursement)

		authMock.AssertExpectations(t)
		distResolverMock.AssertExpectations(t)
		distServiceMock.AssertExpectations(t)
		horizonClientMock.AssertExpectations(t)
	})

	t.Run("distribution account resolution fails", func(t *testing.T) {
		t.Cleanup(func() {
			data.DeleteAllFixtures(t, ctx, dbConnectionPool)
		})

		asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "ADAMANT", "GBXGQJWVLWOYHFLVTKWV5FGHA3LNYY2JQKM7OAJAUEQFU6LPCSEFVXON")
		wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "Fortress Monastery", "https://fortress.com", "fortress.com", "fortress://")
		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
			Email: "dante.invalid.asset@baal.imperium",
		})

		requestBody := fmt.Sprintf(`{
			"amount": "100.00",
			"asset": {"id": %q},
			"receiver": {"id": %q},
			"wallet": {"id": %q}
		}`, asset.ID, receiver.ID, wallet.ID)

		authMock := &auth.AuthManagerMock{}
		distResolverMock := sigMocks.NewMockDistributionAccountResolver(t)
		distServiceMock := &mocks.MockDistributionAccountService{}

		authMock.On("GetUserByID", mock.Anything, "user-id").Return(&auth.User{
			ID: "user-test",
		}, nil)

		distResolverMock.On("DistributionAccountFromContext", mock.Anything).Return(
			schema.TransactionAccount{}, errors.New("resolution failed"))

		directPaymentService := services.NewDirectPaymentService(models, distServiceMock, engine.SubmitterEngine{})

		handler := &PaymentsHandler{
			Models:                      models,
			DBConnectionPool:            dbConnectionPool,
			AuthManager:                 authMock,
			DistributionAccountResolver: distResolverMock,
			DirectPaymentService:        directPaymentService,
		}
		var req *http.Request
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, "/payments",
			strings.NewReader(requestBody))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		rr := httptest.NewRecorder()
		handler.PostDirectPayment(rr, req)

		assert.Equal(t, http.StatusInternalServerError, rr.Code)
		assert.JSONEq(t, `{"error": "resolving distribution account"}`, rr.Body.String())
	})

	t.Run("asset not found", func(t *testing.T) {
		t.Cleanup(func() {
			data.DeleteAllFixtures(t, ctx, dbConnectionPool)
		})
		wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "Asset Not Found Wallet", "https://fortress.com", "fortress.com", "fortress://")
		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
			Email: "dante.asset.notfound@baal.imperium",
		})

		requestBody := `{
			"amount": "100.00",
			"asset": {"id": "non-existent-asset"},
			"receiver": {"id": "` + receiver.ID + `"},
			"wallet": {"id": "` + wallet.ID + `"}
		}`

		authMock := &auth.AuthManagerMock{}
		distResolverMock := sigMocks.NewMockDistributionAccountResolver(t)
		distServiceMock := &mocks.MockDistributionAccountService{}

		authMock.On("GetUserByID", mock.Anything, "user-id").Return(&auth.User{
			ID: "user-test",
		}, nil)

		distResolverMock.On("DistributionAccountFromContext", mock.Anything).Return(
			schema.TransactionAccount{Type: schema.DistributionAccountStellarDBVault}, nil)

		directPaymentService := services.NewDirectPaymentService(models, distServiceMock, engine.SubmitterEngine{})

		handler := &PaymentsHandler{
			Models:                      models,
			DBConnectionPool:            dbConnectionPool,
			AuthManager:                 authMock,
			DistributionAccountResolver: distResolverMock,
			DirectPaymentService:        directPaymentService,
		}
		var req *http.Request
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, "/payments",
			strings.NewReader(requestBody))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		rr := httptest.NewRecorder()
		handler.PostDirectPayment(rr, req)

		assert.Equal(t, http.StatusNotFound, rr.Code)
		assert.JSONEq(t, `{"error": "asset not found with reference: non-existent-asset"}`, rr.Body.String())
	})

	t.Run("insufficient balance", func(t *testing.T) {
		t.Cleanup(func() {
			data.DeleteAllFixtures(t, ctx, dbConnectionPool)
		})

		asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "POWER", "GBXGQJWVLWOYHFLVTKWV5FGHA3LNYY2JQKM7OAJAUEQFU6LPCSEFVXON")
		wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "Insufficient Balance Wallet", "https://fortress.com", "fortress.com", "fortress://")

		_, err = dbConnectionPool.ExecContext(ctx,
			"INSERT INTO wallets_assets (wallet_id, asset_id) VALUES ($1, $2)",
			wallet.ID, asset.ID)
		require.NoError(t, err)

		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
			Email: "dante.insufficient.balance@baal.imperium",
		})
		data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.RegisteredReceiversWalletStatus)

		requestBody := fmt.Sprintf(`{
			"amount": "10000.00",
			"asset": {"id": %q},
			"receiver": {"id": %q},
			"wallet": {"id": %q}
		}`, asset.ID, receiver.ID, wallet.ID)

		authMock := &auth.AuthManagerMock{}
		distResolverMock := sigMocks.NewMockDistributionAccountResolver(t)
		distServiceMock := &mocks.MockDistributionAccountService{}
		horizonClientMock := &horizonclient.MockClient{}

		distributionAccPubKey := "GAAHIL6ZW4QFNLCKALZ3YOIWPP4TXQ7B7J5IU7RLNVGQAV6GFDZHLDTA"
		stellarDistAccount := schema.TransactionAccount{
			Type:    schema.DistributionAccountStellarDBVault,
			Address: distributionAccPubKey,
		}

		authMock.On("GetUserByID", mock.Anything, "user-id").Return(&auth.User{
			ID: "user-test",
		}, nil)

		distResolverMock.On("DistributionAccountFromContext", mock.Anything).Return(stellarDistAccount, nil)

		// Mock horizon client for trustline validation
		horizonClientMock.On("AccountDetail", horizonclient.AccountRequest{
			AccountID: distributionAccPubKey,
		}).Return(horizon.Account{
			AccountID: distributionAccPubKey,
			Sequence:  123,
			Balances: []horizon.Balance{
				{
					Balance: "10000000",
					Asset: base.Asset{
						Code:   asset.Code,
						Issuer: asset.Issuer,
					},
				},
			},
		}, nil).Once()

		distServiceMock.On("GetBalance", mock.Anything, &stellarDistAccount, *asset).Return(decimal.NewFromFloat(100), nil)

		directPaymentService := services.NewDirectPaymentService(
			models,
			distServiceMock,
			engine.SubmitterEngine{HorizonClient: horizonClientMock},
		)

		handler := &PaymentsHandler{
			Models:                      models,
			DBConnectionPool:            dbConnectionPool,
			AuthManager:                 authMock,
			DistributionAccountResolver: distResolverMock,
			DirectPaymentService:        directPaymentService,
		}
		var req *http.Request
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, "/payments",
			strings.NewReader(requestBody))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		rr := httptest.NewRecorder()
		handler.PostDirectPayment(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), "insufficient balance for direct payment")
		assert.Contains(t, rr.Body.String(), "10000.00")
		assert.Contains(t, rr.Body.String(), "100.000000 available")

		authMock.AssertExpectations(t)
		distResolverMock.AssertExpectations(t)
		distServiceMock.AssertExpectations(t)
		horizonClientMock.AssertExpectations(t)
	})

	t.Run("wallet not enabled", func(t *testing.T) {
		t.Cleanup(func() {
			data.DeleteAllFixtures(t, ctx, dbConnectionPool)
		})

		asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "STEEL", "GBXGQJWVLWOYHFLVTKWV5FGHA3LNYY2JQKM7OAJAUEQFU6LPCSEFVXON")
		wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "Fortress Monastery", "https://fortress.com", "fortress.com", "fortress://")
		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
			Email: "dante.wallet.disabled@baal.imperium",
		})

		_, err = dbConnectionPool.ExecContext(ctx,
			"UPDATE wallets SET enabled = false WHERE id = $1", wallet.ID)
		require.NoError(t, err)

		requestBody := fmt.Sprintf(`{
			"amount": "100.00",
			"asset": {"id": %q},
			"receiver": {"id": %q},
			"wallet": {"id": %q}
		}`, asset.ID, receiver.ID, wallet.ID)

		authMock := &auth.AuthManagerMock{}
		distResolverMock := sigMocks.NewMockDistributionAccountResolver(t)
		distServiceMock := &mocks.MockDistributionAccountService{}

		authMock.On("GetUserByID", mock.Anything, "user-id").Return(&auth.User{
			ID: "user-test",
		}, nil)

		distResolverMock.On("DistributionAccountFromContext", mock.Anything).Return(
			schema.TransactionAccount{Type: schema.DistributionAccountStellarDBVault}, nil)

		directPaymentService := services.NewDirectPaymentService(models, distServiceMock, engine.SubmitterEngine{})

		handler := &PaymentsHandler{
			Models:                      models,
			DBConnectionPool:            dbConnectionPool,
			AuthManager:                 authMock,
			DistributionAccountResolver: distResolverMock,
			DirectPaymentService:        directPaymentService,
		}
		var req *http.Request
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, "/payments",
			strings.NewReader(requestBody))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		rr := httptest.NewRecorder()
		handler.PostDirectPayment(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		errMsg := fmt.Sprintf(`{"error": "wallet '%s' is not enabled for payments"}`, wallet.Name)
		assert.JSONEq(t, errMsg, rr.Body.String())
	})

	t.Run("complex reference - receiver by email, asset by type", func(t *testing.T) {
		t.Cleanup(func() {
			data.DeleteAllFixtures(t, ctx, dbConnectionPool)
		})

		asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "PROMETHIUM", "GBXGQJWVLWOYHFLVTKWV5FGHA3LNYY2JQKM7OAJAUEQFU6LPCSEFVXON")
		wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "Complex Reference Wallet", "https://fortress.com", "fortress.com", "fortress://")

		_, err = dbConnectionPool.ExecContext(ctx,
			"INSERT INTO wallets_assets (wallet_id, asset_id) VALUES ($1, $2)",
			wallet.ID, asset.ID)
		require.NoError(t, err)

		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
			Email: "dante.complex.reference@baal.imperium",
		})
		data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.RegisteredReceiversWalletStatus)

		requestBody := `{
			"amount": "75.25",
			"asset": {
				"type": "classic",
				"code": "PROMETHIUM",
				"issuer": "GBXGQJWVLWOYHFLVTKWV5FGHA3LNYY2JQKM7OAJAUEQFU6LPCSEFVXON"
			},
			"receiver": {"email": "dante.complex.reference@baal.imperium"},
			"wallet": {"id": "` + wallet.ID + `"}
		}`

		authMock := &auth.AuthManagerMock{}
		distResolverMock := sigMocks.NewMockDistributionAccountResolver(t)
		distServiceMock := &mocks.MockDistributionAccountService{}
		horizonClientMock := &horizonclient.MockClient{}

		distributionAccPubKey := "GAAHIL6ZW4QFNLCKALZ3YOIWPP4TXQ7B7J5IU7RLNVGQAV6GFDZHLDTA"
		stellarDistAccount := schema.TransactionAccount{
			Type:    schema.DistributionAccountStellarDBVault,
			Address: distributionAccPubKey,
		}

		authMock.On("GetUserByID", mock.Anything, "user-id").Return(&auth.User{
			ID:    "user-test",
			Email: "test@imperium.gov",
		}, nil)

		distResolverMock.On("DistributionAccountFromContext", mock.Anything).Return(stellarDistAccount, nil)

		// Mock horizon client for trustline validation
		horizonClientMock.On("AccountDetail", horizonclient.AccountRequest{
			AccountID: distributionAccPubKey,
		}).Return(horizon.Account{
			AccountID: distributionAccPubKey,
			Sequence:  123,
			Balances: []horizon.Balance{
				{
					Balance: "10000000",
					Asset: base.Asset{
						Code:   asset.Code,
						Issuer: asset.Issuer,
					},
				},
			},
		}, nil).Once()

		distServiceMock.On("GetBalance", mock.Anything, &stellarDistAccount, *asset).Return(decimal.NewFromFloat(1000), nil)

		directPaymentService := services.NewDirectPaymentService(
			models,
			distServiceMock,
			engine.SubmitterEngine{HorizonClient: horizonClientMock},
		)

		handler := &PaymentsHandler{
			Models:                      models,
			DBConnectionPool:            dbConnectionPool,
			AuthManager:                 authMock,
			DistributionAccountResolver: distResolverMock,
			DirectPaymentService:        directPaymentService,
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/payments",
			strings.NewReader(requestBody))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		rr := httptest.NewRecorder()
		handler.PostDirectPayment(rr, req)

		assert.Equal(t, http.StatusCreated, rr.Code)

		var payment data.Payment
		err = json.Unmarshal(rr.Body.Bytes(), &payment)
		require.NoError(t, err)
		assert.Equal(t, "75.2500000", payment.Amount)
		assert.Equal(t, data.PaymentTypeDirect, payment.Type)
		assert.Equal(t, asset.ID, payment.Asset.ID)

		authMock.AssertExpectations(t)
		distResolverMock.AssertExpectations(t)
		distServiceMock.AssertExpectations(t)
		horizonClientMock.AssertExpectations(t)
	})

	t.Run("receiver not registered with wallet", func(t *testing.T) {
		t.Cleanup(func() {
			data.DeleteAllFixtures(t, ctx, dbConnectionPool)
		})

		asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "AURUM", "GBXGQJWVLWOYHFLVTKWV5FGHA3LNYY2JQKM7OAJAUEQFU6LPCSEFVXON")
		wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "Fortress Monastery", "https://fortress.com", "fortress.com", "fortress://")
		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
			Email: "dante.not.registered@baal.imperium",
		})

		requestBody := fmt.Sprintf(`{
			"amount": "100.00",
			"asset": {"id": %q},
			"receiver": {"id": %q},
			"wallet": {"id": %q}
		}`, asset.ID, receiver.ID, wallet.ID)

		authMock := &auth.AuthManagerMock{}
		distResolverMock := sigMocks.NewMockDistributionAccountResolver(t)
		distServiceMock := &mocks.MockDistributionAccountService{}

		authMock.On("GetUserByID", mock.Anything, "user-id").Return(&auth.User{
			ID: "user-test",
		}, nil)

		distResolverMock.On("DistributionAccountFromContext", mock.Anything).Return(
			schema.TransactionAccount{Type: schema.DistributionAccountStellarDBVault}, nil)

		directPaymentService := services.NewDirectPaymentService(models, distServiceMock, engine.SubmitterEngine{})

		handler := &PaymentsHandler{
			Models:                      models,
			DBConnectionPool:            dbConnectionPool,
			AuthManager:                 authMock,
			DistributionAccountResolver: distResolverMock,
			DirectPaymentService:        directPaymentService,
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/payments",
			strings.NewReader(requestBody))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		rr := httptest.NewRecorder()
		handler.PostDirectPayment(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		errMsg := fmt.Sprintf(`{"error":"asset '%s' is not supported by wallet '%s'"}`, asset.Code, wallet.Name)
		assert.JSONEq(t, errMsg, rr.Body.String())
	})
}

func TestPaymentsHandler_PostPayment_InputValidation(t *testing.T) {
	dbConnectionPool := testutils.GetDBConnectionPool(t)
	ctx := sdpcontext.SetUserIDInContext(context.Background(), "user-horus")
	ctx = sdpcontext.SetTenantInContext(ctx, &schema.Tenant{ID: "battle-barge-001"})

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	t.Cleanup(func() {
		data.DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
	})

	asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "TESTCOIN", "GBXGQJWVLWOYHFLVTKWV5FGHA3LNYY2JQKM7OAJAUEQFU6LPCSEFVXON")
	wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "Test Wallet", "https://test.com", "test.com", "test://")
	receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
		Email: "validation.test@imperium.gov",
	})

	authMock := &auth.AuthManagerMock{}
	distResolverMock := sigMocks.NewMockDistributionAccountResolver(t)
	distServiceMock := &mocks.MockDistributionAccountService{}

	authMock.On("GetUserByID", mock.Anything, "user-horus").Return(&auth.User{
		ID: "user-horus", Email: "horus@warmaster.imperium",
	}, nil)

	distResolverMock.On("DistributionAccountFromContext", mock.Anything).Return(
		schema.TransactionAccount{}, nil)

	directPaymentService := services.NewDirectPaymentService(models, distServiceMock, engine.SubmitterEngine{})

	handler := &PaymentsHandler{
		Models:                      models,
		DBConnectionPool:            dbConnectionPool,
		AuthManager:                 authMock,
		DistributionAccountResolver: distResolverMock,
		DirectPaymentService:        directPaymentService,
	}

	testCases := []struct {
		name           string
		requestBody    string
		expectedStatus int
		expectedError  string
	}{
		{
			name: "invalid amount format",
			requestBody: fmt.Sprintf(`{
				"amount": "not-a-number",
				"asset": {"id": %q},
				"receiver": {"id": %q},
				"wallet": {"id": %q}
			}`, asset.ID, receiver.ID, wallet.ID),
			expectedStatus: http.StatusBadRequest,
			expectedError:  `{"error": "request invalid", "extras":{"amount":"the provided amount is not a valid number"}}`,
		},
		{
			name:           "invalid JSON body",
			requestBody:    `{"amount": "100.00", "invalid json`,
			expectedStatus: http.StatusBadRequest,
			expectedError:  `{"error": "invalid request body"}`,
		},
		{
			name: "empty asset reference",
			requestBody: fmt.Sprintf(`{
				"amount": "100.00",
				"asset": {},
				"receiver": {"id": %q},
				"wallet": {"id": %q}
			}`, receiver.ID, wallet.ID),
			expectedStatus: http.StatusBadRequest,
			expectedError:  `{"error":"request invalid", "extras":{"asset":"asset reference is required - must specify either id or type"}}`,
		},
		{
			name: "unsupported contract asset",
			requestBody: fmt.Sprintf(`{
				"amount": "100.00",
				"asset": {
					"type": "contract",
					"contract_id": "CONTRACT123"
				},
				"receiver": {"id": %q},
				"wallet": {"id": %q}
			}`, receiver.ID, wallet.ID),
			expectedStatus: http.StatusBadRequest,
			expectedError:  `{"error": "validation error for asset.contract_id: invalid contract format provided"}`,
		},
		{
			name: "unsupported fiat asset",
			requestBody: fmt.Sprintf(`{
				"amount": "100.00",
				"asset": {
					"type": "fiat",
					"code": "USD"
				},
				"receiver": {"id": %q},
				"wallet": {"id": %q}
			}`, receiver.ID, wallet.ID),
			expectedStatus: http.StatusBadRequest,
			expectedError:  `{"error": "asset: fiat assets not yet supported"}`,
		},
		{
			name: "classic asset missing code",
			requestBody: fmt.Sprintf(`{
				"amount": "100.00",
				"asset": {
					"type": "classic",
					"issuer": "GBXGQJWVLWOYHFLVTKWV5FGHA3LNYY2JQKM7OAJAUEQFU6LPCSEFVXON"
				},
				"receiver": {"id": %q},
				"wallet": {"id": %q}
			}`, receiver.ID, wallet.ID),
			expectedStatus: http.StatusBadRequest,
			expectedError:  `{"error":"request invalid", "extras":{"asset.code":"code is required for classic asset"}}`,
		},
		{
			name: "classic asset missing issuer",
			requestBody: fmt.Sprintf(`{
				"amount": "100.00",
				"asset": {
					"type": "classic",
					"code": "TESTCOIN"
				},
				"receiver": {"id": %q},
				"wallet": {"id": %q}
			}`, receiver.ID, wallet.ID),
			expectedStatus: http.StatusBadRequest,
			expectedError:  `{"error":"request invalid", "extras":{"asset.issuer":"issuer is required for classic asset"}}`,
		},
		{
			name: "negative amount",
			requestBody: fmt.Sprintf(`{
				"amount": "-100.00",
				"asset": {"id": %q},
				"receiver": {"id": %q},
				"wallet": {"id": %q}
			}`, asset.ID, receiver.ID, wallet.ID),
			expectedStatus: http.StatusBadRequest,
			expectedError:  `{"error": "request invalid", "extras":{"amount":"the provided amount must be greater than zero"}}`,
		},
		{
			name: "zero amount",
			requestBody: fmt.Sprintf(`{
				"amount": "0",
				"asset": {"id": %q},
				"receiver": {"id": %q},
				"wallet": {"id": %q}
			}`, asset.ID, receiver.ID, wallet.ID),
			expectedStatus: http.StatusBadRequest,
			expectedError:  `{"error": "request invalid", "extras":{"amount":"the provided amount must be greater than zero"}}`,
		},
		{
			name: "missing receiver reference",
			requestBody: fmt.Sprintf(`{
				"amount": "100.00",
				"asset": {"id": %q},
				"receiver": {},
				"wallet": {"id": %q}
			}`, asset.ID, wallet.ID),
			expectedStatus: http.StatusBadRequest,
			expectedError:  `{"error":"request invalid", "extras":{"receiver":"receiver reference must specify exactly one identifier"}}`,
		},
		{
			name: "missing wallet reference",
			requestBody: fmt.Sprintf(`{
				"amount": "100.00",
				"asset": {"id": %q},
				"receiver": {"id": %q},
				"wallet": {}
			}`, asset.ID, receiver.ID),
			expectedStatus: http.StatusBadRequest,
			expectedError:  `{"error":"request invalid", "extras":{"wallet":"wallet reference is required - must specify either id or address"}}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/payments",
				strings.NewReader(tc.requestBody))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			handler.PostDirectPayment(rr, req)

			assert.Equal(t, tc.expectedStatus, rr.Code)
			assert.JSONEq(t, tc.expectedError, rr.Body.String())
		})
	}
}
