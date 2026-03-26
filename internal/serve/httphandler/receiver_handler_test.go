package httphandler

import (
	"bytes"
	"context"
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

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/dto"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/testutils"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

func Test_ReceiverHandlerGet(t *testing.T) {
	dbConnectionPool := testutils.GetDBConnectionPool(t)

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	handler := &ReceiverHandler{
		Models:           models,
		DBConnectionPool: dbConnectionPool,
	}

	// setup
	r := chi.NewRouter()
	r.Get("/receivers/{id}", handler.GetReceiver)

	ctx := context.Background()

	asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
	wallet1 := data.CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet1.com", "www.wallet1.com", "wallet1://")

	receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})

	disbursement := data.Disbursement{
		Status: data.DraftDisbursementStatus,
		Asset:  asset,
	}

	stellarTransactionID, err := utils.RandomString(64)
	require.NoError(t, err)
	stellarOperationID, err := utils.RandomString(32)
	require.NoError(t, err)

	payment := data.Payment{
		Amount:               "50",
		StellarTransactionID: stellarTransactionID,
		StellarOperationID:   stellarOperationID,
		Asset:                *asset,
	}

	t.Run("successfully returns receiver details with receiver without wallet", func(t *testing.T) {
		// test
		route := fmt.Sprintf("/receivers/%s", receiver.ID)
		req, err := http.NewRequest("GET", route, nil)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		// assert response
		assert.Equal(t, http.StatusOK, rr.Code)

		wantJSON := fmt.Sprintf(`{
			"id": %q,
			"external_id": %q,
			"email": %q,
			"phone_number": %q,
			"created_at": %q,
			"updated_at": %q,
			"total_payments": "0",
			"successful_payments": "0",
			"failed_payments": "0",
			"canceled_payments": "0",
    		"remaining_payments": "0",
			"registered_wallets": "0",
			"wallets": []
		}`, receiver.ID, receiver.ExternalID, receiver.Email, receiver.PhoneNumber, receiver.CreatedAt.Format(time.RFC3339Nano), receiver.UpdatedAt.Format(time.RFC3339Nano))

		assert.JSONEq(t, wantJSON, rr.Body.String())
	})

	receiverWallet1 := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet1.ID, data.DraftReceiversWalletStatus)

	message1 := data.CreateMessageFixture(t, ctx, dbConnectionPool, &data.Message{
		Type:             message.MessengerTypeTwilioSMS,
		AssetID:          nil,
		ReceiverID:       receiver.ID,
		WalletID:         wallet1.ID,
		ReceiverWalletID: &receiverWallet1.ID,
		Status:           data.SuccessMessageStatus,
		CreatedAt:        time.Date(2023, 1, 10, 23, 40, 20, 1000, time.UTC),
	})

	message2 := data.CreateMessageFixture(t, ctx, dbConnectionPool, &data.Message{
		Type:             message.MessengerTypeTwilioSMS,
		AssetID:          nil,
		ReceiverID:       receiver.ID,
		WalletID:         wallet1.ID,
		ReceiverWalletID: &receiverWallet1.ID,
		Status:           data.SuccessMessageStatus,
		CreatedAt:        time.Date(2023, 2, 10, 23, 40, 20, 1000, time.UTC),
	})

	t.Run("successfully returns receiver details with one wallet for given ID", func(t *testing.T) {
		disbursement.Name = "disbursement 1"
		disbursement.Wallet = wallet1
		d := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &disbursement)

		payment.Status = data.SuccessPaymentStatus
		payment.Disbursement = d
		payment.ReceiverWallet = receiverWallet1
		data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &payment)

		// test
		route := fmt.Sprintf("/receivers/%s", receiver.ID)
		req, err := http.NewRequest("GET", route, nil)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		// assert response
		assert.Equal(t, http.StatusOK, rr.Code)

		wantJSON := fmt.Sprintf(`{
			"id": %q,
			"external_id": %q,
			"email": %q,
			"phone_number": %q,
			"created_at": %q,
			"updated_at": %q,
			"total_payments": "1",
			"successful_payments": "1",
			"failed_payments": "0",
			"canceled_payments": "0",
			"remaining_payments": "0",
            "registered_wallets": "0",
			"received_amounts":	[
				{
					"asset_code": "USDC",
					"asset_issuer": "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV",
					"received_amount": "50.0000000"
				}
			],
			"wallets": [
				{
					"id": %q,
					"receiver": {
						"id": %q
					},
					"wallet": {
						"id": %q,
						"name": "wallet1",
						"homepage": "https://www.wallet1.com",
						"sep_10_client_domain": "www.wallet1.com",
						"enabled": true
					},
					"status": "DRAFT",
					"created_at": %q,
					"updated_at": %q,
					"invitation_sent_at": null,
					"invited_at": %q,
					"last_message_sent_at": %q,
					"total_payments": "1",
					"payments_received": "1",
					"failed_payments": "0",
					"canceled_payments": "0",
					"remaining_payments": "0",
					"received_amounts":  [
						{
							"asset_code": "USDC",
							"asset_issuer": "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV",
							"received_amount": "50.0000000"
						}
					]
				}
			]
		}`, receiver.ID, receiver.ExternalID, receiver.Email, receiver.PhoneNumber, receiver.CreatedAt.Format(time.RFC3339Nano),
			receiver.UpdatedAt.Format(time.RFC3339Nano), receiverWallet1.ID, receiverWallet1.Receiver.ID, receiverWallet1.Wallet.ID,
			receiverWallet1.CreatedAt.Format(time.RFC3339Nano), receiverWallet1.UpdatedAt.Format(time.RFC3339Nano),
			message1.CreatedAt.Format(time.RFC3339Nano), message2.CreatedAt.Format(time.RFC3339Nano))

		assert.JSONEq(t, wantJSON, rr.Body.String())
	})

	t.Run("successfully returns receiver details with multiple wallets for given ID", func(t *testing.T) {
		wallet2 := data.CreateWalletFixture(t, ctx, dbConnectionPool, "wallet2", "https://www.wallet2.com", "www.wallet2.com", "wallet2://")
		receiverWallet2 := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet2.ID, data.RegisteredReceiversWalletStatus)

		message3 := data.CreateMessageFixture(t, ctx, dbConnectionPool, &data.Message{
			Type:             message.MessengerTypeTwilioSMS,
			AssetID:          nil,
			ReceiverID:       receiver.ID,
			WalletID:         wallet2.ID,
			ReceiverWalletID: &receiverWallet2.ID,
			Status:           data.SuccessMessageStatus,
			CreatedAt:        time.Date(2023, 1, 10, 23, 40, 20, 1000, time.UTC),
		})

		message4 := data.CreateMessageFixture(t, ctx, dbConnectionPool, &data.Message{
			Type:             message.MessengerTypeTwilioSMS,
			AssetID:          nil,
			ReceiverID:       receiver.ID,
			WalletID:         wallet2.ID,
			ReceiverWalletID: &receiverWallet2.ID,
			Status:           data.SuccessMessageStatus,
			CreatedAt:        time.Date(2023, 2, 10, 23, 40, 20, 1000, time.UTC),
		})

		disbursement.Name = "disbursement 2"
		disbursement.Wallet = wallet2
		d := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &disbursement)

		payment.Status = data.DraftPaymentStatus
		payment.Disbursement = d
		payment.ReceiverWallet = receiverWallet2
		data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &payment)

		// test
		route := fmt.Sprintf("/receivers/%s", receiver.ID)
		req, err := http.NewRequest("GET", route, nil)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		// assert response
		assert.Equal(t, http.StatusOK, rr.Code)

		wantJSON := fmt.Sprintf(`{
			"id": %q,
			"external_id": %q,
			"email": %q,
			"phone_number": %q,
			"created_at": %q,
			"updated_at": %q,
			"total_payments": "2",
			"successful_payments": "1",
			"failed_payments": "0",
			"canceled_payments": "0",
			"remaining_payments": "1",
			"registered_wallets": "1",
			"received_amounts":  [
				{
					"asset_code": "USDC",
					"asset_issuer": "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV",
					"received_amount": "50.0000000"
				}
			],
			"wallets": [
				{
					"id": %q,
					"receiver": {
						"id": %q
					},
					"wallet": {
						"id": %q,
						"name": "wallet1",
						"homepage": "https://www.wallet1.com",
						"sep_10_client_domain": "www.wallet1.com",
						"enabled": true
					},
					"status": "DRAFT",
					"created_at": %q,
					"updated_at": %q,
					"invitation_sent_at": null,
					"invited_at": %q,
					"last_message_sent_at": %q,
					"total_payments": "1",
					"payments_received": "1",
					"failed_payments": "0",
					"canceled_payments": "0",
					"remaining_payments": "0",
					"received_amounts":  [
						{
							"asset_code": "USDC",
							"asset_issuer": "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV",
							"received_amount": "50.0000000"
						}
					]
				},
				{
					"id": %q,
					"receiver": {
						"id": %q
					},
					"wallet": {
						"id": %q,
						"name": "wallet2",
						"homepage": "https://www.wallet2.com",
						"sep_10_client_domain": "www.wallet2.com",
						"enabled": true
					},
					"stellar_address": %q,
					"stellar_memo": %q,
					"stellar_memo_type": %q,
					"status": "REGISTERED",
					"created_at": %q,
					"updated_at": %q,
					"invitation_sent_at": null,
					"invited_at": %q,
					"last_message_sent_at": %q,
					"total_payments": "1",
					"payments_received": "0",
					"failed_payments": "0",
					"canceled_payments": "0",
					"remaining_payments": "1",
					"received_amounts":  [
						{
							"asset_code": "USDC",
							"asset_issuer": "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV",
							"received_amount": "0"
						}
					],
					"sep24_transaction_id": %q
				}
			]
		}`, receiver.ID, receiver.ExternalID, receiver.Email, receiver.PhoneNumber, receiver.CreatedAt.Format(time.RFC3339Nano),
			receiver.UpdatedAt.Format(time.RFC3339Nano), receiverWallet1.ID, receiverWallet1.Receiver.ID,
			receiverWallet1.Wallet.ID, receiverWallet1.CreatedAt.Format(time.RFC3339Nano), receiverWallet1.UpdatedAt.Format(time.RFC3339Nano),
			message1.CreatedAt.Format(time.RFC3339Nano), message2.CreatedAt.Format(time.RFC3339Nano),
			receiverWallet2.ID, receiverWallet2.Receiver.ID, receiverWallet2.Wallet.ID,
			receiverWallet2.StellarAddress, receiverWallet2.StellarMemo, receiverWallet2.StellarMemoType,
			receiverWallet2.CreatedAt.Format(time.RFC3339Nano), receiverWallet2.UpdatedAt.Format(time.RFC3339Nano),
			message3.CreatedAt.Format(time.RFC3339Nano), message4.CreatedAt.Format(time.RFC3339Nano), receiverWallet2.SEP24TransactionID)

		assert.JSONEq(t, wantJSON, rr.Body.String())
	})

	t.Run("error receiver not found for given ID", func(t *testing.T) {
		// test
		req, err := http.NewRequest("GET", "/receivers/invalid_id", nil)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		// assert response
		assert.Equal(t, http.StatusNotFound, rr.Code)

		wantJSON := `{
			"error": "could not retrieve receiver with ID: invalid_id"
		}`
		assert.JSONEq(t, wantJSON, rr.Body.String())
	})
}

