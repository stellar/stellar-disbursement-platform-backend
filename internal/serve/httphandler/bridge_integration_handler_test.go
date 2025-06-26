package httphandler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/bridge"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/middleware"
	sigMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

func Test_BridgeIntegrationHandler_Get(t *testing.T) {
	testCases := []struct {
		name             string
		prepareMocks     func(t *testing.T, mBridgeService *bridge.MockService)
		expectedStatus   int
		expectedResponse string
	}{
		{
			name:           "Bridge service not enabled",
			prepareMocks:   func(t *testing.T, mBridgeService *bridge.MockService) {},
			expectedStatus: http.StatusOK,
			expectedResponse: `{
				"status": "NOT_ENABLED"
			}`,
		},
		{
			name: "Bridge service returns error",
			prepareMocks: func(t *testing.T, mBridgeService *bridge.MockService) {
				mBridgeService.
					On("GetBridgeIntegration", mock.Anything).
					Return(nil, errors.New("service error")).
					Once()
			},
			expectedStatus:   http.StatusInternalServerError,
			expectedResponse: `{"error": "Failed to get Bridge integration status"}`,
		},
		{
			name: "🎉 successfully returns Bridge integration status",
			prepareMocks: func(t *testing.T, mBridgeService *bridge.MockService) {
				bridgeInfo := &bridge.BridgeIntegrationInfo{
					Status:     data.BridgeIntegrationStatusOptedIn,
					CustomerID: utils.StringPtr("customer-123"),
					KYCLinkInfo: &bridge.KYCLinkInfo{
						ID:         "kyc-link-123",
						Type:       bridge.KYCTypeBusiness,
						FullName:   "John Doe",
						Email:      "john.doe@example.com",
						KYCStatus:  bridge.KYCStatusApproved,
						TOSStatus:  bridge.TOSStatusApproved,
						CustomerID: "customer-123",
						KYCLink:    "https://example.com/kyc-link",
						TOSLink:    "https://example.com/tos-link",
					},
				}
				mBridgeService.
					On("GetBridgeIntegration", mock.Anything).
					Return(bridgeInfo, nil).
					Once()
			},
			expectedStatus: http.StatusOK,
			expectedResponse: `{
				"status": "OPTED_IN",
				"customer_id": "customer-123",
				"kyc_status": {
					"id": "kyc-link-123",
					"full_name": "John Doe",
					"email": "john.doe@example.com",
					"kyc_status": "approved",
					"tos_status": "approved",
					"customer_id": "customer-123",
					"kyc_link": "https://example.com/kyc-link",
					"tos_link": "https://example.com/tos-link",
					"type": "business"
				}
			}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var mBridgeService *bridge.MockService
			var handler BridgeIntegrationHandler

			if strings.Contains(tc.name, "not enabled") {
				// Test with nil BridgeService
				handler = BridgeIntegrationHandler{
					BridgeService: nil,
				}
			} else {
				mBridgeService = bridge.NewMockService(t)
				tc.prepareMocks(t, mBridgeService)
				handler = BridgeIntegrationHandler{
					BridgeService: mBridgeService,
				}
			}

			rr := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/bridge-integration", nil)
			require.NoError(t, err)

			http.HandlerFunc(handler.Get).ServeHTTP(rr, req)
			resp := rr.Result()
			defer resp.Body.Close()
			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, tc.expectedStatus, resp.StatusCode)
			assert.JSONEq(t, tc.expectedResponse, string(respBody))
		})
	}
}

func Test_BridgeIntegrationHandler_Patch_optInToBridge(t *testing.T) {
	// Sample data for the test
	redirectURL := "https://example.com/distribution-account"

	testUser := &auth.User{
		ID:        "user-123",
		Email:     "user@example.com",
		FirstName: "John",
		LastName:  "Doe",
	}

	optInOptions := bridge.OptInOptions{
		UserID:      testUser.ID,
		FullName:    "John Doe",
		Email:       "user@example.com",
		RedirectURL: redirectURL,
		KYCType:     bridge.KYCTypeBusiness,
	}
	testCases := []struct {
		name             string
		requestBody      interface{}
		prepareMocks     func(t *testing.T, mBridgeService *bridge.MockService, mAuthenticator *auth.AuthenticatorMock)
		expectedStatus   int
		expectedResponse string
	}{
		{
			name:             "Bridge service not enabled",
			requestBody:      PatchRequest{Status: data.BridgeIntegrationStatusOptedIn},
			prepareMocks:     func(t *testing.T, mBridgeService *bridge.MockService, mAuthenticator *auth.AuthenticatorMock) {},
			expectedStatus:   http.StatusBadRequest,
			expectedResponse: `{"error": "Bridge integration is not enabled"}`,
		},
		{
			name:             "invalid JSON body",
			requestBody:      "invalid json",
			prepareMocks:     func(t *testing.T, mBridgeService *bridge.MockService, mAuthenticator *auth.AuthenticatorMock) {},
			expectedStatus:   http.StatusBadRequest,
			expectedResponse: `{"error": "Invalid request body"}`,
		},
		{
			name:           "invalid status in request",
			requestBody:    PatchRequest{Status: data.BridgeIntegrationStatusNotOptedIn},
			prepareMocks:   func(t *testing.T, mBridgeService *bridge.MockService, mAuthenticator *auth.AuthenticatorMock) {},
			expectedStatus: http.StatusBadRequest,
			expectedResponse: `{
				"error": "Invalid request",
				"extras": {
					"validation_error": "invalid status NOT_OPTED_IN, must be one of [OPTED_IN READY_FOR_DEPOSIT]"
				}
			}`,
		},
		{
			name:           "invalid email format",
			requestBody:    PatchRequest{Status: data.BridgeIntegrationStatusOptedIn, Email: "invalid-email"},
			prepareMocks:   func(t *testing.T, mBridgeService *bridge.MockService, mAuthenticator *auth.AuthenticatorMock) {},
			expectedStatus: http.StatusBadRequest,
			expectedResponse: `{
				"error": "Invalid request",
				"extras": {
					"validation_error": "invalid email: the email address provided is not valid"
				}
			}`,
		},
		{
			name:           "empty full name",
			requestBody:    PatchRequest{Status: data.BridgeIntegrationStatusOptedIn, FullName: "   "},
			prepareMocks:   func(t *testing.T, mBridgeService *bridge.MockService, mAuthenticator *auth.AuthenticatorMock) {},
			expectedStatus: http.StatusBadRequest,
			expectedResponse: `{
				"error": "Invalid request",
				"extras": {
					"validation_error": "full_name cannot be empty or whitespace only"
				}
			}`,
		},
		{
			name:        "cannot retrieve user from context",
			requestBody: PatchRequest{Status: data.BridgeIntegrationStatusOptedIn},
			prepareMocks: func(t *testing.T, mBridgeService *bridge.MockService, mAuthenticator *auth.AuthenticatorMock) {
				mAuthenticator.
					On("GetUser", mock.Anything, testUser.ID).
					Return(nil, errors.New("user not found")).
					Once()
			},
			expectedStatus:   http.StatusInternalServerError,
			expectedResponse: `{"error": "Cannot retrieve user from context"}`,
		},
		{
			name:        "Bridge service already opted in error",
			requestBody: PatchRequest{Status: data.BridgeIntegrationStatusOptedIn},
			prepareMocks: func(t *testing.T, mBridgeService *bridge.MockService, mAuthenticator *auth.AuthenticatorMock) {
				mAuthenticator.
					On("GetUser", mock.Anything, testUser.ID).
					Return(testUser, nil).
					Once()

				mBridgeService.
					On("OptInToBridge", mock.Anything, optInOptions).
					Return(nil, bridge.ErrBridgeAlreadyOptedIn).
					Once()
			},
			expectedStatus:   http.StatusBadRequest,
			expectedResponse: `{"error": "Your organization has already opted into Bridge integration"}`,
		},
		{
			name:        "Bridge API error",
			requestBody: PatchRequest{Status: data.BridgeIntegrationStatusOptedIn},
			prepareMocks: func(t *testing.T, mBridgeService *bridge.MockService, mAuthenticator *auth.AuthenticatorMock) {
				mAuthenticator.
					On("GetUser", mock.Anything, testUser.ID).
					Return(testUser, nil).
					Once()

				bridgeError := bridge.BridgeErrorResponse{
					Code:    "VALIDATION_ERROR",
					Message: "Invalid request",
					Type:    "validation_error",
					Details: "Field 'customer_id' is required",
					Source: struct {
						Location string            `json:"location"`
						Key      map[string]string `json:"key,omitempty"`
					}{
						Location: "body",
						Key:      map[string]string{"customer_id": "required"},
					},
				}
				mBridgeService.
					On("OptInToBridge", mock.Anything, optInOptions).
					Return(nil, bridgeError).
					Once()
			},
			expectedStatus: http.StatusBadRequest,
			expectedResponse: `{
				"error": "Opt-in to Bridge integration failed",
				"extras": {
					"bridge_error_code": "VALIDATION_ERROR",
					"bridge_error_type": "validation_error",
					"bridge_error_details": "Field 'customer_id' is required",
					"bridge_error_source_location": "body",
					"bridge_error_source_key": {"customer_id": "required"}
				}
			}`,
		},
		{
			name:        "internal server error",
			requestBody: PatchRequest{Status: data.BridgeIntegrationStatusOptedIn},
			prepareMocks: func(t *testing.T, mBridgeService *bridge.MockService, mAuthenticator *auth.AuthenticatorMock) {
				mAuthenticator.
					On("GetUser", mock.Anything, testUser.ID).
					Return(testUser, nil).
					Once()

				mBridgeService.
					On("OptInToBridge", mock.Anything, optInOptions).
					Return(nil, errors.New("unexpected error")).
					Once()
			},
			expectedStatus:   http.StatusInternalServerError,
			expectedResponse: `{"error": "Failed to opt into Bridge integration"}`,
		},
		{
			name: "🎉 successfully opts in with custom email and name",
			requestBody: PatchRequest{
				Status:   data.BridgeIntegrationStatusOptedIn,
				Email:    "custom@example.com",
				FullName: "Custom Name",
			},
			prepareMocks: func(t *testing.T, mBridgeService *bridge.MockService, mAuthenticator *auth.AuthenticatorMock) {
				mAuthenticator.
					On("GetUser", mock.Anything, testUser.ID).
					Return(testUser, nil).
					Once()

				bridgeInfo := &bridge.BridgeIntegrationInfo{
					Status:     data.BridgeIntegrationStatusOptedIn,
					CustomerID: utils.StringPtr("customer-123"),
				}
				customOptInOptions := optInOptions
				customOptInOptions.Email = "custom@example.com"
				customOptInOptions.FullName = "Custom Name"
				mBridgeService.
					On("OptInToBridge", mock.Anything, customOptInOptions).
					Return(bridgeInfo, nil).
					Once()
			},
			expectedStatus: http.StatusOK,
			expectedResponse: `{
				"status": "OPTED_IN",
				"customer_id": "customer-123"
			}`,
		},
		{
			name:        "🎉 successfully opts in with user defaults",
			requestBody: PatchRequest{Status: data.BridgeIntegrationStatusOptedIn},
			prepareMocks: func(t *testing.T, mBridgeService *bridge.MockService, mAuthenticator *auth.AuthenticatorMock) {
				mAuthenticator.
					On("GetUser", mock.Anything, testUser.ID).
					Return(testUser, nil).
					Once()

				bridgeInfo := &bridge.BridgeIntegrationInfo{
					Status:     data.BridgeIntegrationStatusOptedIn,
					CustomerID: utils.StringPtr("customer-123"),
				}
				mBridgeService.
					On("OptInToBridge", mock.Anything, optInOptions).
					Return(bridgeInfo, nil).
					Once()
			},
			expectedStatus: http.StatusOK,
			expectedResponse: `{
				"status": "OPTED_IN",
				"customer_id": "customer-123"
			}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var mBridgeService *bridge.MockService
			var mAuthenticator *auth.AuthenticatorMock
			var handler BridgeIntegrationHandler

			if strings.Contains(tc.name, "not enabled") {
				// Test with nil BridgeService
				handler = BridgeIntegrationHandler{
					BridgeService: nil,
				}
			} else {
				mBridgeService = bridge.NewMockService(t)
				mAuthenticator = &auth.AuthenticatorMock{}
				tc.prepareMocks(t, mBridgeService, mAuthenticator)

				authManager := auth.NewAuthManager(
					auth.WithCustomAuthenticatorOption(mAuthenticator),
				)

				handler = BridgeIntegrationHandler{
					BridgeService: mBridgeService,
					AuthManager:   authManager,
				}
			}

			var bodyReader io.Reader
			if tc.requestBody != nil {
				if str, ok := tc.requestBody.(string); ok {
					bodyReader = strings.NewReader(str)
				} else {
					jsonBody, err := json.Marshal(tc.requestBody)
					require.NoError(t, err)
					bodyReader = bytes.NewReader(jsonBody)
				}
			}

			tnt := tenant.Tenant{
				ID:           "test-tenant",
				BaseURL:      utils.Ptr("https://example.com"),
				SDPUIBaseURL: utils.Ptr("https://example.com"),
			}
			ctx := tenant.SaveTenantInContext(context.Background(), &tnt)

			rr := httptest.NewRecorder()
			req, err := http.NewRequestWithContext(ctx, http.MethodPatch, "/bridge-integration", bodyReader)
			require.NoError(t, err)

			// Add user context if needed for auth
			if !strings.Contains(tc.name, "not enabled") && !strings.Contains(tc.name, "invalid JSON") {
				ctx := context.WithValue(req.Context(), middleware.UserIDContextKey, testUser.ID)
				req = req.WithContext(ctx)
			}

			http.HandlerFunc(handler.Patch).ServeHTTP(rr, req)
			resp := rr.Result()
			defer resp.Body.Close()
			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, tc.expectedStatus, resp.StatusCode)
			assert.JSONEq(t, tc.expectedResponse, string(respBody))
		})
	}
}

