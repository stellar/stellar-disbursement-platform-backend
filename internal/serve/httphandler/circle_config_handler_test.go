package httphandler

import (
	"context"
	"encoding/json"
	"fmt"
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
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/testutils"
	sigMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

func TestCircleConfigHandler_Patch(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	// Creates a tenant and inserts it in the context
	tnt := tenant.Tenant{ID: "test-tenant-id"}
	ctx := tenant.SaveTenantInContext(context.Background(), &tnt)

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
		prepareMocksFn func(t *testing.T, mDistributionAccountResolver *sigMocks.MockDistributionAccountResolver, mCircleClient *circle.MockClient, mTenantManager *tenant.TenantManagerMock)
		requestBody    string
		statusCode     int
		assertions     func(t *testing.T, rr *httptest.ResponseRecorder)
	}{
		{
			name: "returns bad request if distribution account type is not Circle",
			prepareMocksFn: func(t *testing.T, mDistAccResolver *sigMocks.MockDistributionAccountResolver, mCircleClient *circle.MockClient, mTenantManager *tenant.TenantManagerMock) {
				t.Helper()
				mDistAccResolver.
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
			name: "returns bad request for invalid request json body",
			prepareMocksFn: func(t *testing.T, mDistAccResolver *sigMocks.MockDistributionAccountResolver, mCircleClient *circle.MockClient, mTenantManager *tenant.TenantManagerMock) {
				t.Helper()
				mDistAccResolver.
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
			name: "returns bad request for invalid patch request data",
			prepareMocksFn: func(t *testing.T, mDistAccResolver *sigMocks.MockDistributionAccountResolver, mCircleClient *circle.MockClient, mTenantManager *tenant.TenantManagerMock) {
				t.Helper()
				mDistAccResolver.
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
			name: "returns an error if Circle client ping fails",
			prepareMocksFn: func(t *testing.T, mDistAccResolver *sigMocks.MockDistributionAccountResolver, mCircleClient *circle.MockClient, mTenantManager *tenant.TenantManagerMock) {
				t.Helper()
				mDistAccResolver.
					On("DistributionAccountFromContext", mock.Anything).
					Return(schema.TransactionAccount{Type: schema.DistributionAccountCircleDBVault}, nil).
					Once()

				mCircleClient.
					On("Ping", mock.Anything).
					Return(false, nil).
					Once()
			},
			requestBody: string(validRequestBody),
			statusCode:  http.StatusBadRequest,
			assertions: func(t *testing.T, rr *httptest.ResponseRecorder) {
				t.Helper()

				assert.JSONEq(t, `{"error":"Failed to ping, please make sure that the provided API Key is correct."}`, rr.Body.String())
			},
		},
		{
			name: "🎉 successfully updates Circle configuration and the tenant DistributionAccountStatus",
			prepareMocksFn: func(t *testing.T, mDistAccResolver *sigMocks.MockDistributionAccountResolver, mCircleClient *circle.MockClient, mTenantManager *tenant.TenantManagerMock) {
				t.Helper()
				mDistAccResolver.
					On("DistributionAccountFromContext", mock.Anything).
					Return(schema.TransactionAccount{Type: schema.DistributionAccountCircleDBVault}, nil).
					Once()

				mCircleClient.
					On("Ping", mock.Anything).
					Return(true, nil).
					Once()
				mCircleClient.
					On("GetWalletByID", mock.Anything, "new_wallet_id").
					Return(&circle.Wallet{WalletID: "new_wallet_id"}, nil).
					Once()

				mTenantManager.
					On("UpdateTenantConfig", mock.Anything, &tenant.TenantUpdate{
						ID:                        "test-tenant-id",
						DistributionAccountStatus: schema.AccountStatusActive,
					}).
					Return(&tenant.Tenant{}, nil).
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
			handler := CircleConfigHandler{
				Encrypter:               &encrypter,
				EncryptionPassphrase:    encryptionPassphrase,
				CircleClientConfigModel: &ccm,
			}

			if tc.prepareMocksFn != nil {
				mDistributionAccountResolver := sigMocks.NewMockDistributionAccountResolver(t)
				mCircleClient := circle.NewMockClient(t)
				mTenantManager := tenant.NewTenantManagerMock(t)
				tc.prepareMocksFn(t, mDistributionAccountResolver, mCircleClient, mTenantManager)

				handler.DistributionAccountResolver = mDistributionAccountResolver
				handler.CircleFactory = func(clientOpts circle.ClientOptions) circle.ClientInterface {
					return mCircleClient
				}
				handler.TenantManager = mTenantManager
			}

			r := chi.NewRouter()
			url := "/organization/circle-config"
			r.Patch(url, handler.Patch)

			rr := testutils.Request(t, ctx, r, url, http.MethodPatch, strings.NewReader(tc.requestBody))
			assert.Equal(t, tc.statusCode, rr.Code)
			tc.assertions(t, rr)
		})
	}
}

func Test_CircleConfigHandler_validateConfigWithCircle(t *testing.T) {
	ctx := context.Background()

	encryptionPassphrase := "SCW5I426WV3IDTLSTLQEHC6BMXWI2Z6C4DXAOC4ZA2EIHTAZQ6VD3JI6"
	newAPIKey := "new-api-key"
	newWalletID := "new-wallet-id"

	testCases := []struct {
		name           string
		patchRequest   PatchCircleConfigRequest
		prepareMocksFn func(t *testing.T, mEncrypter *utils.PrivateKeyEncrypterMock, mCircleClientConfigModel *circle.MockClientConfigModel, mCircleClient *circle.MockClient)
		expectedError  *httperror.HTTPError
	}{
		{
			name:          "returns error if request body is not valid",
			patchRequest:  PatchCircleConfigRequest{},
			expectedError: httperror.BadRequest("Request body is not valid", fmt.Errorf("wallet_id or api_key must be provided"), nil),
		},
		{
			name:         "returns error if CircleClientConfigModel.Get returns error",
			patchRequest: PatchCircleConfigRequest{APIKey: &newAPIKey, WalletID: nil},
			prepareMocksFn: func(t *testing.T, mEncrypter *utils.PrivateKeyEncrypterMock, mCircleClientConfigModel *circle.MockClientConfigModel, mCircleClient *circle.MockClient) {
				mCircleClientConfigModel.
					On("Get", ctx).
					Return(nil, fmt.Errorf("get error")).
					Once()
			},
			expectedError: httperror.InternalError(ctx, "Cannot retrieve the existing Circle configuration", fmt.Errorf("get error"), nil),
		},
		{
			name:         "returns error if CircleClientConfigModel.Get returns nil",
			patchRequest: PatchCircleConfigRequest{APIKey: &newAPIKey, WalletID: nil},
			prepareMocksFn: func(t *testing.T, mEncrypter *utils.PrivateKeyEncrypterMock, mCircleClientConfigModel *circle.MockClientConfigModel, mCircleClient *circle.MockClient) {
				mCircleClientConfigModel.
					On("Get", ctx).
					Return(nil, nil).
					Once()
			},
			expectedError: httperror.BadRequest("You must provide both the Circle walletID and Circle APIKey during the first configuration", nil, nil),
		},
		{
			name:         "returns an error if the existing API Key cannot be decrypted",
			patchRequest: PatchCircleConfigRequest{WalletID: &newWalletID},
			prepareMocksFn: func(t *testing.T, mEncrypter *utils.PrivateKeyEncrypterMock, mCircleClientConfigModel *circle.MockClientConfigModel, mCircleClient *circle.MockClient) {
				mCircleClientConfigModel.
					On("Get", ctx).
					Return(&circle.ClientConfig{EncryptedAPIKey: utils.StringPtr("encrypted-api-key")}, nil).
					Once()
				mEncrypter.
					On("Decrypt", "encrypted-api-key", encryptionPassphrase).
					Return("", fmt.Errorf("decrypt error")).
					Once()
			},
			expectedError: httperror.InternalError(ctx, "Cannot decrypt the API key", fmt.Errorf("decrypt error"), nil),
		},
		{
			name:         "returns an error if circleClient.Ping returns an error",
			patchRequest: PatchCircleConfigRequest{APIKey: &newAPIKey, WalletID: &newWalletID},
			prepareMocksFn: func(t *testing.T, mEncrypter *utils.PrivateKeyEncrypterMock, mCircleClientConfigModel *circle.MockClientConfigModel, mCircleClient *circle.MockClient) {
				mCircleClient.
					On("Ping", ctx).
					Return(false, fmt.Errorf("ping error")).
					Once()
			},
			expectedError: wrapCircleError(ctx, fmt.Errorf("ping error")),
		},
		{
			name:         "returns an error if circleClient.Ping returns 'false'",
			patchRequest: PatchCircleConfigRequest{APIKey: &newAPIKey, WalletID: &newWalletID},
			prepareMocksFn: func(t *testing.T, mEncrypter *utils.PrivateKeyEncrypterMock, mCircleClientConfigModel *circle.MockClientConfigModel, mCircleClient *circle.MockClient) {
				mCircleClient.
					On("Ping", ctx).
					Return(false, nil).
					Once()
			},
			expectedError: httperror.BadRequest("Failed to ping, please make sure that the provided API Key is correct.", nil, nil),
		},
		{
			name:         "returns an error if circleClient.GetWalletByID returns an error",
			patchRequest: PatchCircleConfigRequest{APIKey: &newAPIKey, WalletID: &newWalletID},
			prepareMocksFn: func(t *testing.T, mEncrypter *utils.PrivateKeyEncrypterMock, mCircleClientConfigModel *circle.MockClientConfigModel, mCircleClient *circle.MockClient) {
				mCircleClient.
					On("Ping", ctx).
					Return(true, nil).
					Once()
				mCircleClient.
					On("GetWalletByID", ctx, newWalletID).
					Return(nil, fmt.Errorf("get wallet error")).
					Once()
			},
			expectedError: wrapCircleError(ctx, fmt.Errorf("get wallet error")),
		},
		{
			name:         "🎉 successfully validate for a new pair of apiKey and walletID",
			patchRequest: PatchCircleConfigRequest{APIKey: &newAPIKey, WalletID: &newWalletID},
			prepareMocksFn: func(t *testing.T, mEncrypter *utils.PrivateKeyEncrypterMock, mCircleClientConfigModel *circle.MockClientConfigModel, mCircleClient *circle.MockClient) {
				mCircleClient.
					On("Ping", ctx).
					Return(true, nil).
					Once()
				mCircleClient.
					On("GetWalletByID", ctx, newWalletID).
					Return(&circle.Wallet{WalletID: newWalletID}, nil).
					Once()
			},
			expectedError: nil,
		},
		{
			name:         "🎉 successfully validate for a new apiKey",
			patchRequest: PatchCircleConfigRequest{APIKey: &newAPIKey, WalletID: nil},
			prepareMocksFn: func(t *testing.T, mEncrypter *utils.PrivateKeyEncrypterMock, mCircleClientConfigModel *circle.MockClientConfigModel, mCircleClient *circle.MockClient) {
				mCircleClientConfigModel.
					On("Get", ctx).
					Return(&circle.ClientConfig{EncryptedAPIKey: utils.StringPtr("encrypted-api-key")}, nil).
					Once()
				mCircleClient.
					On("Ping", ctx).
					Return(true, nil).
					Once()
			},
			expectedError: nil,
		},
		{
			name:         "🎉 successfully validate for a new walletID",
			patchRequest: PatchCircleConfigRequest{APIKey: nil, WalletID: &newWalletID},
			prepareMocksFn: func(t *testing.T, mEncrypter *utils.PrivateKeyEncrypterMock, mCircleClientConfigModel *circle.MockClientConfigModel, mCircleClient *circle.MockClient) {
				mCircleClientConfigModel.
					On("Get", ctx).
					Return(&circle.ClientConfig{EncryptedAPIKey: utils.StringPtr("encrypted-api-key")}, nil).
					Once()
				mEncrypter.
					On("Decrypt", "encrypted-api-key", encryptionPassphrase).
					Return("api-key", nil).
					Once()
				mCircleClient.
					On("GetWalletByID", ctx, newWalletID).
					Return(&circle.Wallet{WalletID: newWalletID}, nil).
					Once()
			},
			expectedError: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			handler := CircleConfigHandler{EncryptionPassphrase: encryptionPassphrase}

			if tc.prepareMocksFn != nil {
				mEncrypter := utils.NewPrivateKeyEncrypterMock(t)
				mCircleClientConfigModel := circle.NewMockClientConfigModel(t)
				mCircleClient := circle.NewMockClient(t)
				tc.prepareMocksFn(t, mEncrypter, mCircleClientConfigModel, mCircleClient)

				handler.Encrypter = mEncrypter
				handler.CircleClientConfigModel = mCircleClientConfigModel
				handler.CircleFactory = func(clientOpts circle.ClientOptions) circle.ClientInterface {
					return mCircleClient
				}
			}

			err := handler.validateConfigWithCircle(ctx, tc.patchRequest)
			if tc.expectedError != nil {
				assert.Equal(t, tc.expectedError, err)
			} else {
				assert.Nil(t, err, "expected no error")
			}
		})
	}
}
