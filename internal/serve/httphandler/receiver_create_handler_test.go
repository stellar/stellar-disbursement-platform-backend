// internal/serve/httphandler/receiver_create_handler_test.go
package httphandler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

func Test_ReceiverHandler_CreateReceiver_Validation(t *testing.T) {
	r := chi.NewRouter()

	handler := &ReceiverHandler{}
	r.Post("/receivers", handler.CreateReceiver)

	testCases := []struct {
		name        string
		request     CreateReceiverRequest
		expectError bool
		errorMsg    string
	}{
		{
			name: "missing required contact information",
			request: CreateReceiverRequest{
				ExternalID: "Cadia-Station",
				Verifications: []VerificationRequest{
					{
						Type:  data.VerificationTypeDateOfBirth,
						Value: "1990-01-01",
					},
				},
			},
			expectError: true,
			errorMsg:    "either email or phone_number must be provided",
		},
		{
			name: "invalid email format",
			request: CreateReceiverRequest{
				Email:      "@horus.com",
				ExternalID: "Cadia-Station",
				Verifications: []VerificationRequest{
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
			request: CreateReceiverRequest{
				PhoneNumber: "01-HERESY",
				ExternalID:  "Cadia-Station",
				Verifications: []VerificationRequest{
					{
						Type:  data.VerificationTypeDateOfBirth,
						Value: "1990-01-01",
					},
				},
			},
			expectError: true,
			errorMsg:    "invalid phone_number: the provided phone number is not a valid E.164 number",
		},
		{
			name: "missing external ID",
			request: CreateReceiverRequest{
				Email: "inquisitor@imperium.gov",
				Verifications: []VerificationRequest{
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
			request: CreateReceiverRequest{
				Email:      "magnus@prospero.edu",
				ExternalID: "Prospero-001",
			},
			expectError: true,
			errorMsg:    "either verifications or wallets must be provided",
		},
		{
			name: "invalid verification type",
			request: CreateReceiverRequest{
				Email:      "magnus@prospero.edu",
				ExternalID: "Prospero-001",
				Verifications: []VerificationRequest{
					{
						Type:  "WARP_TAINT",
						Value: "1990-01-01",
					},
				},
			},
			expectError: true,
			errorMsg:    "invalid verification type for verification[0]: WARP_TAINT",
		},
		{
			name: "invalid date format",
			request: CreateReceiverRequest{
				Email:      "magnus@prospero.edu",
				ExternalID: "Prospero-001",
				Verifications: []VerificationRequest{
					{
						Type:  data.VerificationTypeDateOfBirth,
						Value: "30/M41",
					},
				},
			},
			expectError: true,
			errorMsg:    "invalid date of birth format for verification[0]: must be YYYY-MM-DD",
		},
		{
			name: "invalid stellar address format",
			request: CreateReceiverRequest{
				Email:      "magnus@prospero.edu",
				ExternalID: "Prospero-001",
				Wallets: []WalletRequest{
					{
						Address: "INVALIDADDRESS",
					},
				},
			},
			expectError: true,
			errorMsg:    "invalid stellar address for wallet[0]",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.request.Validate()

			if tc.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errorMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func Test_ReceiverHandler_CreateReceiver_Success(t *testing.T) {
	dbConnectionPool := getDBConnectionPool(t)
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	// Create a user-managed wallet for the tests
	ctx := context.Background()
	wallets := data.ClearAndCreateWalletFixtures(t, ctx, dbConnectionPool)
	data.MakeWalletUserManaged(t, ctx, dbConnectionPool, wallets[0].ID)

	handler := &ReceiverHandler{
		Models:           models,
		DBConnectionPool: dbConnectionPool,
	}

	// Setup router
	r := chi.NewRouter()
	r.Post("/receivers", handler.CreateReceiver)

	testCases := []struct {
		name            string
		requestBody     CreateReceiverRequest
		expectedStatus  int
		assertCreatedFn func(t *testing.T, receiverID string)
	}{
		{
			name: "create receiver with email and verifications",
			requestBody: CreateReceiverRequest{
				Email:      "horus.lupercal@chaos.com",
				ExternalID: "Cadia-001",
				Verifications: []VerificationRequest{
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
				assert.Equal(t, "horus.lupercal@chaos.com", receiver.Email)
				assert.Equal(t, "Cadia-001", receiver.ExternalID)

				verifications, err := models.ReceiverVerification.GetAllByReceiverId(ctx, dbConnectionPool, receiverID)
				require.NoError(t, err)
				assert.Len(t, verifications, 2)

				receiverWallets, err := models.ReceiverWallet.GetWithReceiverIDs(ctx, dbConnectionPool, data.ReceiverIDs{receiverID})
				require.NoError(t, err)
				assert.Len(t, receiverWallets, 0)
			},
		},
		{
			name: "create receiver with phone and wallet",
			requestBody: CreateReceiverRequest{
				PhoneNumber: "+41555511112",
				ExternalID:  "Terra-001",
				Wallets: []WalletRequest{
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

				verifications, err := models.ReceiverVerification.GetAllByReceiverId(ctx, dbConnectionPool, receiverID)
				require.NoError(t, err)
				assert.Len(t, verifications, 0)

				receiverWallets, err := models.ReceiverWallet.GetWithReceiverIDs(ctx, dbConnectionPool, data.ReceiverIDs{receiverID})
				require.NoError(t, err)
				assert.Len(t, receiverWallets, 1)

				wallet := receiverWallets[0]
				assert.Equal(t, data.ReadyReceiversWalletStatus, wallet.Status)
				assert.Equal(t, "GCQFMQ7U33ICSLAVGBJNX6P66M5GGOTQWCRZ5Y3YXYK3EB3DNCWOAD5K", wallet.StellarAddress)
				assert.Equal(t, "13371337", wallet.StellarMemo)
				assert.Equal(t, schema.MemoTypeID, wallet.StellarMemoType)
			},
		},
		{
			name: "create complete receiver with both email/phone and verifications/wallet",
			requestBody: CreateReceiverRequest{
				Email:       "guilliman@ultramar.gov",
				PhoneNumber: "+41555511111",
				ExternalID:  "Ultramar-001",
				Verifications: []VerificationRequest{
					{
						Type:  data.VerificationTypeDateOfBirth,
						Value: "1990-01-01",
					},
				},
				Wallets: []WalletRequest{
					{
						Address: "GCQFMQ7U33ICSLAVGBJNX6P66M5GGOTQWCRZ5Y3YXYK3EB3DNCWOAD5K",
					},
				},
			},
			expectedStatus: http.StatusCreated,
			assertCreatedFn: func(t *testing.T, receiverID string) {
				receiver, err := models.Receiver.Get(ctx, dbConnectionPool, receiverID)
				require.NoError(t, err)
				assert.Equal(t, "guilliman@ultramar.gov", receiver.Email)
				assert.Equal(t, "+41555511111", receiver.PhoneNumber)
				assert.Equal(t, "Ultramar-001", receiver.ExternalID)

				verifications, err := models.ReceiverVerification.GetAllByReceiverId(ctx, dbConnectionPool, receiverID)
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
