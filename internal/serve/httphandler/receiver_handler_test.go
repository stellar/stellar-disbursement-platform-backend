package httphandler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_ReceiverHandlerGet(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

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
	country := data.CreateCountryFixture(t, ctx, dbConnectionPool, "FRA", "France")
	wallet1 := data.CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet1.com", "www.wallet1.com", "wallet1://")

	receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})

	disbursement := data.Disbursement{
		Status:  data.DraftDisbursementStatus,
		Asset:   asset,
		Country: country,
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

		wantJson := fmt.Sprintf(`{
			"id": %q,
			"external_id": %q,
			"email": %q,
			"phone_number": %q,
			"created_at": %q,
			"updated_at": %q,
			"total_payments": "0",
			"successful_payments": "0",
			"failed_payments": "0",
    		"remaining_payments": "0",
			"registered_wallets": "0",
			"wallets": []
		}`, receiver.ID, receiver.ExternalID, *receiver.Email, receiver.PhoneNumber, receiver.CreatedAt.Format(time.RFC3339Nano), receiver.UpdatedAt.Format(time.RFC3339Nano))

		assert.JSONEq(t, wantJson, rr.Body.String())
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

		wantJson := fmt.Sprintf(`{
			"id": %q,
			"external_id": %q,
			"email": %q,
			"phone_number": %q,
			"created_at": %q,
			"updated_at": %q,
			"total_payments": "1",
			"successful_payments": "1",
			"failed_payments": "0",
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
					"stellar_address": %q,
					"stellar_memo": %q,
					"stellar_memo_type": %q,
					"status": "DRAFT",
					"created_at": %q,
					"updated_at": %q,
					"invited_at": %q,
					"last_sms_sent": %q,
					"total_payments": "1",
					"payments_received": "1",
					"failed_payments": "0",
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
		}`, receiver.ID, receiver.ExternalID, *receiver.Email, receiver.PhoneNumber, receiver.CreatedAt.Format(time.RFC3339Nano),
			receiver.UpdatedAt.Format(time.RFC3339Nano), receiverWallet1.ID, receiverWallet1.Receiver.ID, receiverWallet1.Wallet.ID,
			receiverWallet1.StellarAddress, receiverWallet1.StellarMemo, receiverWallet1.StellarMemoType,
			receiverWallet1.CreatedAt.Format(time.RFC3339Nano), receiverWallet1.UpdatedAt.Format(time.RFC3339Nano),
			message1.CreatedAt.Format(time.RFC3339Nano), message2.CreatedAt.Format(time.RFC3339Nano))

		assert.JSONEq(t, wantJson, rr.Body.String())
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

		wantJson := fmt.Sprintf(`{
			"id": %q,
			"external_id": %q,
			"email": %q,
			"phone_number": %q,
			"created_at": %q,
			"updated_at": %q,
			"total_payments": "2",
			"successful_payments": "1",
			"failed_payments": "0",
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
					"stellar_address": %q,
					"stellar_memo": %q,
					"stellar_memo_type": %q,
					"status": "DRAFT",
					"created_at": %q,
					"updated_at": %q,
					"invited_at": %q,
					"last_sms_sent": %q,
					"total_payments": "1",
					"payments_received": "1",
					"failed_payments": "0",
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
					"invited_at": %q,
					"last_sms_sent": %q,
					"total_payments": "1",
					"payments_received": "0",
					"failed_payments": "0",
					"remaining_payments": "1",
					"received_amounts":  [
						{
							"asset_code": "USDC",
							"asset_issuer": "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV",
							"received_amount": "0"
						}
					]
				}
			]
		}`, receiver.ID, receiver.ExternalID, *receiver.Email, receiver.PhoneNumber, receiver.CreatedAt.Format(time.RFC3339Nano),
			receiver.UpdatedAt.Format(time.RFC3339Nano), receiverWallet1.ID, receiverWallet1.Receiver.ID,
			receiverWallet1.Wallet.ID, receiverWallet1.StellarAddress, receiverWallet1.StellarMemo, receiverWallet1.StellarMemoType,
			receiverWallet1.CreatedAt.Format(time.RFC3339Nano), receiverWallet1.UpdatedAt.Format(time.RFC3339Nano),
			message1.CreatedAt.Format(time.RFC3339Nano), message2.CreatedAt.Format(time.RFC3339Nano),
			receiverWallet2.ID, receiverWallet2.Receiver.ID, receiverWallet2.Wallet.ID,
			receiverWallet2.StellarAddress, receiverWallet2.StellarMemo, receiverWallet2.StellarMemoType,
			receiverWallet2.CreatedAt.Format(time.RFC3339Nano), receiverWallet2.UpdatedAt.Format(time.RFC3339Nano),
			message3.CreatedAt.Format(time.RFC3339Nano), message4.CreatedAt.Format(time.RFC3339Nano))

		assert.JSONEq(t, wantJson, rr.Body.String())
	})

	t.Run("error receiver not found for given ID", func(t *testing.T) {
		// test
		req, err := http.NewRequest("GET", "/receivers/invalid_id", nil)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		// assert response
		assert.Equal(t, http.StatusNotFound, rr.Code)

		wantJson := `{
			"error": "could not retrieve receiver with ID: invalid_id"
		}`
		assert.JSONEq(t, wantJson, rr.Body.String())
	})
}