func Test_BridgeIntegrationHandler_Patch_createVirtualAccount(t *testing.T) {
	testUser := &auth.User{
		ID:        "user-123",
		Email:     "user@example.com",
		FirstName: "John",
		LastName:  "Doe",
	}

	testDistributionAccount := schema.TransactionAccount{
		Address: "GCKFBEIYTKP5RDBPFKWYFVQNMZ5KMGMW3RFKAWJ3CCDQPWXEMFXH7YDN",
	}

	testCases := []struct {
		name             string
		requestBody      interface{}
		prepareMocks     func(t *testing.T, mBridgeService *bridge.MockService, mAuthenticator *auth.AuthenticatorMock, mDistAccountResolver *sigMocks.MockDistributionAccountResolver)
		expectedStatus   int
		expectedResponse string
	}{
		{
			name:        "Bridge service not enabled",
			requestBody: PatchRequest{Status: data.BridgeIntegrationStatusReadyForDeposit},
			prepareMocks: func(t *testing.T, mBridgeService *bridge.MockService, mAuthenticator *auth.AuthenticatorMock, mDistAccountResolver *sigMocks.MockDistributionAccountResolver) {
			},
			expectedStatus:   http.StatusBadRequest,
			expectedResponse: `{"error": "Bridge integration is not enabled"}`,
		},
		{
			name:        "invalid JSON body",
			requestBody: "invalid json",
			prepareMocks: func(t *testing.T, mBridgeService *bridge.MockService, mAuthenticator *auth.AuthenticatorMock, mDistAccountResolver *sigMocks.MockDistributionAccountResolver) {
			},
			expectedStatus:   http.StatusBadRequest,
			expectedResponse: `{"error": "Invalid request body"}`,
		},
		{
			name:        "failed to get distribution account",
			requestBody: PatchRequest{Status: data.BridgeIntegrationStatusReadyForDeposit},
			prepareMocks: func(t *testing.T, mBridgeService *bridge.MockService, mAuthenticator *auth.AuthenticatorMock, mDistAccountResolver *sigMocks.MockDistributionAccountResolver) {
				mAuthenticator.
					On("GetUser", mock.Anything, testUser.ID).
					Return(testUser, nil).
					Once()

				mDistAccountResolver.
					On("DistributionAccountFromContext", mock.Anything).
					Return(schema.TransactionAccount{}, errors.New("distribution account error")).
					Once()
			},
			expectedStatus:   http.StatusInternalServerError,
			expectedResponse: `{"error": "Failed to get distribution account"}`,
		},
		{
			name:        "cannot retrieve user from context",
			requestBody: PatchRequest{Status: data.BridgeIntegrationStatusReadyForDeposit},
			prepareMocks: func(t *testing.T, mBridgeService *bridge.MockService, mAuthenticator *auth.AuthenticatorMock, mDistAccountResolver *sigMocks.MockDistributionAccountResolver) {
				mAuthenticator.
					On("GetUser", mock.Anything, testUser.ID).
					Return(nil, errors.New("user not found")).
					Once()
			},
			expectedStatus:   http.StatusInternalServerError,
			expectedResponse: `{"error": "Cannot retrieve user from context"}`,
		},
		{
			name:        "organization not opted in",
			requestBody: PatchRequest{Status: data.BridgeIntegrationStatusReadyForDeposit},
			prepareMocks: func(t *testing.T, mBridgeService *bridge.MockService, mAuthenticator *auth.AuthenticatorMock, mDistAccountResolver *sigMocks.MockDistributionAccountResolver) {
				mDistAccountResolver.
					On("DistributionAccountFromContext", mock.Anything).
					Return(testDistributionAccount, nil).
					Once()

				mAuthenticator.
					On("GetUser", mock.Anything, testUser.ID).
					Return(testUser, nil).
					Once()

				mBridgeService.
					On("CreateVirtualAccount", mock.Anything, "user-123", testDistributionAccount.Address).
					Return(nil, bridge.ErrBridgeNotOptedIn).
					Once()
			},
			expectedStatus:   http.StatusBadRequest,
			expectedResponse: `{"error": "Organization must opt into Bridge integration before creating a virtual account"}`,
		},
		{
			name:        "virtual account already exists",
			requestBody: PatchRequest{Status: data.BridgeIntegrationStatusReadyForDeposit},
			prepareMocks: func(t *testing.T, mBridgeService *bridge.MockService, mAuthenticator *auth.AuthenticatorMock, mDistAccountResolver *sigMocks.MockDistributionAccountResolver) {
				mDistAccountResolver.
					On("DistributionAccountFromContext", mock.Anything).
					Return(testDistributionAccount, nil).
					Once()

				mAuthenticator.
					On("GetUser", mock.Anything, testUser.ID).
					Return(testUser, nil).
					Once()

				mBridgeService.
					On("CreateVirtualAccount", mock.Anything, "user-123", testDistributionAccount.Address).
					Return(nil, bridge.ErrBridgeVirtualAccountAlreadyExists).
					Once()
			},
			expectedStatus:   http.StatusBadRequest,
			expectedResponse: `{"error": "Virtual account already exists for this organization"}`,
		},
		{
			name:        "KYC not approved",
			requestBody: PatchRequest{Status: data.BridgeIntegrationStatusReadyForDeposit},
			prepareMocks: func(t *testing.T, mBridgeService *bridge.MockService, mAuthenticator *auth.AuthenticatorMock, mDistAccountResolver *sigMocks.MockDistributionAccountResolver) {
				mDistAccountResolver.
					On("DistributionAccountFromContext", mock.Anything).
					Return(testDistributionAccount, nil).
					Once()

				mAuthenticator.
					On("GetUser", mock.Anything, testUser.ID).
					Return(testUser, nil).
					Once()

				mBridgeService.
					On("CreateVirtualAccount", mock.Anything, "user-123", testDistributionAccount.Address).
					Return(nil, bridge.ErrBridgeKYCNotApproved).
					Once()
			},
			expectedStatus:   http.StatusBadRequest,
			expectedResponse: `{"error": "KYC verification must be approved before creating a virtual account"}`,
		},
		{
			name:        "TOS not accepted",
			requestBody: PatchRequest{Status: data.BridgeIntegrationStatusReadyForDeposit},
			prepareMocks: func(t *testing.T, mBridgeService *bridge.MockService, mAuthenticator *auth.AuthenticatorMock, mDistAccountResolver *sigMocks.MockDistributionAccountResolver) {
				mDistAccountResolver.
					On("DistributionAccountFromContext", mock.Anything).
					Return(testDistributionAccount, nil).
					Once()

				mAuthenticator.
					On("GetUser", mock.Anything, testUser.ID).
					Return(testUser, nil).
					Once()

				mBridgeService.
					On("CreateVirtualAccount", mock.Anything, "user-123", testDistributionAccount.Address).
					Return(nil, bridge.ErrBridgeTOSNotAccepted).
					Once()
			},
			expectedStatus:   http.StatusBadRequest,
			expectedResponse: `{"error": "Terms of service must be accepted before creating a virtual account"}`,
		},
		{
			name:        "KYC rejected",
			requestBody: PatchRequest{Status: data.BridgeIntegrationStatusReadyForDeposit},
			prepareMocks: func(t *testing.T, mBridgeService *bridge.MockService, mAuthenticator *auth.AuthenticatorMock, mDistAccountResolver *sigMocks.MockDistributionAccountResolver) {
				mDistAccountResolver.
					On("DistributionAccountFromContext", mock.Anything).
					Return(testDistributionAccount, nil).
					Once()

				mAuthenticator.
					On("GetUser", mock.Anything, testUser.ID).
					Return(testUser, nil).
					Once()

				mBridgeService.
					On("CreateVirtualAccount", mock.Anything, "user-123", testDistributionAccount.Address).
					Return(nil, bridge.ErrBridgeKYCRejected).
					Once()
			},
			expectedStatus:   http.StatusBadRequest,
			expectedResponse: `{"error": "Cannot create virtual account because KYC verification was rejected"}`,
		},
		{
			name:        "Bridge API error",
			requestBody: PatchRequest{Status: data.BridgeIntegrationStatusReadyForDeposit},
			prepareMocks: func(t *testing.T, mBridgeService *bridge.MockService, mAuthenticator *auth.AuthenticatorMock, mDistAccountResolver *sigMocks.MockDistributionAccountResolver) {
				mDistAccountResolver.
					On("DistributionAccountFromContext", mock.Anything).
					Return(testDistributionAccount, nil).
					Once()

				mAuthenticator.
					On("GetUser", mock.Anything, testUser.ID).
					Return(testUser, nil).
					Once()

				bridgeError := bridge.BridgeErrorResponse{
					Code:    "INVALID_CUSTOMER",
					Message: "Customer not found",
					Type:    "validation_error",
				}
				mBridgeService.
					On("CreateVirtualAccount", mock.Anything, "user-123", testDistributionAccount.Address).
					Return(nil, bridgeError).
					Once()
			},
			expectedStatus: http.StatusBadRequest,
			expectedResponse: `{
				"error": "Virtual account creation failed",
				"extras": {
					"bridge_error_code": "INVALID_CUSTOMER",
					"bridge_error_type": "validation_error"
				}
			}`,
		},
		{
			name:        "internal server error",
			requestBody: PatchRequest{Status: data.BridgeIntegrationStatusReadyForDeposit},
			prepareMocks: func(t *testing.T, mBridgeService *bridge.MockService, mAuthenticator *auth.AuthenticatorMock, mDistAccountResolver *sigMocks.MockDistributionAccountResolver) {
				mDistAccountResolver.
					On("DistributionAccountFromContext", mock.Anything).
					Return(testDistributionAccount, nil).
					Once()

				mAuthenticator.
					On("GetUser", mock.Anything, testUser.ID).
					Return(testUser, nil).
					Once()

				mBridgeService.
					On("CreateVirtualAccount", mock.Anything, "user-123", testDistributionAccount.Address).
					Return(nil, errors.New("unexpected error")).
					Once()
			},
			expectedStatus:   http.StatusInternalServerError,
			expectedResponse: `{"error": "Failed to create virtual account"}`,
		},
		{
			name:        "🎉 successfully creates virtual account",
			requestBody: PatchRequest{Status: data.BridgeIntegrationStatusReadyForDeposit},
			prepareMocks: func(t *testing.T, mBridgeService *bridge.MockService, mAuthenticator *auth.AuthenticatorMock, mDistAccountResolver *sigMocks.MockDistributionAccountResolver) {
				mDistAccountResolver.
					On("DistributionAccountFromContext", mock.Anything).
					Return(testDistributionAccount, nil).
					Once()

				mAuthenticator.
					On("GetUser", mock.Anything, testUser.ID).
					Return(testUser, nil).
					Once()

				bridgeInfo := &bridge.BridgeIntegrationInfo{
					Status:                  data.BridgeIntegrationStatusReadyForDeposit,
					CustomerID:              utils.StringPtr("customer-123"),
					VirtualAccountCreatedBy: utils.StringPtr("user-123"),
					VirtualAccountDetails: &bridge.VirtualAccountInfo{
						ID:         "va-123",
						CustomerID: "customer-123",
						Status:     bridge.VirtualAccountActivated,
					},
				}
				mBridgeService.
					On("CreateVirtualAccount", mock.Anything, "user-123", testDistributionAccount.Address).
					Return(bridgeInfo, nil).
					Once()
			},
			expectedStatus: http.StatusOK,
			expectedResponse: `{
				"status": "READY_FOR_DEPOSIT",
				"customer_id": "customer-123",
				"virtual_account": {
					"id": "va-123",
					"status": "activated",
					"developer_fee_percent": "",
					"customer_id": "customer-123",
					"source_deposit_instructions": {
						"bank_beneficiary_name": "",
						"currency": "",
						"bank_name": "",
						"bank_address": "",
						"bank_account_number": "",
						"bank_routing_number": "",
						"payment_rails": null
					},
					"destination": {
						"payment_rail": "",
						"currency": "",
						"address": ""
					}
				},
				"virtual_account_created_by": "user-123"
			}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var mBridgeService *bridge.MockService
			var mAuthenticator *auth.AuthenticatorMock
			var mDistAccountResolver *sigMocks.MockDistributionAccountResolver
			var handler BridgeIntegrationHandler

			if strings.Contains(tc.name, "not enabled") {
				// Test with nil BridgeService
				handler = BridgeIntegrationHandler{
					BridgeService: nil,
				}
			} else {
				mBridgeService = bridge.NewMockService(t)
				mAuthenticator = &auth.AuthenticatorMock{}
				mDistAccountResolver = sigMocks.NewMockDistributionAccountResolver(t)
				tc.prepareMocks(t, mBridgeService, mAuthenticator, mDistAccountResolver)

				authManager := auth.NewAuthManager(
					auth.WithCustomAuthenticatorOption(mAuthenticator),
				)

				handler = BridgeIntegrationHandler{
					BridgeService:               mBridgeService,
					AuthManager:                 authManager,
					DistributionAccountResolver: mDistAccountResolver,
				}
			}

			var bodyReader io.Reader
			if tc.requestBody != nil {
				if str, ok := tc.requestBody.(string); ok {
					bodyReader = strings.NewReader(str)
				} else {
					jsonBody, err := json.Marshal(tc.requestBody)
					require.NoError(t, err)
					bodyReader = bytes.NewReader(jsonBody)
				}
			}

			tnt := tenant.Tenant{
				ID:      "test-tenant",
				BaseURL: utils.Ptr("https://example.com"),
			}
			ctx := tenant.SaveTenantInContext(context.Background(), &tnt)
			ctx = context.WithValue(ctx, middleware.UserIDContextKey, testUser.ID)

			rr := httptest.NewRecorder()
			req, err := http.NewRequestWithContext(ctx, http.MethodPatch, "/bridge-integration", bodyReader)
			require.NoError(t, err)

			http.HandlerFunc(handler.Patch).ServeHTTP(rr, req)
			resp := rr.Result()
			defer resp.Body.Close()
			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, tc.expectedStatus, resp.StatusCode)
			assert.JSONEq(t, tc.expectedResponse, string(respBody))
		})
	}
}

func Test_OptInRequest_Validate(t *testing.T) {
	testCases := []struct {
		name                string
		request             PatchRequest
		expectedErrContains string
	}{
		{
			name:                "invalid status",
			request:             PatchRequest{Status: data.BridgeIntegrationStatusNotOptedIn},
			expectedErrContains: "invalid status NOT_OPTED_IN, must be one of [OPTED_IN READY_FOR_DEPOSIT]",
		},
		{
			name:                "invalid email format",
			request:             PatchRequest{Status: data.BridgeIntegrationStatusOptedIn, Email: "invalid-email"},
			expectedErrContains: "invalid email",
		},
		{
			name:                "empty full name",
			request:             PatchRequest{Status: data.BridgeIntegrationStatusOptedIn, FullName: "   "},
			expectedErrContains: "full_name cannot be empty or whitespace only",
		},
		{
			name:    "🎉 valid request with email and full name",
			request: PatchRequest{Status: data.BridgeIntegrationStatusOptedIn, Email: "test@example.com", FullName: "John Doe"},
		},
		{
			name:    "🎉 valid request without email and full name",
			request: PatchRequest{Status: data.BridgeIntegrationStatusOptedIn},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.request.Validate()
			if tc.expectedErrContains != "" {
				assert.ErrorContains(t, err, tc.expectedErrContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_bridgeErrorToExtras(t *testing.T) {
	testCases := []struct {
		name           string
		bridgeError    bridge.BridgeErrorResponse
		expectedExtras map[string]interface{}
	}{
		{
			name: "error with all fields",
			bridgeError: bridge.BridgeErrorResponse{
				Code:    "VALIDATION_ERROR",
				Message: "Invalid request",
				Type:    "validation_error",
				Details: "Field 'customer_id' is required",
				Source: struct {
					Location string            `json:"location"`
					Key      map[string]string `json:"key,omitempty"`
				}{
					Location: "body",
					Key:      map[string]string{"customer_id": "required"},
				},
			},
			expectedExtras: map[string]interface{}{
				"bridge_error_code":            "VALIDATION_ERROR",
				"bridge_error_type":            "validation_error",
				"bridge_error_details":         "Field 'customer_id' is required",
				"bridge_error_source_location": "body",
				"bridge_error_source_key":      map[string]string{"customer_id": "required"},
			},
		},
		{
			name: "error without details and source",
			bridgeError: bridge.BridgeErrorResponse{
				Code:    "SERVER_ERROR",
				Message: "Internal error",
				Type:    "server_error",
			},
			expectedExtras: map[string]interface{}{
				"bridge_error_code": "SERVER_ERROR",
				"bridge_error_type": "server_error",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actualExtras := bridgeErrorToExtras(tc.bridgeError)
			assert.Equal(t, tc.expectedExtras, actualExtras)
		})
	}
}