func Test_ReceiverHandler_GetReceivers_Errors(t *testing.T) {
	dbConnectionPool := testutils.GetDBConnectionPool(t)

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	handler := &ReceiverHandler{
		Models:           models,
		DBConnectionPool: dbConnectionPool,
	}

	ts := httptest.NewServer(http.HandlerFunc(handler.GetReceivers))
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
			expectedResponse:   `{"error":"request invalid", "extras":{"status":"invalid parameter. valid values are: draft, ready, registered, flagged"}}`,
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
			expectedResponse:   `{"data":[], "pagination":{"pages":0, "total": 0}}`,
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

func Test_ReceiverHandler_GetReceivers_Success(t *testing.T) {
	dbConnectionPool := testutils.GetDBConnectionPool(t)

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)
	handler := &ReceiverHandler{
		Models:           models,
		DBConnectionPool: dbConnectionPool,
	}

	ts := httptest.NewServer(http.HandlerFunc(handler.GetReceivers))
	defer ts.Close()

	ctx := context.Background()

	// create fixtures
	asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
	wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")

	// create receivers
	date := time.Date(2022, 12, 10, 23, 40, 20, 1431, time.UTC)
	receiver1Email := "receiver1@mock.com"
	receiver1 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
		Email:       receiver1Email,
		ExternalID:  "external_id_1",
		PhoneNumber: "+99991111",
		CreatedAt:   &date,
		UpdatedAt:   &date,
	})

	date = time.Date(2023, 1, 10, 23, 40, 20, 1431, time.UTC)
	receiver2Email := "receiver2@mock.com"
	receiver2 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
		Email:       receiver2Email,
		ExternalID:  "external_id_2",
		PhoneNumber: "+99992222",
		CreatedAt:   &date,
		UpdatedAt:   &date,
	})
	receiverWallet2 := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver2.ID, wallet.ID, data.RegisteredReceiversWalletStatus)

	message1 := data.CreateMessageFixture(t, ctx, dbConnectionPool, &data.Message{
		Type:             message.MessengerTypeTwilioSMS,
		AssetID:          nil,
		ReceiverID:       receiver2.ID,
		WalletID:         wallet.ID,
		ReceiverWalletID: &receiverWallet2.ID,
		Status:           data.SuccessMessageStatus,
		CreatedAt:        time.Date(2023, 1, 10, 23, 40, 20, 1000, time.UTC),
	})

	message2 := data.CreateMessageFixture(t, ctx, dbConnectionPool, &data.Message{
		Type:             message.MessengerTypeTwilioSMS,
		AssetID:          nil,
		ReceiverID:       receiver2.ID,
		WalletID:         wallet.ID,
		ReceiverWalletID: &receiverWallet2.ID,
		Status:           data.SuccessMessageStatus,
		CreatedAt:        time.Date(2023, 2, 10, 23, 40, 20, 1000, time.UTC),
	})

	date = time.Date(2023, 2, 10, 23, 40, 21, 1431, time.UTC)
	receiver3Email := "receiver3@mock.com"
	receiver3 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
		Email:       receiver3Email,
		ExternalID:  "external_id_3",
		PhoneNumber: "+99993333",
		CreatedAt:   &date,
		UpdatedAt:   &date,
	})
	receiverWallet3 := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver3.ID, wallet.ID, data.RegisteredReceiversWalletStatus)

	message3 := data.CreateMessageFixture(t, ctx, dbConnectionPool, &data.Message{
		Type:             message.MessengerTypeTwilioSMS,
		AssetID:          nil,
		ReceiverID:       receiver3.ID,
		WalletID:         wallet.ID,
		ReceiverWalletID: &receiverWallet3.ID,
		Status:           data.SuccessMessageStatus,
		CreatedAt:        time.Date(2023, 1, 10, 23, 40, 20, 1000, time.UTC),
	})

	message4 := data.CreateMessageFixture(t, ctx, dbConnectionPool, &data.Message{
		Type:             message.MessengerTypeTwilioSMS,
		AssetID:          nil,
		ReceiverID:       receiver3.ID,
		WalletID:         wallet.ID,
		ReceiverWalletID: &receiverWallet3.ID,
		Status:           data.SuccessMessageStatus,
		CreatedAt:        time.Date(2023, 2, 10, 23, 40, 20, 1000, time.UTC),
	})

	date = time.Date(2023, 3, 10, 23, 40, 20, 1431, time.UTC)
	receiver4 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
		Email:       "receiver4@mock.com",
		ExternalID:  "external_id_4",
		PhoneNumber: "+99994444",
		CreatedAt:   &date,
		UpdatedAt:   &date,
	})
	receiverWallet4 := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver4.ID, wallet.ID, data.DraftReceiversWalletStatus)

	message5 := data.CreateMessageFixture(t, ctx, dbConnectionPool, &data.Message{
		Type:             message.MessengerTypeTwilioSMS,
		AssetID:          nil,
		ReceiverID:       receiver4.ID,
		WalletID:         wallet.ID,
		ReceiverWalletID: &receiverWallet4.ID,
		Status:           data.SuccessMessageStatus,
		CreatedAt:        time.Date(2023, 1, 10, 23, 40, 20, 1000, time.UTC),
	})

	message6 := data.CreateMessageFixture(t, ctx, dbConnectionPool, &data.Message{
		Type:             message.MessengerTypeTwilioSMS,
		AssetID:          nil,
		ReceiverID:       receiver4.ID,
		WalletID:         wallet.ID,
		ReceiverWalletID: &receiverWallet4.ID,
		Status:           data.SuccessMessageStatus,
		CreatedAt:        time.Date(2023, 2, 10, 23, 40, 20, 1000, time.UTC),
	})

	// create disbursements
	disbursement1 := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Name:   "disbursement 1",
		Status: data.DraftDisbursementStatus,
		Asset:  asset,
		Wallet: wallet,
	})

	stellarTransactionID, err := utils.RandomString(64)
	require.NoError(t, err)
	stellarOperationID, err := utils.RandomString(32)
	require.NoError(t, err)

	// create payments
	data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		Amount:               "50",
		StellarTransactionID: stellarTransactionID,
		StellarOperationID:   stellarOperationID,
		Status:               data.SuccessPaymentStatus,
		Disbursement:         disbursement1,
		Asset:                *asset,
		ReceiverWallet:       receiverWallet2,
	})

	stellarTransactionID, err = utils.RandomString(64)
	require.NoError(t, err)
	stellarOperationID, err = utils.RandomString(32)
	require.NoError(t, err)

	data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		Amount:               "50",
		StellarTransactionID: stellarTransactionID,
		StellarOperationID:   stellarOperationID,
		Status:               data.DraftPaymentStatus,
		Disbursement:         disbursement1,
		Asset:                *asset,
		ReceiverWallet:       receiverWallet3,
	})

	tests := []struct {
		name               string
		queryParams        map[string]string
		expectedStatusCode int
		expectedResponse   string
	}{
		{
			name:               "fetch all receivers without filters",
			queryParams:        map[string]string{},
			expectedStatusCode: http.StatusOK,
			expectedResponse: fmt.Sprintf(`{
				"pagination": {
					"pages": 1,
					"total": 4
				},
				"data": [
					{
						"id": %q,
						"email": "receiver4@mock.com",
						"external_id": "external_id_4",
						"phone_number": "+99994444",
						"created_at": %q,
						"updated_at": %q,
						"total_payments": "0",
						"successful_payments": "0",
						"failed_payments": "0",
						"canceled_payments": "0",
						"remaining_payments": "0",
						"registered_wallets":"0",
						"wallets": [
							{
								"id": %q,
								"receiver": {
									"id": %q
								},
								"wallet": {
									"id": %q,
									"name": "wallet1",
									"homepage": "https://www.wallet.com",
									"sep_10_client_domain": "www.wallet.com",
									"enabled": true
								},
								"status": "DRAFT",
								"created_at": %q,
								"updated_at": %q,
								"invitation_sent_at": null,
								"invited_at": %q,
								"last_message_sent_at": %q,
								"total_payments": "0",
								"payments_received": "0",
								"failed_payments": "0",
								"canceled_payments": "0",
								"remaining_payments": "0"
							}
						]
					},
					{
						"id": %q,
						"email": "receiver3@mock.com",
						"external_id": "external_id_3",
						"phone_number": "+99993333",
						"created_at": %q,
						"updated_at": %q,
						"total_payments": "1",
						"successful_payments": "0",
						"failed_payments": "0",
						"canceled_payments": "0",
						"remaining_payments": "1",
						"received_amounts":  [
							{
								"asset_code": "USDC",
								"asset_issuer": "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV",
								"received_amount": "0"
							}
						],
						"registered_wallets":"1",
						"wallets": [
							{
								"id": %q,
								"receiver": {
									"id": %q
								},
								"wallet": {
									"id": %q,
									"name": "wallet1",
									"homepage": "https://www.wallet.com",
									"sep_10_client_domain": "www.wallet.com",
									"enabled": true
								},
								"stellar_address": %q,
								"stellar_memo": %q,
								"stellar_memo_type": %q,
								"status": "REGISTERED",
								"created_at": %q,
								"updated_at": %q,
								"invitation_sent_at": null,
								"invited_at": %q,
								"last_message_sent_at": %q,
								"total_payments": "1",
								"payments_received": "0",
								"failed_payments": "0",
								"canceled_payments": "0",
								"remaining_payments": "1",
								"received_amounts":  [
									{
										"asset_code": "USDC",
										"asset_issuer": "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV",
										"received_amount": "0"
									}
								],
								"sep24_transaction_id": %q
							}
						]
					},
					{
						"id": %q,
						"email": "receiver2@mock.com",
						"external_id": "external_id_2",
						"phone_number": "+99992222",
						"created_at": %q,
						"updated_at": %q,
						"total_payments": "1",
						"successful_payments": "1",
						"failed_payments": "0",
						"canceled_payments": "0",
						"remaining_payments": "0",
						"received_amounts":  [
							{
								"asset_code": "USDC",
								"asset_issuer": "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV",
								"received_amount": "50.0000000"
							}
						],
						"registered_wallets":"1",
						"wallets": [
							{
								"id": %q,
								"receiver": {
									"id": %q
								},
								"wallet": {
									"id": %q,
									"name": "wallet1",
									"homepage": "https://www.wallet.com",
									"sep_10_client_domain": "www.wallet.com",
									"enabled": true
								},
								"stellar_address": %q,
								"stellar_memo": %q,
								"stellar_memo_type": %q,
								"status": "REGISTERED",
								"created_at": %q,
								"updated_at": %q,
								"invitation_sent_at": null,
								"invited_at": %q,
								"last_message_sent_at": %q,
								"total_payments": "1",
								"payments_received": "1",
								"failed_payments": "0",
								"canceled_payments": "0",
								"remaining_payments": "0",
								"received_amounts":  [
									{
										"asset_code": "USDC",
										"asset_issuer": "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV",
										"received_amount": "50.0000000"
									}
								],
								"sep24_transaction_id": %q
							}
						]
					},
					{
						"id": %q,
						"email": "receiver1@mock.com",
						"external_id": "external_id_1",
						"phone_number": "+99991111",
						"created_at": %q,
						"updated_at": %q,
						"total_payments": "0",
						"successful_payments": "0",
						"failed_payments": "0",
						"canceled_payments": "0",
						"remaining_payments": "0",
						"registered_wallets":"0",
						"wallets": []
					}
				]
			}`,
				receiver4.ID, receiver4.CreatedAt.Format(time.RFC3339Nano), receiver4.UpdatedAt.Format(time.RFC3339Nano),
				receiverWallet4.ID, receiverWallet4.Receiver.ID, receiverWallet4.Wallet.ID,
				receiverWallet4.CreatedAt.Format(time.RFC3339Nano), receiverWallet4.UpdatedAt.Format(time.RFC3339Nano),
				message5.CreatedAt.Format(time.RFC3339Nano), message6.CreatedAt.Format(time.RFC3339Nano),
				receiver3.ID, receiver3.CreatedAt.Format(time.RFC3339Nano), receiver3.UpdatedAt.Format(time.RFC3339Nano),
				receiverWallet3.ID, receiverWallet3.Receiver.ID, receiverWallet3.Wallet.ID,
				receiverWallet3.StellarAddress, receiverWallet3.StellarMemo, receiverWallet3.StellarMemoType,
				receiverWallet3.CreatedAt.Format(time.RFC3339Nano), receiverWallet3.UpdatedAt.Format(time.RFC3339Nano),
				message3.CreatedAt.Format(time.RFC3339Nano), message4.CreatedAt.Format(time.RFC3339Nano), receiverWallet3.SEP24TransactionID,
				receiver2.ID, receiver2.CreatedAt.Format(time.RFC3339Nano), receiver2.UpdatedAt.Format(time.RFC3339Nano),
				receiverWallet2.ID, receiverWallet2.Receiver.ID, receiverWallet2.Wallet.ID, receiverWallet2.StellarAddress,
				receiverWallet2.StellarMemo, receiverWallet2.StellarMemoType,
				receiverWallet2.CreatedAt.Format(time.RFC3339Nano), receiverWallet2.UpdatedAt.Format(time.RFC3339Nano),
				message1.CreatedAt.Format(time.RFC3339Nano), message2.CreatedAt.Format(time.RFC3339Nano), receiverWallet2.SEP24TransactionID,
				receiver1.ID, receiver1.CreatedAt.Format(time.RFC3339Nano), receiver1.UpdatedAt.Format(time.RFC3339Nano)),
		},
		{
			name: "fetch first page of receivers with limit 1 and sort by created_at",
			queryParams: map[string]string{
				"page":       "1",
				"page_limit": "1",
				"sort":       "created_at",
				"direction":  "asc",
			},
			expectedStatusCode: http.StatusOK,
			expectedResponse: fmt.Sprintf(`{
				"pagination": {
					"next": "/receivers?direction=asc\u0026page=2\u0026page_limit=1\u0026sort=created_at",
					"pages": 4,
					"total": 4
				},
				"data": [
					{
						"id": %q,
						"email": "receiver1@mock.com",
						"external_id": "external_id_1",
						"phone_number": "+99991111",
						"created_at": %q,
						"updated_at": %q,
						"total_payments": "0",
						"successful_payments": "0",
						"failed_payments": "0",
						"canceled_payments": "0",
						"remaining_payments": "0",
						"registered_wallets":"0",
						"wallets": []
					}
				]
			}`, receiver1.ID, receiver1.CreatedAt.Format(time.RFC3339Nano), receiver1.UpdatedAt.Format(time.RFC3339Nano)),
		},
		{
			name: "fetch second page of receivers with limit 1 and sort by created_at",
			queryParams: map[string]string{
				"page":       "2",
				"page_limit": "1",
				"sort":       "created_at",
				"direction":  "asc",
			},
			expectedStatusCode: http.StatusOK,
			expectedResponse: fmt.Sprintf(`{
				"pagination": {
					"prev": "/receivers?direction=asc\u0026page=1\u0026page_limit=1\u0026sort=created_at",
					"next": "/receivers?direction=asc\u0026page=3\u0026page_limit=1\u0026sort=created_at",
					"pages": 4,
					"total": 4
				},
				"data": [
					{
						"id": %q,
						"email": "receiver2@mock.com",
						"external_id": "external_id_2",
						"phone_number": "+99992222",
						"created_at": %q,
						"updated_at": %q,
						"total_payments": "1",
						"successful_payments": "1",
						"failed_payments": "0",
						"canceled_payments": "0",
						"remaining_payments": "0",
						"received_amounts":  [
							{
								"asset_code": "USDC",
								"asset_issuer": "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV",
								"received_amount": "50.0000000"
							}
						],
						"registered_wallets":"1",
						"wallets": [
							{
								"id": %q,
								"receiver": {
									"id": %q
								},
								"wallet": {
									"id": %q,
									"name": "wallet1",
									"homepage": "https://www.wallet.com",
									"sep_10_client_domain": "www.wallet.com",
									"enabled": true
								},
								"stellar_address": %q,
								"stellar_memo": %q,
								"stellar_memo_type": %q,
								"status": "REGISTERED",
								"created_at": %q,
								"updated_at": %q,
								"invitation_sent_at": null,
								"invited_at": %q,
								"last_message_sent_at": %q,
								"total_payments": "1",
								"payments_received": "1",
								"failed_payments": "0",
								"canceled_payments": "0",
								"remaining_payments": "0",
								"received_amounts":  [
									{
										"asset_code": "USDC",
										"asset_issuer": "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV",
										"received_amount": "50.0000000"
									}
								],
								"sep24_transaction_id": %q
							}
						]
					}
				]
			}`, receiver2.ID, receiver2.CreatedAt.Format(time.RFC3339Nano), receiver2.UpdatedAt.Format(time.RFC3339Nano),
				receiverWallet2.ID, receiverWallet2.Receiver.ID, receiverWallet2.Wallet.ID,
				receiverWallet2.StellarAddress, receiverWallet2.StellarMemo, receiverWallet2.StellarMemoType,
				receiverWallet2.CreatedAt.Format(time.RFC3339Nano), receiverWallet2.UpdatedAt.Format(time.RFC3339Nano),
				message1.CreatedAt.Format(time.RFC3339Nano), message2.CreatedAt.Format(time.RFC3339Nano), receiverWallet2.SEP24TransactionID),
		},
		{
			name: "fetch last page of receivers with limit 1 and sort by created_at",
			queryParams: map[string]string{
				"page":       "4",
				"page_limit": "1",
				"sort":       "created_at",
				"direction":  "asc",
			},
			expectedStatusCode: http.StatusOK,
			expectedResponse: fmt.Sprintf(`{
				"pagination": {
					"prev": "/receivers?direction=asc\u0026page=3\u0026page_limit=1\u0026sort=created_at",
					"pages": 4,
					"total": 4
				},
				"data": [
					{
						"id": %q,
						"email": "receiver4@mock.com",
						"external_id": "external_id_4",
						"phone_number": "+99994444",
						"created_at": %q,
						"updated_at": %q,
						"total_payments": "0",
						"successful_payments": "0",
						"failed_payments": "0",
						"canceled_payments": "0",
						"remaining_payments": "0",
						"registered_wallets":"0",
						"wallets": [
							{
								"id": %q,
								"receiver": {
									"id": %q
								},
								"wallet": {
									"id": %q,
									"name": "wallet1",
									"homepage": "https://www.wallet.com",
									"sep_10_client_domain": "www.wallet.com",
									"enabled": true
								},
								"status": "DRAFT",
								"created_at": %q,
								"updated_at": %q,
								"invitation_sent_at": null,
								"invited_at": %q,
								"last_message_sent_at": %q,
								"total_payments": "0",
								"payments_received": "0",
								"failed_payments": "0",
								"canceled_payments": "0",
								"remaining_payments": "0"
							}
						]
					}
				]
			}`, receiver4.ID, receiver4.CreatedAt.Format(time.RFC3339Nano), receiver4.UpdatedAt.Format(time.RFC3339Nano),
				receiverWallet4.ID, receiverWallet4.Receiver.ID, receiverWallet4.Wallet.ID,
				receiverWallet4.CreatedAt.Format(time.RFC3339Nano), receiverWallet4.UpdatedAt.Format(time.RFC3339Nano),
				message5.CreatedAt.Format(time.RFC3339Nano), message6.CreatedAt.Format(time.RFC3339Nano)),
		},
		{
			name: "fetch receivers with status draft",
			queryParams: map[string]string{
				"status": "dRaFt",
			},
			expectedStatusCode: http.StatusOK,
			expectedResponse: fmt.Sprintf(`{
				"pagination": {
					"pages": 1,
					"total": 1
				},
				"data": [
					{
						"id": %q,
						"email": "receiver4@mock.com",
						"external_id": "external_id_4",
						"phone_number": "+99994444",
						"created_at": %q,
						"updated_at": %q,
						"total_payments": "0",
						"successful_payments": "0",
						"failed_payments": "0",
						"canceled_payments": "0",
						"remaining_payments": "0",
						"registered_wallets":"0",
						"wallets": [
							{
								"id": %q,
								"receiver": {
									"id": %q
								},
								"wallet": {
									"id": %q,
									"name": "wallet1",
									"homepage": "https://www.wallet.com",
									"sep_10_client_domain": "www.wallet.com",
									"enabled": true
								},
								"status": "DRAFT",
								"created_at": %q,
								"updated_at": %q,
								"invitation_sent_at": null,
								"invited_at": %q,
								"last_message_sent_at": %q,
								"total_payments": "0",
								"payments_received": "0",
								"failed_payments": "0",
								"canceled_payments": "0",
								"remaining_payments": "0"
							}
						]
					}
				]
			}`, receiver4.ID, receiver4.CreatedAt.Format(time.RFC3339Nano), receiver4.UpdatedAt.Format(time.RFC3339Nano),
				receiverWallet4.ID, receiverWallet4.Receiver.ID, receiverWallet4.Wallet.ID,
				receiverWallet4.CreatedAt.Format(time.RFC3339Nano), receiverWallet4.UpdatedAt.Format(time.RFC3339Nano),
				message5.CreatedAt.Format(time.RFC3339Nano), message6.CreatedAt.Format(time.RFC3339Nano)),
		},
		{
			name: "fetch receivers created before 2023-01-01",
			queryParams: map[string]string{
				"created_at_before": "2023-01-01",
			},
			expectedStatusCode: http.StatusOK,
			expectedResponse: fmt.Sprintf(`{
				"pagination": {
					"pages": 1,
					"total": 1
				},
				"data": [
					{
						"id": %q,
						"email": "receiver1@mock.com",
						"external_id": "external_id_1",	
						"phone_number": "+99991111",
						"created_at": %q,
						"updated_at": %q,
						"total_payments": "0",
						"successful_payments": "0",
						"failed_payments": "0",
						"canceled_payments": "0",
						"remaining_payments": "0",
						"registered_wallets":"0",
						"wallets": []
					}
				]
			}`, receiver1.ID, receiver1.CreatedAt.Format(time.RFC3339Nano), receiver1.UpdatedAt.Format(time.RFC3339Nano)),
		},
		{
			name: "fetch receivers created after 2023-03-01",
			queryParams: map[string]string{
				"created_at_after": "2023-03-01",
			},
			expectedStatusCode: http.StatusOK,
			expectedResponse: fmt.Sprintf(`{
				"pagination": {
					"pages": 1,
					"total": 1
				},
				"data": [
					{
						"id": %q,
						"email": "receiver4@mock.com",
						"external_id": "external_id_4",
						"phone_number": "+99994444",
						"created_at": %q,
						"updated_at": %q,
						"total_payments": "0",
						"successful_payments": "0",
						"failed_payments": "0",
						"canceled_payments": "0",
						"remaining_payments": "0",
						"registered_wallets":"0",
						"wallets": [
							{
								"id": %q,
								"receiver": {
									"id": %q
								},
								"wallet": {
									"id": %q,
									"name": "wallet1",
									"homepage": "https://www.wallet.com",
									"sep_10_client_domain": "www.wallet.com",
									"enabled": true
								},
								"status": "DRAFT",
								"created_at": %q,
								"updated_at": %q,
								"invitation_sent_at": null,
								"invited_at": %q,
								"last_message_sent_at": %q,
								"total_payments": "0",
								"payments_received": "0",
								"failed_payments": "0",
								"canceled_payments": "0",
								"remaining_payments": "0"
							}
						]
					}
				]
			}`, receiver4.ID, receiver4.CreatedAt.Format(time.RFC3339Nano), receiver4.UpdatedAt.Format(time.RFC3339Nano),
				receiverWallet4.ID, receiverWallet4.Receiver.ID, receiverWallet4.Wallet.ID,
				receiverWallet4.CreatedAt.Format(time.RFC3339Nano), receiverWallet4.UpdatedAt.Format(time.RFC3339Nano),
				message5.CreatedAt.Format(time.RFC3339Nano), message6.CreatedAt.Format(time.RFC3339Nano)),
		},
		{
			name: "fetch receivers created after 2023-01-01 and before 2023-03-01",
			queryParams: map[string]string{
				"created_at_after":  "2023-01-01",
				"created_at_before": "2023-03-01",
				"sort":              "created_at",
				"direction":         "desc",
			},
			expectedStatusCode: http.StatusOK,
			expectedResponse: fmt.Sprintf(`{
				"pagination": {
					"pages": 1,
					"total": 2
				},
				"data": [
					{
						"id": %q,
						"email": "receiver3@mock.com",
						"external_id": "external_id_3",
						"phone_number": "+99993333",
						"created_at": %q,
						"updated_at": %q,
						"total_payments": "1",
						"successful_payments": "0",
						"received_amounts":  "0",
						"failed_payments": "0",
						"canceled_payments": "0",
						"remaining_payments": "1",
						"received_amounts":  [
							{
								"asset_code": "USDC",
								"asset_issuer": "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV",
								"received_amount": "0"
							}
						],
						"registered_wallets":"1",
						"wallets": [
							{
								"id": %q,
								"receiver": {
									"id": %q
								},
								"wallet": {
									"id": %q,
									"name": "wallet1",
									"homepage": "https://www.wallet.com",
									"sep_10_client_domain": "www.wallet.com",
									"enabled": true
								},
								"stellar_address": %q,
								"stellar_memo": %q,
								"stellar_memo_type": %q,
								"status": "REGISTERED",
								"created_at": %q,
								"updated_at": %q,
								"invitation_sent_at": null,
								"invited_at": %q,
								"last_message_sent_at": %q,
								"total_payments": "1",
								"payments_received": "0",
								"failed_payments": "0",
								"canceled_payments": "0",
								"remaining_payments": "1",
								"received_amounts":  [
									{
										"asset_code": "USDC",
										"asset_issuer": "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV",
										"received_amount": "0"
									}
								],
								"sep24_transaction_id": %q
							}
						]
					},
					{
						"id": %q,
						"email": "receiver2@mock.com",
						"external_id": "external_id_2",
						"phone_number": "+99992222",
						"created_at": %q,
						"updated_at": %q,
						"total_payments": "1",
						"successful_payments": "1",
						"failed_payments": "0",
						"canceled_payments": "0",
						"remaining_payments": "0",
						"received_amounts":  [
							{
								"asset_code": "USDC",
								"asset_issuer": "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV",
								"received_amount": "50.0000000"
							}
						],
						"registered_wallets":"1",
						"wallets": [
							{
								"id": %q,
								"receiver": {
									"id": %q
								},
								"wallet": {
									"id": %q,
									"name": "wallet1",
									"homepage": "https://www.wallet.com",
									"sep_10_client_domain": "www.wallet.com",
									"enabled": true
								},
								"stellar_address": %q,
								"stellar_memo": %q,
								"stellar_memo_type": %q,
								"status": "REGISTERED",
								"created_at": %q,
								"updated_at": %q,
								"invitation_sent_at": null,
								"invited_at": %q,
								"last_message_sent_at": %q,
								"total_payments": "1",
								"payments_received": "1",
								"failed_payments": "0",
								"canceled_payments": "0",
								"remaining_payments": "0",
								"received_amounts":  [
									{
										"asset_code": "USDC",
										"asset_issuer": "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV",
										"received_amount": "50.0000000"
									}
								],
								"sep24_transaction_id": %q
							}
						]
					}
				]
			}`, receiver3.ID, receiver3.CreatedAt.Format(time.RFC3339Nano), receiver3.UpdatedAt.Format(time.RFC3339Nano),
				receiverWallet3.ID, receiverWallet3.Receiver.ID, receiverWallet3.Wallet.ID,
				receiverWallet3.StellarAddress, receiverWallet3.StellarMemo, receiverWallet3.StellarMemoType,
				receiverWallet3.CreatedAt.Format(time.RFC3339Nano), receiverWallet3.UpdatedAt.Format(time.RFC3339Nano),
				message3.CreatedAt.Format(time.RFC3339Nano), message4.CreatedAt.Format(time.RFC3339Nano), receiverWallet3.SEP24TransactionID,
				receiver2.ID, receiver2.CreatedAt.Format(time.RFC3339Nano), receiver2.UpdatedAt.Format(time.RFC3339Nano),
				receiverWallet2.ID, receiverWallet2.Receiver.ID, receiverWallet2.Wallet.ID,
				receiverWallet2.StellarAddress, receiverWallet2.StellarMemo, receiverWallet2.StellarMemoType,
				receiverWallet2.CreatedAt.Format(time.RFC3339Nano), receiverWallet2.UpdatedAt.Format(time.RFC3339Nano),
				message1.CreatedAt.Format(time.RFC3339Nano), message2.CreatedAt.Format(time.RFC3339Nano), receiverWallet2.SEP24TransactionID),
		},
		{
			name: "fetch receivers with email = receiver1@mock.com",
			queryParams: map[string]string{
				"q": receiver1Email,
			},
			expectedStatusCode: http.StatusOK,
			expectedResponse: fmt.Sprintf(`{
				"pagination": {
					"pages": 1,
					"total": 1
				},
				"data": [
					{
						"id": %q,
						"email": "receiver1@mock.com",
						"external_id": "external_id_1",
						"phone_number": "+99991111",
						"created_at": %q,
						"updated_at": %q,
						"total_payments": "0",
						"successful_payments": "0",
						"failed_payments": "0",
						"canceled_payments": "0",
						"remaining_payments": "0",
						"registered_wallets":"0",
						"wallets": []
					}
				]
			}`, receiver1.ID, receiver1.CreatedAt.Format(time.RFC3339Nano), receiver1.UpdatedAt.Format(time.RFC3339Nano)),
		},
		{
			name: "fetch receivers with phone_number = +99991111",
			queryParams: map[string]string{
				"q": "+99991111",
			},
			expectedStatusCode: http.StatusOK,
			expectedResponse: fmt.Sprintf(`{
				"pagination": {
					"pages": 1,
					"total": 1
				},
				"data": [
					{
						"id": %q,
						"email": "receiver1@mock.com",
						"external_id": "external_id_1",
						"phone_number": "+99991111",
						"created_at": %q,
						"updated_at": %q,
						"total_payments": "0",
						"successful_payments": "0",
						"failed_payments": "0",
						"canceled_payments": "0",
						"remaining_payments": "0",
						"registered_wallets":"0",
						"wallets": []
					}
				]
			}`, receiver1.ID, receiver1.CreatedAt.Format(time.RFC3339Nano), receiver1.UpdatedAt.Format(time.RFC3339Nano)),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Build the URL for the test request
			url := buildURLWithQueryParams(ts.URL, "/receivers", tc.queryParams)
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

func Test_ReceiverHandler_BuildReceiversResponse(t *testing.T) {
	dbConnectionPool := testutils.GetDBConnectionPool(t)

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	handler := &ReceiverHandler{
		Models:           models,
		DBConnectionPool: dbConnectionPool,
	}

	ctx := context.Background()

	wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")

	receiver1Email := "receiver1@mock.com"
	receiver1 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
		Email:       receiver1Email,
		ExternalID:  "external_id_1",
		PhoneNumber: "+99991111",
	})
	receiver2Email := "receiver2@mock.com"
	receiver2 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
		Email:       receiver2Email,
		ExternalID:  "external_id_2",
		PhoneNumber: "+99992222",
	})

	receiverWallet1 := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver1.ID, wallet.ID, data.DraftReceiversWalletStatus)
	receiverWallet2 := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver2.ID, wallet.ID, data.ReadyReceiversWalletStatus)

	message1 := data.CreateMessageFixture(t, ctx, dbConnectionPool, &data.Message{
		Type:             message.MessengerTypeTwilioSMS,
		AssetID:          nil,
		ReceiverID:       receiver1.ID,
		WalletID:         wallet.ID,
		ReceiverWalletID: &receiverWallet1.ID,
		Status:           data.SuccessMessageStatus,
		CreatedAt:        time.Date(2023, 1, 10, 23, 40, 20, 1000, time.UTC),
	})

	message2 := data.CreateMessageFixture(t, ctx, dbConnectionPool, &data.Message{
		Type:             message.MessengerTypeTwilioSMS,
		AssetID:          nil,
		ReceiverID:       receiver1.ID,
		WalletID:         wallet.ID,
		ReceiverWalletID: &receiverWallet1.ID,
		Status:           data.SuccessMessageStatus,
		CreatedAt:        time.Date(2023, 2, 10, 23, 40, 20, 1000, time.UTC),
	})

	message3 := data.CreateMessageFixture(t, ctx, dbConnectionPool, &data.Message{
		Type:             message.MessengerTypeTwilioSMS,
		AssetID:          nil,
		ReceiverID:       receiver2.ID,
		WalletID:         wallet.ID,
		ReceiverWalletID: &receiverWallet2.ID,
		Status:           data.SuccessMessageStatus,
		CreatedAt:        time.Date(2023, 1, 10, 23, 40, 20, 1000, time.UTC),
	})

	message4 := data.CreateMessageFixture(t, ctx, dbConnectionPool, &data.Message{
		Type:             message.MessengerTypeTwilioSMS,
		AssetID:          nil,
		ReceiverID:       receiver2.ID,
		WalletID:         wallet.ID,
		ReceiverWalletID: &receiverWallet2.ID,
		Status:           data.SuccessMessageStatus,
		CreatedAt:        time.Date(2023, 2, 10, 23, 40, 20, 1000, time.UTC),
	})

	dbTx, err := dbConnectionPool.BeginTxx(ctx, nil)
	require.NoError(t, err)
	// Defer a rollback in case anything fails.
	defer func() {
		err = dbTx.Rollback()
		require.Error(t, err, "not in transaction")
	}()

	receivers, err := handler.Models.Receiver.GetAll(ctx, dbTx,
		&data.QueryParams{SortBy: data.SortFieldUpdatedAt, SortOrder: data.SortOrderDESC},
		data.QueryTypeSelectPaginated)
	require.NoError(t, err)
	receiversID := handler.Models.Receiver.ParseReceiverIDs(receivers)
	receiversWallets, err := handler.Models.ReceiverWallet.GetWithReceiverIDs(ctx, dbTx, receiversID)
	require.NoError(t, err)

	actualResponse := handler.buildReceiversResponse(receivers, receiversWallets)

	ar, err := json.Marshal(actualResponse)
	require.NoError(t, err)

	wantJSON := fmt.Sprintf(`[
		{
			"id": %q,
			"email": "receiver2@mock.com",
			"external_id": "external_id_2",
			"phone_number": "+99992222",
			"created_at": %q,
			"updated_at": %q,
			"total_payments": "0",
			"successful_payments": "0",
			"failed_payments": "0",
			"canceled_payments": "0",
			"remaining_payments": "0",
			"registered_wallets":"0",
			"wallets": [
				{
					"id": %q,
					"receiver": {
						"id": %q
					},
					"wallet": {
						"id": %q,
						"name": "wallet1",
						"homepage": "https://www.wallet.com",
						"sep_10_client_domain": "www.wallet.com",
						"enabled": true
					},
					"status": "READY",
					"created_at": %q,
					"updated_at": %q,
					"invitation_sent_at": null,
					"invited_at": %q,
					"last_message_sent_at": %q,
					"total_payments": "0",
					"payments_received": "0",
					"failed_payments": "0",
					"canceled_payments": "0",
					"remaining_payments": "0"
				}
			]
		},
		{
			"id": %q,
			"email": "receiver1@mock.com",
			"external_id": "external_id_1",
			"phone_number": "+99991111",
			"created_at": %q,
			"updated_at": %q,
			"total_payments": "0",
			"successful_payments": "0",
			"failed_payments": "0",
			"canceled_payments": "0",
			"remaining_payments": "0",
			"registered_wallets":"0",
			"wallets": [
				{
					"id": %q,
					"receiver": {
						"id": %q
					},
					"wallet": {
						"id": %q,
						"name": "wallet1",
						"homepage": "https://www.wallet.com",
						"sep_10_client_domain": "www.wallet.com",
						"enabled": true
					},
					"status": "DRAFT",
					"created_at": %q,
					"updated_at": %q,
					"invitation_sent_at": null,
					"invited_at": %q,
					"last_message_sent_at": %q,
					"total_payments": "0",
					"payments_received": "0",
					"failed_payments": "0",
					"canceled_payments": "0",
					"remaining_payments": "0"
				}
			]
		}
	]`, receiver2.ID, receiver2.CreatedAt.Format(time.RFC3339Nano), receiver2.UpdatedAt.Format(time.RFC3339Nano),
		receiverWallet2.ID, receiverWallet2.Receiver.ID, receiverWallet2.Wallet.ID,
		receiverWallet2.CreatedAt.Format(time.RFC3339Nano), receiverWallet2.UpdatedAt.Format(time.RFC3339Nano),
		message3.CreatedAt.Format(time.RFC3339Nano), message4.CreatedAt.Format(time.RFC3339Nano),
		receiver1.ID, receiver1.CreatedAt.Format(time.RFC3339Nano), receiver1.UpdatedAt.Format(time.RFC3339Nano),
		receiverWallet1.ID, receiverWallet1.Receiver.ID, receiverWallet1.Wallet.ID,
		receiverWallet1.CreatedAt.Format(time.RFC3339Nano), receiverWallet1.UpdatedAt.Format(time.RFC3339Nano),
		message1.CreatedAt.Format(time.RFC3339Nano), message2.CreatedAt.Format(time.RFC3339Nano))

	assert.JSONEq(t, wantJSON, string(ar))

	err = dbTx.Commit()
	require.NoError(t, err)
}

