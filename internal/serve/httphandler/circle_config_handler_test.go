package httphandler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/mock"

	sigMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing/mocks"

	"github.com/go-chi/chi/v5"
	"github.com/stellar/go/keypair"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/circle"
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
	mDistributionAccountResolver := sigMocks.NewMockDistributionAccountResolver(t)
	handler := CircleConfigHandler{
		Encrypter:                   &encrypter,
		EncryptionPassphrase:        encryptionPassphrase,
		CircleClientConfigModel:     &ccm,
		DistributionAccountResolver: mDistributionAccountResolver,
	}
	r := chi.NewRouter()
	u := "/organization/circle-config"
	r.Patch(u, handler.Patch)

	validPatchRequest := PatchCircleConfigRequest{
		APIKey:   utils.StringPtr("new_api_key"),
		WalletID: utils.StringPtr("new_wallet_id"),
	}

	mCircleDistributionAccount := mDistributionAccountResolver.
		On("DistributionAccountFromContext", mock.Anything).
		Return(schema.TransactionAccount{Type: schema.DistributionAccountCircleDBVault}, nil)

	t.Run("returns bad request for invalid request body", func(t *testing.T) {
		mCircleDistributionAccount.Once()

		rr := request(t, r, http.MethodPatch, strings.NewReader("invalid json"))

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.JSONEq(t, `{"error": "Request body is not valid"}`, rr.Body.String())
	})

	t.Run("returns bad request for invalid patch request", func(t *testing.T) {
		mCircleDistributionAccount.Once()

		invalidPatchRequest := PatchCircleConfigRequest{}
		body, err := json.Marshal(invalidPatchRequest)
		require.NoError(t, err)

		rr := request(t, r, http.MethodPatch, strings.NewReader(string(body)))

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.JSONEq(t, `{"error":"Request body is not valid", "extras":{"validation_error":"wallet_id or api_key must be provided"}}`, rr.Body.String())
	})

	t.Run("returns bad request if distribution account type is not Circle", func(t *testing.T) {
		mDistributionAccountResolver.
			On("DistributionAccountFromContext", mock.Anything).
			Return(schema.TransactionAccount{Type: schema.DistributionAccountStellarEnv}, nil).
			Once()

		body, err := json.Marshal(validPatchRequest)
		require.NoError(t, err)

		rr := request(t, r, http.MethodPatch, strings.NewReader(string(body)))

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.JSONEq(t, `{"error": "This endpoint is only available for tenants using CIRCLE"}`, rr.Body.String())
	})

	t.Run("updates Circle configuration successfully", func(t *testing.T) {
		mCircleDistributionAccount.Once()

		body, err := json.Marshal(validPatchRequest)
		require.NoError(t, err)

		rr := request(t, r, http.MethodPatch, strings.NewReader(string(body)))
		assert.Equal(t, http.StatusOK, rr.Code)

		// Check the updated config in the database
		config, err := ccm.Get(context.Background())
		require.NoError(t, err)
		require.NotNil(t, config)
		assert.Equal(t, "new_wallet_id", *config.WalletID)

		decryptedAPIKey, err := encrypter.Decrypt(*config.EncryptedAPIKey, encryptionPassphrase)
		assert.NoError(t, err)
		assert.Equal(t, "new_api_key", decryptedAPIKey)
		assert.Equal(t, encryptionPublicKey, *config.EncrypterPublicKey)
	})
}

func request(t *testing.T, r *chi.Mux, httpMethod string, body io.Reader) *httptest.ResponseRecorder {
	t.Helper()

	req, err := http.NewRequest(httpMethod, "/organization/circle-config", body)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	return rr
}