func Test_ReceiverHandler_GetReceivers_Errors(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

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
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

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
	country := data.CreateCountryFixture(t, ctx, dbConnectionPool, "FRA", "France")
	wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")

	// create receivers
	date := time.Date(2022, 12, 10, 23, 40, 20, 1431, time.UTC)
	receiver1Email := "receiver1@mock.com"
	receiver1 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
		Email:       &receiver1Email,
		ExternalID:  "external_id_1",
		PhoneNumber: "+99991111",
		CreatedAt:   &date,
		UpdatedAt:   &date,
	})

	date = time.Date(2023, 1, 10, 23, 40, 20, 1431, time.UTC)
	receiver2Email := "receiver2@mock.com"
	receiver2 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
		Email:       &receiver2Email,
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
		Email:       &receiver3Email,
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
	receiver4Email := "receiver4@mock.com"
	receiver4 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
		Email:       &receiver4Email,
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
								"stellar_address": %q,
								"stellar_memo": %q,
								"stellar_memo_type": %q,
								"status": "DRAFT",
								"created_at": %q,
								"updated_at": %q,
								"invited_at": %q,
								"last_sms_sent": %q,
								"total_payments": "0",
								"payments_received": "0",
								"failed_payments": "0",
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
								"invited_at": %q,
								"last_sms_sent": %q,
								"total_payments": "1",
								"payments_received": "0",
								"failed_payments": "0",
								"remaining_payments": "1",
								"received_amounts":  [
									{
										"asset_code": "USDC",
										"asset_issuer": "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV",
										"received_amount": "0"
									}
								]
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
								"invited_at": %q,
								"last_sms_sent": %q,
								"total_payments": "1",
								"payments_received": "1",
								"failed_payments": "0",
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
						"remaining_payments": "0",
						"registered_wallets":"0",
						"wallets": []
					}
				]
			}`,
				receiver4.ID, receiver4.CreatedAt.Format(time.RFC3339Nano), receiver4.UpdatedAt.Format(time.RFC3339Nano),
				receiverWallet4.ID, receiverWallet4.Receiver.ID, receiverWallet4.Wallet.ID,
				receiverWallet4.StellarAddress, receiverWallet4.StellarMemo, receiverWallet4.StellarMemoType,
				receiverWallet4.CreatedAt.Format(time.RFC3339Nano), receiverWallet4.UpdatedAt.Format(time.RFC3339Nano),
				message5.CreatedAt.Format(time.RFC3339Nano), message6.CreatedAt.Format(time.RFC3339Nano),
				receiver3.ID, receiver3.CreatedAt.Format(time.RFC3339Nano), receiver3.UpdatedAt.Format(time.RFC3339Nano),
				receiverWallet3.ID, receiverWallet3.Receiver.ID, receiverWallet3.Wallet.ID,
				receiverWallet3.StellarAddress, receiverWallet3.StellarMemo, receiverWallet3.StellarMemoType,
				receiverWallet3.CreatedAt.Format(time.RFC3339Nano), receiverWallet3.UpdatedAt.Format(time.RFC3339Nano),
				message3.CreatedAt.Format(time.RFC3339Nano), message4.CreatedAt.Format(time.RFC3339Nano),
				receiver2.ID, receiver2.CreatedAt.Format(time.RFC3339Nano), receiver2.UpdatedAt.Format(time.RFC3339Nano),
				receiverWallet2.ID, receiverWallet2.Receiver.ID, receiverWallet2.Wallet.ID, receiverWallet2.StellarAddress,
				receiverWallet2.StellarMemo, receiverWallet2.StellarMemoType,
				receiverWallet2.CreatedAt.Format(time.RFC3339Nano), receiverWallet2.UpdatedAt.Format(time.RFC3339Nano),
				message1.CreatedAt.Format(time.RFC3339Nano), message2.CreatedAt.Format(time.RFC3339Nano),
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
								"invited_at": %q,
								"last_sms_sent": %q,
								"total_payments": "1",
								"payments_received": "1",
								"failed_payments": "0",
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
					}
				]
			}`, receiver2.ID, receiver2.CreatedAt.Format(time.RFC3339Nano), receiver2.UpdatedAt.Format(time.RFC3339Nano),
				receiverWallet2.ID, receiverWallet2.Receiver.ID, receiverWallet2.Wallet.ID,
				receiverWallet2.StellarAddress, receiverWallet2.StellarMemo, receiverWallet2.StellarMemoType,
				receiverWallet2.CreatedAt.Format(time.RFC3339Nano), receiverWallet2.UpdatedAt.Format(time.RFC3339Nano),
				message1.CreatedAt.Format(time.RFC3339Nano), message2.CreatedAt.Format(time.RFC3339Nano)),
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
								"stellar_address": %q,
								"stellar_memo": %q,
								"stellar_memo_type": %q,
								"status": "DRAFT",
								"created_at": %q,
								"updated_at": %q,
								"invited_at": %q,
								"last_sms_sent": %q,
								"total_payments": "0",
								"payments_received": "0",
								"failed_payments": "0",
								"remaining_payments": "0"
							}
						]
					}
				]
			}`, receiver4.ID, receiver4.CreatedAt.Format(time.RFC3339Nano), receiver4.UpdatedAt.Format(time.RFC3339Nano),
				receiverWallet4.ID, receiverWallet4.Receiver.ID, receiverWallet4.Wallet.ID,
				receiverWallet4.StellarAddress, receiverWallet4.StellarMemo, receiverWallet4.StellarMemoType,
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
								"stellar_address": %q,
								"stellar_memo": %q,
								"stellar_memo_type": %q,
								"status": "DRAFT",
								"created_at": %q,
								"updated_at": %q,
								"invited_at": %q,
								"last_sms_sent": %q,
								"total_payments": "0",
								"payments_received": "0",
								"failed_payments": "0",
								"remaining_payments": "0"
							}
						]
					}
				]
			}`, receiver4.ID, receiver4.CreatedAt.Format(time.RFC3339Nano), receiver4.UpdatedAt.Format(time.RFC3339Nano),
				receiverWallet4.ID, receiverWallet4.Receiver.ID, receiverWallet4.Wallet.ID,
				receiverWallet4.StellarAddress, receiverWallet4.StellarMemo, receiverWallet4.StellarMemoType,
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
								"stellar_address": %q,
								"stellar_memo": %q,
								"stellar_memo_type": %q,
								"status": "DRAFT",
								"created_at": %q,
								"updated_at": %q,
								"invited_at": %q,
								"last_sms_sent": %q,
								"total_payments": "0",
								"payments_received": "0",
								"failed_payments": "0",
								"remaining_payments": "0"
							}
						]
					}
				]
			}`, receiver4.ID, receiver4.CreatedAt.Format(time.RFC3339Nano), receiver4.UpdatedAt.Format(time.RFC3339Nano),
				receiverWallet4.ID, receiverWallet4.Receiver.ID, receiverWallet4.Wallet.ID,
				receiverWallet4.StellarAddress, receiverWallet4.StellarMemo, receiverWallet4.StellarMemoType,
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
								"invited_at": %q,
								"last_sms_sent": %q,
								"total_payments": "1",
								"payments_received": "0",
								"failed_payments": "0",
								"remaining_payments": "1",
								"received_amounts":  [
									{
										"asset_code": "USDC",
										"asset_issuer": "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV",
										"received_amount": "0"
									}
								]
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
								"invited_at": %q,
								"last_sms_sent": %q,
								"total_payments": "1",
								"payments_received": "1",
								"failed_payments": "0",
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
					}
				]
			}`, receiver3.ID, receiver3.CreatedAt.Format(time.RFC3339Nano), receiver3.UpdatedAt.Format(time.RFC3339Nano),
				receiverWallet3.ID, receiverWallet3.Receiver.ID, receiverWallet3.Wallet.ID,
				receiverWallet3.StellarAddress, receiverWallet3.StellarMemo, receiverWallet3.StellarMemoType,
				receiverWallet3.CreatedAt.Format(time.RFC3339Nano), receiverWallet3.UpdatedAt.Format(time.RFC3339Nano),
				message3.CreatedAt.Format(time.RFC3339Nano), message4.CreatedAt.Format(time.RFC3339Nano),
				receiver2.ID, receiver2.CreatedAt.Format(time.RFC3339Nano), receiver2.UpdatedAt.Format(time.RFC3339Nano),
				receiverWallet2.ID, receiverWallet2.Receiver.ID, receiverWallet2.Wallet.ID,
				receiverWallet2.StellarAddress, receiverWallet2.StellarMemo, receiverWallet2.StellarMemoType,
				receiverWallet2.CreatedAt.Format(time.RFC3339Nano), receiverWallet2.UpdatedAt.Format(time.RFC3339Nano),
				message1.CreatedAt.Format(time.RFC3339Nano), message2.CreatedAt.Format(time.RFC3339Nano)),
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
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

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
		Email:       &receiver1Email,
		ExternalID:  "external_id_1",
		PhoneNumber: "+99991111",
	})
	receiver2Email := "receiver2@mock.com"
	receiver2 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
		Email:       &receiver2Email,
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

	receivers, err := handler.Models.Receiver.GetAll(ctx, dbTx, &data.QueryParams{SortBy: data.SortFieldUpdatedAt, SortOrder: data.SortOrderDESC})
	require.NoError(t, err)
	receiversId := handler.Models.Receiver.ParseReceiverIDs(receivers)
	receiversWallets, err := handler.Models.ReceiverWallet.GetWithReceiverIds(ctx, dbTx, receiversId)
	require.NoError(t, err)

	actualResponse := handler.buildReceiversResponse(receivers, receiversWallets)

	ar, err := json.Marshal(actualResponse)
	require.NoError(t, err)

	wantJson := fmt.Sprintf(`[
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
					"stellar_address": %q,
					"stellar_memo": %q,
					"stellar_memo_type": %q,
					"status": "READY",
					"created_at": %q,
					"updated_at": %q,
					"invited_at": %q,
					"last_sms_sent": %q,
					"total_payments": "0",
					"payments_received": "0",
					"failed_payments": "0",
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
					"stellar_address": %q,
					"stellar_memo": %q,
					"stellar_memo_type": %q,
					"status": "DRAFT",
					"created_at": %q,
					"updated_at": %q,
					"invited_at": %q,
					"last_sms_sent": %q,
					"total_payments": "0",
					"payments_received": "0",
					"failed_payments": "0",
					"remaining_payments": "0"
				}
			]
		}
	]`, receiver2.ID, receiver2.CreatedAt.Format(time.RFC3339Nano), receiver2.UpdatedAt.Format(time.RFC3339Nano),
		receiverWallet2.ID, receiverWallet2.Receiver.ID, receiverWallet2.Wallet.ID,
		receiverWallet2.StellarAddress, receiverWallet2.StellarMemo, receiverWallet2.StellarMemoType,
		receiverWallet2.CreatedAt.Format(time.RFC3339Nano), receiverWallet2.UpdatedAt.Format(time.RFC3339Nano),
		message3.CreatedAt.Format(time.RFC3339Nano), message4.CreatedAt.Format(time.RFC3339Nano),
		receiver1.ID, receiver1.CreatedAt.Format(time.RFC3339Nano), receiver1.UpdatedAt.Format(time.RFC3339Nano),
		receiverWallet1.ID, receiverWallet1.Receiver.ID, receiverWallet1.Wallet.ID,
		receiverWallet1.StellarAddress, receiverWallet1.StellarMemo, receiverWallet1.StellarMemoType,
		receiverWallet1.CreatedAt.Format(time.RFC3339Nano), receiverWallet1.UpdatedAt.Format(time.RFC3339Nano),
		message1.CreatedAt.Format(time.RFC3339Nano), message2.CreatedAt.Format(time.RFC3339Nano))

	assert.JSONEq(t, wantJson, string(ar))

	err = dbTx.Commit()
	require.NoError(t, err)
}