func Test_ReceiverHandler_GetReceiverVerificatioTypes(t *testing.T) {
	handler := &ReceiverHandler{}

	rr := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, "/receivers/verification-types", nil)
	require.NoError(t, err)
	http.HandlerFunc(handler.GetReceiverVerificationTypes).ServeHTTP(rr, req)

	resp := rr.Result()
	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	defer resp.Body.Close()
	expectedBody := `[
		"DATE_OF_BIRTH",
		"YEAR_MONTH",
		"PIN",
		"NATIONAL_ID_NUMBER"
	]`
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.JSONEq(t, expectedBody, string(respBody))
}

func Test_ReceiverHandler_CreateReceiver_Validation(t *testing.T) {
	r := chi.NewRouter()

	handler := &ReceiverHandler{}
	r.Post("/receivers", handler.CreateReceiver)

	testCases := []struct {
		name        string
		request     dto.CreateReceiverRequest
		expectError bool
		errorMsg    string
	}{
		{
			name: "missing required contact information",
			request: dto.CreateReceiverRequest{
				ExternalID: "Cadia-Station",
				Verifications: []dto.ReceiverVerificationRequest{
					{
						Type:  data.VerificationTypeDateOfBirth,
						Value: "1990-01-01",
					},
				},
			},
			expectError: true,
			errorMsg:    "email is required when phone_number is not provided",
		},
		{
			name: "invalid email format",
			request: dto.CreateReceiverRequest{
				Email:      "@example.com",
				ExternalID: "Cadia-Station",
				Verifications: []dto.ReceiverVerificationRequest{
					{
						Type:  data.VerificationTypeDateOfBirth,
						Value: "1990-01-01",
					},
				},
			},
			expectError: true,
			errorMsg:    "the email address provided is not valid",
		},
		{
			name: "invalid phone number format",
			request: dto.CreateReceiverRequest{
				PhoneNumber: "01-HERESY",
				ExternalID:  "Cadia-Station",
				Verifications: []dto.ReceiverVerificationRequest{
					{
						Type:  data.VerificationTypeDateOfBirth,
						Value: "1990-01-01",
					},
				},
			},
			expectError: true,
			errorMsg:    "the provided phone number is not a valid E.164 number",
		},
		{
			name: "missing external ID",
			request: dto.CreateReceiverRequest{
				Email: "inquisitor@example.com",
				Verifications: []dto.ReceiverVerificationRequest{
					{
						Type:  data.VerificationTypeDateOfBirth,
						Value: "1990-01-01",
					},
				},
			},
			expectError: true,
			errorMsg:    "external_id is required",
		},
		{
			name: "missing verifications and wallets",
			request: dto.CreateReceiverRequest{
				Email:      "magnus@example.com",
				ExternalID: "Prospero-001",
			},
			expectError: true,
			errorMsg:    "verifications are required when wallets are not provided",
		},
		{
			name: "invalid verification type",
			request: dto.CreateReceiverRequest{
				Email:      "magnus@example.com",
				ExternalID: "Prospero-001",
				Verifications: []dto.ReceiverVerificationRequest{
					{
						Type:  "WARP_TAINT",
						Value: "1990-01-01",
					},
				},
			},
			expectError: true,
			errorMsg:    "invalid verification type",
		},
		{
			name: "invalid date format",
			request: dto.CreateReceiverRequest{
				Email:      "magnus@example.com",
				ExternalID: "Prospero-001",
				Verifications: []dto.ReceiverVerificationRequest{
					{
						Type:  data.VerificationTypeDateOfBirth,
						Value: "30/M41",
					},
				},
			},
			expectError: true,
			errorMsg:    "invalid date of birth format: must be YYYY-MM-DD",
		},
		{
			name: "invalid stellar address format",
			request: dto.CreateReceiverRequest{
				Email:      "magnus@example.com",
				ExternalID: "Prospero-001",
				Wallets: []dto.ReceiverWalletRequest{
					{
						Address: "INVALIDADDRESS",
					},
				},
			},
			expectError: true,
			errorMsg:    "invalid stellar address format",
		},
		{
			name: "multiple wallets not allowed",
			request: dto.CreateReceiverRequest{
				Email:      "fulgrim@example.com",
				ExternalID: "Chemos-001",
				Wallets: []dto.ReceiverWalletRequest{
					{
						Address: "GCQFMQ7U33ICSLAVGBJNX6P66M5GGOTQWCRZ5Y3YXYK3EB3DNCWOAD5K",
						Memo:    "1337",
					},
					{
						Address: "GDQNY3PBOJOKYZSRMK2S7LHHGWZIUISD4QORETLMXEWXBI7KFZZMKTL3",
						Memo:    "1338",
					},
				},
			},
			expectError: true,
			errorMsg:    "only one wallet is allowed per receiver",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			validator := validators.NewReceiverValidator()
			validator.ValidateCreateReceiverRequest(&tc.request)

			if tc.expectError {
				require.True(t, validator.HasErrors(), "Expected validation errors but none found")

				found := false
				for _, value := range validator.Errors {
					if str, ok := value.(string); ok && strings.Contains(str, tc.errorMsg) {
						found = true
						break
					}
				}
				assert.True(t, found, "Expected error message '%s' should be present in validation errors", tc.errorMsg)
			} else {
				require.False(t, validator.HasErrors(), "Expected no validation errors but found: %v", validator.Errors)
			}
		})
	}
}

