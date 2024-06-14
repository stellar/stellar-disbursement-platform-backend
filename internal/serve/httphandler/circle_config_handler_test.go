package httphandler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stellar/go/keypair"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/circle"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/testutils"
	sigMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

func TestCircleConfigHandler_Patch(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	kp := keypair.MustRandom()
	encryptionPassphrase := kp.Seed()
	encryptionPublicKey := kp.Address()

	ccm := circle.ClientConfigModel{DBConnectionPool: dbConnectionPool}
	encrypter := utils.DefaultPrivateKeyEncrypter{}

	validPatchRequest := PatchCircleConfigRequest{
		APIKey:   utils.StringPtr("new_api_key"),
		WalletID: utils.StringPtr("new_wallet_id"),
	}
	validRequestBody, err := json.Marshal(validPatchRequest)
	require.NoError(t, err)

	invalidRequestBody, err := json.Marshal(PatchCircleConfigRequest{})
	require.NoError(t, err)

	testCases := []struct {
		name           string
		prepareMocksFn func(t *testing.T, mDistributionAccountResolver *sigMocks.MockDistributionAccountResolver)
		requestBody    string
		statusCode     int
		assertions     func(t *testing.T, rr *httptest.ResponseRecorder)
	}{
		{
			name: "returns bad request for invalid request body",
			prepareMocksFn: func(t *testing.T, m *sigMocks.MockDistributionAccountResolver) {
				t.Helper()
				m.
					On("DistributionAccountFromContext", mock.Anything).
					Return(schema.TransactionAccount{Type: schema.DistributionAccountCircleDBVault}, nil).
					Once()
			},
			requestBody: "invalid json",
			statusCode:  http.StatusBadRequest,
			assertions: func(t *testing.T, rr *httptest.ResponseRecorder) {
				t.Helper()

				assert.JSONEq(t, `{"error": "Request body is not valid"}`, rr.Body.String())
			},
		},
		{
			name: "returns bad request for invalid patch request",
			prepareMocksFn: func(t *testing.T, m *sigMocks.MockDistributionAccountResolver) {
				t.Helper()
				m.
					On("DistributionAccountFromContext", mock.Anything).
					Return(schema.TransactionAccount{Type: schema.DistributionAccountCircleDBVault}, nil).
					Once()
			},
			requestBody: string(invalidRequestBody),
			statusCode:  http.StatusBadRequest,
			assertions: func(t *testing.T, rr *httptest.ResponseRecorder) {
				t.Helper()

				assert.JSONEq(t, `{"error":"Request body is not valid", "extras":{"validation_error":"wallet_id or api_key must be provided"}}`, rr.Body.String())
			},
		},
		{
			name: "returns bad request if distribution account type is not Circle",
			prepareMocksFn: func(t *testing.T, m *sigMocks.MockDistributionAccountResolver) {
				t.Helper()
				m.
					On("DistributionAccountFromContext", mock.Anything).
					Return(schema.TransactionAccount{Type: schema.DistributionAccountStellarEnv}, nil).
					Once()
			},
			requestBody: string(validRequestBody),
			statusCode:  http.StatusBadRequest,
			assertions: func(t *testing.T, rr *httptest.ResponseRecorder) {
				t.Helper()

				assert.JSONEq(t, `{"error": "This endpoint is only available for tenants using CIRCLE"}`, rr.Body.String())
			},
		},
		{
			name: "updates Circle configuration successfully",
			prepareMocksFn: func(t *testing.T, m *sigMocks.MockDistributionAccountResolver) {
				t.Helper()
				m.
					On("DistributionAccountFromContext", mock.Anything).
					Return(schema.TransactionAccount{Type: schema.DistributionAccountCircleDBVault}, nil).
					Once()
			},
			requestBody: string(validRequestBody),
			statusCode:  http.StatusOK,
			assertions: func(t *testing.T, rr *httptest.ResponseRecorder) {
				t.Helper()

				// Check the updated config in the database
				config, err := ccm.Get(context.Background())
				require.NoError(t, err)
				require.NotNil(t, config)
				assert.Equal(t, "new_wallet_id", *config.WalletID)

				decryptedAPIKey, err := encrypter.Decrypt(*config.EncryptedAPIKey, encryptionPassphrase)
				assert.NoError(t, err)
				assert.Equal(t, "new_api_key", decryptedAPIKey)
				assert.Equal(t, encryptionPublicKey, *config.EncrypterPublicKey)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mDistributionAccountResolver := sigMocks.NewMockDistributionAccountResolver(t)
			handler := CircleConfigHandler{
				Encrypter:                   &encrypter,
				EncryptionPassphrase:        encryptionPassphrase,
				CircleClientConfigModel:     &ccm,
				DistributionAccountResolver: mDistributionAccountResolver,
			}

			if tc.prepareMocksFn != nil {
				tc.prepareMocksFn(t, mDistributionAccountResolver)
			}

			r := chi.NewRouter()
			url := "/organization/circle-config"
			r.Patch(url, handler.Patch)

			rr := testutils.Request(t, r, url, http.MethodPatch, strings.NewReader(tc.requestBody))
			assert.Equal(t, tc.statusCode, rr.Code)
			tc.assertions(t, rr)
		})
	}
}