func Test_ReceiverHandler_CreateReceiver_HTTPValidationError(t *testing.T) {
	dbConnectionPool := testutils.GetDBConnectionPool(t)
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	handler := &ReceiverHandler{
		Models:           models,
		DBConnectionPool: dbConnectionPool,
	}

	r := chi.NewRouter()
	r.Post("/receivers", handler.CreateReceiver)

	testCases := []struct {
		name           string
		requestBody    dto.CreateReceiverRequest
		expectedStatus int
		expectedError  string
	}{
		{
			name: "missing required contact information",
			requestBody: dto.CreateReceiverRequest{
				ExternalID: "Cadia-Station",
				Verifications: []dto.ReceiverVerificationRequest{
					{
						Type:  data.VerificationTypeDateOfBirth,
						Value: "1990-01-01",
					},
				},
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "email is required when phone_number is not provided",
		},
		{
			name: "invalid email format",
			requestBody: dto.CreateReceiverRequest{
				Email:      "@example.com",
				ExternalID: "Cadia-Station",
				Verifications: []dto.ReceiverVerificationRequest{
					{
						Type:  data.VerificationTypeDateOfBirth,
						Value: "1990-01-01",
					},
				},
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "the email address provided is not valid",
		},
		{
			name: "missing external ID",
			requestBody: dto.CreateReceiverRequest{
				Email: "inquisitor@example.com",
				Verifications: []dto.ReceiverVerificationRequest{
					{
						Type:  data.VerificationTypeDateOfBirth,
						Value: "1990-01-01",
					},
				},
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "external_id is required",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			reqBody, err := json.Marshal(tc.requestBody)
			require.NoError(t, err)

			req := httptest.NewRequest("POST", "/receivers", bytes.NewBuffer(reqBody))
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()

			r.ServeHTTP(w, req)

			assert.Equal(t, tc.expectedStatus, w.Code)

			var errorResponse httperror.HTTPError
			err = json.Unmarshal(w.Body.Bytes(), &errorResponse)
			require.NoError(t, err)

			assert.Equal(t, "validation error", errorResponse.Error())

			require.NotNil(t, errorResponse.Extras)

			found := false
			for _, value := range errorResponse.Extras {
				if str, ok := value.(string); ok && strings.Contains(str, tc.expectedError) {
					found = true
					break
				}
			}
			assert.True(t, found, "Expected error message '%s' should be present in extras", tc.expectedError)
		})
	}
}

func Test_ReceiverHandler_CreateReceiver_Success(t *testing.T) {
	dbConnectionPool := testutils.GetDBConnectionPool(t)
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	ctx := context.Background()
	handler := &ReceiverHandler{
		Models:           models,
		DBConnectionPool: dbConnectionPool,
	}

	// Setup router
	r := chi.NewRouter()
	r.Post("/receivers", handler.CreateReceiver)

	testCases := []struct {
		name            string
		requestBody     dto.CreateReceiverRequest
		expectedStatus  int
		assertCreatedFn func(t *testing.T, receiverID string)
	}{
		{
			name: "create receiver with email and verifications",
			requestBody: dto.CreateReceiverRequest{
				Email:      "horus.lupercal@example.com",
				ExternalID: "Cadia-001",
				Verifications: []dto.ReceiverVerificationRequest{
					{
						Type:  data.VerificationTypeDateOfBirth,
						Value: "1990-01-01",
					},
					{
						Type:  data.VerificationTypePin,
						Value: "40401",
					},
				},
			},
			expectedStatus: http.StatusCreated,
			assertCreatedFn: func(t *testing.T, receiverID string) {
				receiver, err := models.Receiver.Get(ctx, dbConnectionPool, receiverID)
				require.NoError(t, err)
				assert.Equal(t, "horus.lupercal@example.com", receiver.Email)
				assert.Equal(t, "Cadia-001", receiver.ExternalID)

				verifications, err := models.ReceiverVerification.GetAllByReceiverID(ctx, dbConnectionPool, receiverID)
				require.NoError(t, err)
				assert.Len(t, verifications, 2)

				receiverWallets, err := models.ReceiverWallet.GetWithReceiverIDs(ctx, dbConnectionPool, data.ReceiverIDs{receiverID})
				require.NoError(t, err)
				assert.Len(t, receiverWallets, 0)
			},
		},
		{
			name: "create receiver with phone and wallet",
			requestBody: dto.CreateReceiverRequest{
				PhoneNumber: "+41555511112",
				ExternalID:  "Terra-001",
				Wallets: []dto.ReceiverWalletRequest{
					{
						Address: "GCQFMQ7U33ICSLAVGBJNX6P66M5GGOTQWCRZ5Y3YXYK3EB3DNCWOAD5K",
						Memo:    "13371337",
					},
				},
			},
			expectedStatus: http.StatusCreated,
			assertCreatedFn: func(t *testing.T, receiverID string) {
				receiver, err := models.Receiver.Get(ctx, dbConnectionPool, receiverID)
				require.NoError(t, err)
				assert.Equal(t, "+41555511112", receiver.PhoneNumber)
				assert.Equal(t, "Terra-001", receiver.ExternalID)

				verifications, err := models.ReceiverVerification.GetAllByReceiverID(ctx, dbConnectionPool, receiverID)
				require.NoError(t, err)
				assert.Len(t, verifications, 0)

				receiverWallets, err := models.ReceiverWallet.GetWithReceiverIDs(ctx, dbConnectionPool, data.ReceiverIDs{receiverID})
				require.NoError(t, err)
				assert.Len(t, receiverWallets, 1)

				wallet := receiverWallets[0]
				assert.Equal(t, data.RegisteredReceiversWalletStatus, wallet.Status)
				assert.Equal(t, "GCQFMQ7U33ICSLAVGBJNX6P66M5GGOTQWCRZ5Y3YXYK3EB3DNCWOAD5K", wallet.StellarAddress)
				assert.Equal(t, "13371337", wallet.StellarMemo)
				assert.Equal(t, schema.MemoTypeID, wallet.StellarMemoType)
			},
		},
		{
			name: "create complete receiver with both email/phone and verifications/wallet",
			requestBody: dto.CreateReceiverRequest{
				Email:       "guilliman@example.com",
				PhoneNumber: "+41555511111",
				ExternalID:  "Ultramar-001",
				Verifications: []dto.ReceiverVerificationRequest{
					{
						Type:  data.VerificationTypeDateOfBirth,
						Value: "1990-01-01",
					},
				},
				Wallets: []dto.ReceiverWalletRequest{
					{
						Address: "GCQFMQ7U33ICSLAVGBJNX6P66M5GGOTQWCRZ5Y3YXYK3EB3DNCWOAD5K",
					},
				},
			},
			expectedStatus: http.StatusCreated,
			assertCreatedFn: func(t *testing.T, receiverID string) {
				receiver, err := models.Receiver.Get(ctx, dbConnectionPool, receiverID)
				require.NoError(t, err)
				assert.Equal(t, "guilliman@example.com", receiver.Email)
				assert.Equal(t, "+41555511111", receiver.PhoneNumber)
				assert.Equal(t, "Ultramar-001", receiver.ExternalID)

				verifications, err := models.ReceiverVerification.GetAllByReceiverID(ctx, dbConnectionPool, receiverID)
				require.NoError(t, err)
				assert.Len(t, verifications, 1)

				receiverWallets, err := models.ReceiverWallet.GetWithReceiverIDs(ctx, dbConnectionPool, data.ReceiverIDs{receiverID})
				require.NoError(t, err)
				assert.Len(t, receiverWallets, 1)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			data.DeleteAllFixtures(t, ctx, dbConnectionPool)

			wallets := data.CreateWalletFixtures(t, ctx, dbConnectionPool)
			data.MakeWalletUserManaged(t, ctx, dbConnectionPool, wallets[0].ID)

			jsonBody, err := json.Marshal(tc.requestBody)
			require.NoError(t, err)

			req, err := http.NewRequest("POST", "/receivers", bytes.NewBuffer(jsonBody))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			assert.Equal(t, tc.expectedStatus, rr.Code)

			if tc.expectedStatus == http.StatusCreated {
				var response GetReceiverResponse
				err = json.Unmarshal(rr.Body.Bytes(), &response)
				require.NoError(t, err)

				// Check created receiver details
				tc.assertCreatedFn(t, response.Receiver.ID)
			}
		})
	}
}

func Test_ReceiverHandler_CreateReceiver_Conflict(t *testing.T) {
	dbConnectionPool := testutils.GetDBConnectionPool(t)
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	ctx := context.Background()
	data.DeleteAllFixtures(t, ctx, dbConnectionPool)

	handler := &ReceiverHandler{
		Models:           models,
		DBConnectionPool: dbConnectionPool,
	}

	r := chi.NewRouter()
	r.Post("/receivers", handler.CreateReceiver)

	wallets := data.CreateWalletFixtures(t, ctx, dbConnectionPool)
	data.MakeWalletUserManaged(t, ctx, dbConnectionPool, wallets[0].ID)

	// Create a receiver with a specific phone number and email
	existingReceiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
		PhoneNumber: "+14155556666",
		Email:       "existing@example.com",
	})

	existingWalletAddress := "GCQFMQ7U33ICSLAVGBJNX6P66M5GGOTQWCRZ5Y3YXYK3EB3DNCWOAD5K"
	receiverWalletID, err := models.ReceiverWallet.GetOrInsertReceiverWallet(ctx, dbConnectionPool, data.ReceiverWalletInsert{
		ReceiverID: existingReceiver.ID,
		WalletID:   wallets[0].ID,
	})
	require.NoError(t, err)

	err = models.ReceiverWallet.Update(ctx, receiverWalletID, data.ReceiverWalletUpdate{
		Status:         data.RegisteredReceiversWalletStatus,
		StellarAddress: existingWalletAddress,
	}, dbConnectionPool)
	require.NoError(t, err)

	testCases := []struct {
		name         string
		request      dto.CreateReceiverRequest
		expectedBody string
	}{
		{
			name: "duplicate email conflict",
			request: dto.CreateReceiverRequest{
				Email:      existingReceiver.Email,
				ExternalID: "test-external-id-1",
				Verifications: []dto.ReceiverVerificationRequest{
					{
						Type:  data.VerificationTypeDateOfBirth,
						Value: "1990-01-01",
					},
				},
			},
			expectedBody: `{
				"error": "The provided email is already associated with another user.",
				"extras": {
					"email": "email must be unique"
				}
			}`,
		},
		{
			name: "duplicate phone number conflict",
			request: dto.CreateReceiverRequest{
				PhoneNumber: existingReceiver.PhoneNumber,
				ExternalID:  "test-external-id-2",
				Verifications: []dto.ReceiverVerificationRequest{
					{
						Type:  data.VerificationTypeDateOfBirth,
						Value: "1990-01-01",
					},
				},
			},
			expectedBody: `{
				"error": "The provided phone number is already associated with another user.",
				"extras": {
					"phone_number": "phone number must be unique"
				}
			}`,
		},
		{
			name: "duplicate wallet address conflict",
			request: dto.CreateReceiverRequest{
				PhoneNumber: "+14155557777",
				ExternalID:  "test-external-id-3",
				Wallets: []dto.ReceiverWalletRequest{
					{
						Address: existingWalletAddress,
						Memo:    "12345678",
					},
				},
			},
			expectedBody: `{
				"error": "The provided wallet address is already associated with another user.",
				"extras": {
					"wallet_address": "wallet address must be unique"
				}
			}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			reqBody, err := json.Marshal(tc.request)
			require.NoError(t, err)

			req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/receivers", bytes.NewReader(reqBody))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			assert.Equal(t, http.StatusConflict, rr.Code)
			assert.JSONEq(t, tc.expectedBody, rr.Body.String())
		})
	}
}

func Test_ReceiverHandler_CreateReceiver_MemoTypeDetection(t *testing.T) {
	dbConnectionPool := testutils.GetDBConnectionPool(t)
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	ctx := context.Background()
	data.DeleteAllFixtures(t, ctx, dbConnectionPool)

	handler := &ReceiverHandler{
		Models:           models,
		DBConnectionPool: dbConnectionPool,
	}

	r := chi.NewRouter()
	r.Post("/receivers", handler.CreateReceiver)

	testCases := []struct {
		name             string
		memo             string
		expectedMemoType schema.MemoType
		expectedStatus   int
		expectError      bool
	}{
		{
			name:             "numeric memo should be detected as ID type",
			memo:             "12345678",
			expectedMemoType: schema.MemoTypeID,
			expectedStatus:   http.StatusCreated,
			expectError:      false,
		},
		{
			name:             "text memo should be detected as TEXT type",
			memo:             "hello",
			expectedMemoType: schema.MemoTypeText,
			expectedStatus:   http.StatusCreated,
			expectError:      false,
		},
		{
			name:             "hash memo should be detected as HASH type",
			memo:             "12f37f82eb6708daa0ac372a1a67a0f33efa6a9cd213ed430517e45fefb51577",
			expectedMemoType: schema.MemoTypeHash,
			expectedStatus:   http.StatusCreated,
			expectError:      false,
		},
		{
			name:           "invalid memo that cannot be parsed should return error",
			memo:           "this-is-a-very-long-string-also-not-valid-hex",
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
	}

	for i, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			wallets := data.CreateWalletFixtures(t, ctx, dbConnectionPool)
			data.MakeWalletUserManaged(t, ctx, dbConnectionPool, wallets[0].ID)

			requestBody := dto.CreateReceiverRequest{
				PhoneNumber: fmt.Sprintf("+41555511%03d", 100+i),
				ExternalID:  fmt.Sprintf("MemoTest-%d", i),
				Wallets: []dto.ReceiverWalletRequest{
					{
						Address: "GCQFMQ7U33ICSLAVGBJNX6P66M5GGOTQWCRZ5Y3YXYK3EB3DNCWOAD5K",
						Memo:    tc.memo,
					},
				},
			}

			reqBody, err := json.Marshal(requestBody)
			require.NoError(t, err)

			req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/receivers", bytes.NewReader(reqBody))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			assert.Equal(t, tc.expectedStatus, rr.Code)

			if tc.expectError {
				var errorResponse map[string]interface{}
				err = json.Unmarshal(rr.Body.Bytes(), &errorResponse)
				require.NoError(t, err)
				assert.Contains(t, errorResponse, "error")
			} else {
				var response GetReceiverResponse
				err = json.Unmarshal(rr.Body.Bytes(), &response)
				require.NoError(t, err)

				// Verify the receiver was created
				assert.NotEmpty(t, response.Receiver.ID)
				assert.Len(t, response.Wallets, 1)

				// Verify the memo and memo type
				wallet := response.Wallets[0]
				assert.Equal(t, tc.memo, wallet.StellarMemo)
				assert.Equal(t, tc.expectedMemoType, wallet.StellarMemoType)

				// Clean up
				data.DeleteAllFixtures(t, ctx, dbConnectionPool)
			}
		})
	}
}
