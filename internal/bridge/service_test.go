package bridge

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/assets"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/mocks"
	sigMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

func Test_ServiceOptions_Validate(t *testing.T) {
	models := &data.Models{}

	testCases := []struct {
		name                string
		opts                ServiceOptions
		expectedErrContains string
	}{
		{
			name:                "BaseURL validation fails",
			opts:                ServiceOptions{},
			expectedErrContains: "baseURL is required",
		},
		{
			name:                "APIKey validation fails",
			opts:                ServiceOptions{BaseURL: "https://api.bridge.example.com"},
			expectedErrContains: "apiKey is required",
		},
		{
			name:                "Models validation fails",
			opts:                ServiceOptions{BaseURL: "https://api.bridge.example.com", APIKey: "test-key"},
			expectedErrContains: "models is required",
		},
		{
			name:                "DistributionAccountResolver validation fails",
			opts:                ServiceOptions{BaseURL: "https://api.bridge.example.com", APIKey: "test-key", Models: models},
			expectedErrContains: "distributionAccountResolver is required",
		},
		{
			name: "DistributionAccountService validation fails",
			opts: ServiceOptions{
				BaseURL:                     "https://api.bridge.example.com",
				APIKey:                      "test-key",
				Models:                      models,
				DistributionAccountResolver: sigMocks.NewMockDistributionAccountResolver(t),
			},
			expectedErrContains: "distributionAccountService is required",
		},
		{
			name: "NetworkType validation fails",
			opts: ServiceOptions{
				BaseURL:                     "https://api.bridge.example.com",
				APIKey:                      "test-key",
				Models:                      models,
				DistributionAccountResolver: sigMocks.NewMockDistributionAccountResolver(t),
				DistributionAccountService:  mocks.NewMockDistributionAccountService(t),
				NetworkType:                 "",
			},
			expectedErrContains: "validating NetworkType",
		},
		{
			name: "🎉 successfully validates options",
			opts: ServiceOptions{
				BaseURL:                     "https://api.bridge.example.com",
				APIKey:                      "test-api-key",
				Models:                      models,
				DistributionAccountResolver: sigMocks.NewMockDistributionAccountResolver(t),
				DistributionAccountService:  mocks.NewMockDistributionAccountService(t),
				NetworkType:                 utils.TestnetNetworkType,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.opts.Validate()
			if tc.expectedErrContains != "" {
				assert.ErrorContains(t, err, tc.expectedErrContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_Service_OptInToBridge(t *testing.T) {
	models := data.SetupModels(t)
	dbcp := models.DBConnectionPool
	ctx := context.Background()

	// Sample data for the test
	fullName := "John Doe"
	email := "john@example.com"
	redirectURL := "https://example.com/distribution-account"

	t.Run("missing userID", func(t *testing.T) {
		data.CleanupBridgeIntegration(t, ctx, dbcp)
		mockClient := NewMockClient(t)
		svc := createService(t, mockClient, models)

		result, err := svc.OptInToBridge(ctx, OptInOptions{
			UserID:      "",
			FullName:    fullName,
			Email:       email,
			RedirectURL: redirectURL,
			KYCType:     CustomerTypeBusiness,
		})
		assert.EqualError(t, err, "validating opt-in options: userID is required to opt into Bridge integration")
		assert.Nil(t, result)
	})

	t.Run("missing fullName", func(t *testing.T) {
		data.CleanupBridgeIntegration(t, ctx, dbcp)
		mockClient := NewMockClient(t)
		svc := createService(t, mockClient, models)

		result, err := svc.OptInToBridge(ctx, OptInOptions{
			UserID:      "user-123",
			FullName:    "",
			Email:       email,
			RedirectURL: redirectURL,
			KYCType:     CustomerTypeBusiness,
		})
		assert.EqualError(t, err, "validating opt-in options: fullName is required to opt into Bridge integration")
		assert.Nil(t, result)
	})

	t.Run("missing redirectURL", func(t *testing.T) {
		data.CleanupBridgeIntegration(t, ctx, dbcp)
		mockClient := NewMockClient(t)
		svc := createService(t, mockClient, models)
		result, err := svc.OptInToBridge(ctx, OptInOptions{
			UserID:      "user-123",
			FullName:    fullName,
			Email:       email,
			RedirectURL: "",
			KYCType:     CustomerTypeBusiness,
		})
		assert.EqualError(t, err, "validating opt-in options: redirectURL is required to opt into Bridge integration")
		assert.Nil(t, result)
	})

	t.Run("missing email", func(t *testing.T) {
		data.CleanupBridgeIntegration(t, ctx, dbcp)
		mockClient := NewMockClient(t)
		svc := createService(t, mockClient, models)

		result, err := svc.OptInToBridge(ctx, OptInOptions{
			UserID:      "user-123",
			FullName:    fullName,
			Email:       "",
			RedirectURL: redirectURL,
			KYCType:     CustomerTypeBusiness,
		})
		assert.EqualError(t, err, "validating opt-in options: email is required to opt into Bridge integration")
		assert.Nil(t, result)
	})

	t.Run("missing CustomerType", func(t *testing.T) {
		data.CleanupBridgeIntegration(t, ctx, dbcp)
		mockClient := NewMockClient(t)
		svc := createService(t, mockClient, models)
		result, err := svc.OptInToBridge(ctx, OptInOptions{
			UserID:      "user-123",
			FullName:    fullName,
			Email:       email,
			RedirectURL: redirectURL,
			KYCType:     "",
		})
		assert.EqualError(t, err, "validating opt-in options: CustomerType must be either 'individual' or 'business'")
		assert.Nil(t, result)
	})

	t.Run("already opted in", func(t *testing.T) {
		data.CleanupBridgeIntegration(t, ctx, dbcp)
		mockClient := NewMockClient(t)
		svc := createService(t, mockClient, models)

		// Insert existing integration
		_, err := models.BridgeIntegration.Insert(ctx, models.DBConnectionPool, data.BridgeIntegrationInsert{
			KYCLinkID:  utils.StringPtr("existing-kyc-id"),
			CustomerID: "existing-customer-id",
			OptedInBy:  "existing-user",
		})
		require.NoError(t, err)

		result, err := svc.OptInToBridge(ctx, OptInOptions{
			UserID:      "user-123",
			FullName:    fullName,
			Email:       email,
			RedirectURL: redirectURL,
			KYCType:     CustomerTypeBusiness,
		})
		assert.EqualError(t, err, ErrBridgeAlreadyOptedIn.Error())
		assert.Nil(t, result)
	})

	t.Run("Bridge API error", func(t *testing.T) {
		data.CleanupBridgeIntegration(t, ctx, dbcp)
		mockClient := NewMockClient(t)
		bridgeErr := errors.New("bridge API error")
		mockClient.
			On("PostKYCLink", ctx, KYCLinkRequest{
				FullName:    fullName,
				Email:       email,
				Type:        CustomerTypeBusiness,
				RedirectURI: redirectURL,
			}).
			Return(nil, bridgeErr).
			Once()

		svc := createService(t, mockClient, models)

		result, err := svc.OptInToBridge(ctx, OptInOptions{
			UserID:      "user-123",
			FullName:    fullName,
			Email:       email,
			RedirectURL: redirectURL,
			KYCType:     CustomerTypeBusiness,
		})
		assert.EqualError(t, err, "creating KYC link via Bridge API: bridge API error")
		assert.Nil(t, result)
	})

	t.Run("USDC trustline validation fails - distribution account resolver error", func(t *testing.T) {
		data.CleanupBridgeIntegration(t, ctx, dbcp)
		mockClient := NewMockClient(t)
		svc := createService(t, mockClient, models)

		// Create service with failing distribution account resolver
		mockDistAccountResolver := sigMocks.NewMockDistributionAccountResolver(t)
		mockDistAccountResolver.
			On("DistributionAccountFromContext", mock.Anything).
			Return(schema.TransactionAccount{}, errors.New("failed to get distribution account")).
			Once()
		svc.distributionAccountResolver = mockDistAccountResolver

		result, err := svc.OptInToBridge(ctx, OptInOptions{
			UserID:      "user-123",
			FullName:    fullName,
			Email:       email,
			RedirectURL: redirectURL,
			KYCType:     CustomerTypeBusiness,
		})
		assert.ErrorContains(t, err, "validating USDC trustline: getting distribution account from context: failed to get distribution account")
		assert.Nil(t, result)
	})

	t.Run("USDC trustline validation fails - no trustline", func(t *testing.T) {
		data.CleanupBridgeIntegration(t, ctx, dbcp)
		mockClient := NewMockClient(t)
		svc := createService(t, mockClient, models)

		// Create service with no USDC trustline
		mockDistAccountService := mocks.NewMockDistributionAccountService(t)
		mockDistAccountService.
			On("GetBalance", mock.Anything, mock.Anything, assets.USDCAssetTestnet).
			Return(decimal.Zero, errors.New("no trustline found")).
			Once()
		svc.distributionAccountService = mockDistAccountService

		result, err := svc.OptInToBridge(ctx, OptInOptions{
			UserID:      "user-123",
			FullName:    fullName,
			Email:       email,
			RedirectURL: redirectURL,
			KYCType:     CustomerTypeBusiness,
		})
		assert.ErrorIs(t, err, ErrBridgeUSDCTrustlineRequired)
		assert.ErrorContains(t, err, "distribution account must have a USDC trustline to opt into Bridge integration")
		assert.Nil(t, result)
	})

	t.Run("USDC trustline validation succeeds with pubnet asset", func(t *testing.T) {
		data.CleanupBridgeIntegration(t, ctx, dbcp)
		mockClient := NewMockClient(t)

		// Create service with pubnet network type
		mockDistAccountResolver := sigMocks.NewMockDistributionAccountResolver(t)
		mockDistAccountService := mocks.NewMockDistributionAccountService(t)
		testDistAccount := schema.TransactionAccount{
			Address: "GCKFBEIYTKP5RDBPFKWYFVQNMZ5KMGMW3RFKAWJ3CCDQPWXEMFXH7YDN",
		}

		mockDistAccountResolver.
			On("DistributionAccountFromContext", mock.Anything).
			Return(testDistAccount, nil).
			Once()

		mockDistAccountService.
			On("GetBalance", mock.Anything, &testDistAccount, assets.USDCAssetPubnet).
			Return(decimal.NewFromFloat(50.0), nil).
			Once()

		kycResponse := &KYCLinkInfo{
			ID:         "kyc-link-123",
			CustomerID: "customer-123",
			FullName:   fullName,
			Email:      email,
			Type:       CustomerTypeBusiness,
			KYCStatus:  KYCStatusNotStarted,
			TOSStatus:  TOSStatusPending,
		}

		mockClient.
			On("PostKYCLink", ctx, KYCLinkRequest{
				FullName:    fullName,
				Email:       email,
				Type:        CustomerTypeBusiness,
				RedirectURI: redirectURL,
			}).
			Return(kycResponse, nil).
			Once()

		svc := &Service{
			client:                      mockClient,
			baseURL:                     "https://api.bridge.example.com",
			apiKey:                      "test-api-key",
			models:                      models,
			distributionAccountResolver: mockDistAccountResolver,
			distributionAccountService:  mockDistAccountService,
			networkType:                 utils.PubnetNetworkType,
		}

		result, err := svc.OptInToBridge(ctx, OptInOptions{
			UserID:      "user-123",
			FullName:    fullName,
			Email:       email,
			RedirectURL: redirectURL,
			KYCType:     CustomerTypeBusiness,
		})
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, data.BridgeIntegrationStatusOptedIn, result.Status)
		assert.Equal(t, "customer-123", *result.CustomerID)
		assert.Equal(t, "user-123", *result.OptedInBy)
		assert.NotNil(t, result.OptedInAt)
		assert.Equal(t, kycResponse, result.KYCLinkInfo)
	})

	t.Run("🎉 successfully opts in to Bridge", func(t *testing.T) {
		data.CleanupBridgeIntegration(t, ctx, dbcp)
		mockClient := NewMockClient(t)
		kycResponse := &KYCLinkInfo{
			ID:         "kyc-link-123",
			CustomerID: "customer-123",
			FullName:   fullName,
			Email:      email,
			Type:       CustomerTypeBusiness,
			KYCStatus:  KYCStatusNotStarted,
			TOSStatus:  TOSStatusPending,
		}

		mockClient.
			On("PostKYCLink", ctx, KYCLinkRequest{
				FullName:    fullName,
				Email:       email,
				Type:        CustomerTypeBusiness,
				RedirectURI: redirectURL,
			}).
			Return(kycResponse, nil).
			Once()

		svc := createService(t, mockClient, models)

		result, err := svc.OptInToBridge(ctx, OptInOptions{
			UserID:      "user-123",
			FullName:    fullName,
			Email:       email,
			RedirectURL: redirectURL,
			KYCType:     CustomerTypeBusiness,
		})
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, data.BridgeIntegrationStatusOptedIn, result.Status)
		assert.Equal(t, "customer-123", *result.CustomerID)
		assert.Equal(t, "user-123", *result.OptedInBy)
		assert.NotNil(t, result.OptedInAt)
		assert.Equal(t, kycResponse, result.KYCLinkInfo)
	})
}

func Test_Service_GetBridgeIntegration(t *testing.T) {
	models := data.SetupModels(t)
	dbcp := models.DBConnectionPool
	ctx := context.Background()

	t.Run("no integration record exists", func(t *testing.T) {
		data.CleanupBridgeIntegration(t, ctx, dbcp)
		mockClient := NewMockClient(t)
		svc := createService(t, mockClient, models)

		result, err := svc.GetBridgeIntegration(ctx)
		assert.NoError(t, err)
		assert.Equal(t, data.BridgeIntegrationStatusNotOptedIn, result.Status)
		assert.Nil(t, result.CustomerID)
		assert.Nil(t, result.KYCLinkInfo)
		assert.Nil(t, result.VirtualAccountDetails)
	})

	t.Run("integration exists with KYC info", func(t *testing.T) {
		data.CleanupBridgeIntegration(t, ctx, dbcp)
		mockClient := NewMockClient(t)

		customerResponse := &CustomerInfo{
			ID:        "customer-123",
			Status:    CustomerStatusActive,
			Email:     "john.doe@example.com",
			FirstName: "John",
			LastName:  "Doe",
			Type:      CustomerTypeBusiness,
		}

		expectedKYCInfo := &KYCLinkInfo{
			CustomerID: "customer-123",
			Email:      "john.doe@example.com",
			FullName:   "John Doe",
			Type:       CustomerTypeBusiness,
			KYCStatus:  KYCStatusApproved,
			TOSStatus:  TOSStatusApproved,
		}

		// Insert integration
		integration, err := models.BridgeIntegration.Insert(ctx, models.DBConnectionPool, data.BridgeIntegrationInsert{
			KYCLinkID:  utils.StringPtr("kyc-link-123"),
			CustomerID: "customer-123",
			OptedInBy:  "user-123",
		})
		require.NoError(t, err)

		mockClient.
			On("GetCustomer", ctx, "customer-123").
			Return(customerResponse, nil).
			Once()

		svc := createService(t, mockClient, models)

		result, err := svc.GetBridgeIntegration(ctx)
		assert.NoError(t, err)
		assert.Equal(t, integration.Status, result.Status)
		assert.Equal(t, integration.CustomerID, result.CustomerID)
		assert.Equal(t, integration.OptedInBy, result.OptedInBy)
		assert.Equal(t, expectedKYCInfo, result.KYCLinkInfo)
	})

	t.Run("integration exists with virtual account", func(t *testing.T) {
		data.CleanupBridgeIntegration(t, ctx, dbcp)
		mockClient := NewMockClient(t)
		vaResponse := &VirtualAccountInfo{
			ID:         "va-123",
			CustomerID: "customer-123",
			Status:     VirtualAccountActivated,
		}

		// Create a READY_FOR_DEPOSIT status integration with a virtual account
		_, err := models.DBConnectionPool.ExecContext(ctx, `
			INSERT INTO bridge_integration (
				status, kyc_link_id, customer_id, opted_in_by, opted_in_at,
				virtual_account_id, virtual_account_created_by, virtual_account_created_at
			) VALUES (
				'READY_FOR_DEPOSIT', 'kyc-link-123', 'customer-123', 'user-123', NOW(),
				'va-123', 'user-123', NOW()
			)
		`)
		require.NoError(t, err)

		customerResponse := &CustomerInfo{
			ID:        "customer-123",
			Status:    CustomerStatusActive,
			Email:     "john.doe@example.com",
			FirstName: "John",
			LastName:  "Doe",
			Type:      CustomerTypeBusiness,
		}

		mockClient.
			On("GetCustomer", ctx, "customer-123").
			Return(customerResponse, nil).
			Once()

		mockClient.
			On("GetVirtualAccount", ctx, "customer-123", "va-123").
			Return(vaResponse, nil).
			Once()

		svc := createService(t, mockClient, models)

		result, err := svc.GetBridgeIntegration(ctx)
		assert.NoError(t, err)
		assert.Equal(t, vaResponse, result.VirtualAccountDetails)
		assert.Equal(t, "user-123", *result.VirtualAccountCreatedBy)
		assert.NotNil(t, result.VirtualAccountCreatedAt)
	})
}

func Test_Service_CreateVirtualAccount(t *testing.T) {
	models := data.SetupModels(t)
	dbcp := models.DBConnectionPool
	ctx := context.Background()

	tnt := schema.Tenant{
		ID:      "test-tenant",
		BaseURL: utils.Ptr("https://example.com"),
	}
	ctx = sdpcontext.SetTenantInContext(ctx, &tnt)

	t.Run("integration not found", func(t *testing.T) {
		data.CleanupBridgeIntegration(t, ctx, dbcp)
		mockClient := NewMockClient(t)
		svc := createService(t, mockClient, models)

		result, err := svc.CreateVirtualAccount(ctx, "user-123", "GCKFBEIYTKP5RDBPFKWYFVQNMZ5KMGMW3RFKAWJ3CCDQPWXEMFXH7YDN")
		assert.ErrorContains(t, err, "getting Bridge integration")
		assert.Nil(t, result)
	})

	t.Run("integration in error status", func(t *testing.T) {
		data.CleanupBridgeIntegration(t, ctx, dbcp)
		mockClient := NewMockClient(t)
		svc := createService(t, mockClient, models)

		// Create an ERROR status integration
		// For ERROR status, error_message must be NOT NULL
		_, err := models.DBConnectionPool.ExecContext(ctx, `
			INSERT INTO bridge_integration (
				status, kyc_link_id, customer_id, opted_in_by, opted_in_at, error_message
			) VALUES (
				'ERROR', 'kyc-link-123', 'customer-123', 'user-123', NOW(), 'Test error message'
			)
		`)
		require.NoError(t, err)

		result, err := svc.CreateVirtualAccount(ctx, "user-123", "GCKFBEIYTKP5RDBPFKWYFVQNMZ5KMGMW3RFKAWJ3CCDQPWXEMFXH7YDN")
		assert.EqualError(t, err, ErrBridgeNotOptedIn.Error())
		assert.Nil(t, result)
	})

	t.Run("KYC not approved", func(t *testing.T) {
		data.CleanupBridgeIntegration(t, ctx, dbcp)
		mockClient := NewMockClient(t)
		kycResponse := &KYCLinkInfo{
			ID:        "kyc-link-123",
			KYCStatus: KYCStatusUnderReview,
		}

		// Insert integration
		_, err := models.BridgeIntegration.Insert(ctx, models.DBConnectionPool, data.BridgeIntegrationInsert{
			KYCLinkID:  utils.StringPtr("kyc-link-123"),
			CustomerID: "customer-123",
			OptedInBy:  "user-123",
		})
		require.NoError(t, err)

		mockClient.
			On("GetKYCLink", ctx, "kyc-link-123").
			Return(kycResponse, nil).
			Once()

		svc := createService(t, mockClient, models)

		result, err := svc.CreateVirtualAccount(ctx, "user-123", "GCKFBEIYTKP5RDBPFKWYFVQNMZ5KMGMW3RFKAWJ3CCDQPWXEMFXH7YDN")
		assert.EqualError(t, err, ErrBridgeKYCNotApproved.Error())
		assert.Nil(t, result)
	})

	t.Run("KYC rejected", func(t *testing.T) {
		data.CleanupBridgeIntegration(t, ctx, dbcp)
		mockClient := NewMockClient(t)
		kycResponse := &KYCLinkInfo{
			ID:               "kyc-link-123",
			KYCStatus:        KYCStatusRejected,
			RejectionReasons: []string{"invalid documents", "incomplete information"},
		}

		// Insert integration
		_, err := models.BridgeIntegration.Insert(ctx, models.DBConnectionPool, data.BridgeIntegrationInsert{
			KYCLinkID:  utils.StringPtr("kyc-link-123"),
			CustomerID: "customer-123",
			OptedInBy:  "user-123",
		})
		require.NoError(t, err)

		mockClient.
			On("GetKYCLink", ctx, "kyc-link-123").
			Return(kycResponse, nil).
			Once()

		svc := createService(t, mockClient, models)

		result, err := svc.CreateVirtualAccount(ctx, "user-123", "GCKFBEIYTKP5RDBPFKWYFVQNMZ5KMGMW3RFKAWJ3CCDQPWXEMFXH7YDN")
		assert.ErrorContains(t, err, "KYC verification was rejected")
		assert.ErrorContains(t, err, "invalid documents")
		assert.ErrorContains(t, err, "incomplete information")
		assert.Nil(t, result)
	})

	t.Run("Bridge API error creating virtual account", func(t *testing.T) {
		data.CleanupBridgeIntegration(t, ctx, dbcp)
		mockClient := NewMockClient(t)

		// KYC Approved
		kycResponse := &KYCLinkInfo{
			ID:        "kyc-link-123",
			KYCStatus: KYCStatusApproved,
			TOSStatus: TOSStatusApproved,
		}
		// Customer is active
		customerResponse := &CustomerInfo{
			ID:     "customer-123",
			Status: CustomerStatusActive,
		}

		// Insert integration
		_, err := models.BridgeIntegration.Insert(ctx, models.DBConnectionPool, data.BridgeIntegrationInsert{
			KYCLinkID:  utils.StringPtr("kyc-link-123"),
			CustomerID: "customer-123",
			OptedInBy:  "user-123",
		})
		require.NoError(t, err)

		vaRequest := VirtualAccountRequest{
			Source: VirtualAccountSource{
				Currency: "usd",
			},
			Destination: VirtualAccountDestination{
				PaymentRail:    "stellar",
				Currency:       "usdc",
				Address:        "GCKFBEIYTKP5RDBPFKWYFVQNMZ5KMGMW3RFKAWJ3CCDQPWXEMFXH7YDN",
				BlockchainMemo: "sdp-100680ad546c",
			},
		}

		bridgeErr := errors.New("bridge API error")

		mockClient.
			On("GetKYCLink", ctx, "kyc-link-123").
			Return(kycResponse, nil).
			Once()
		mockClient.
			On("GetCustomer", ctx, "customer-123").
			Return(customerResponse, nil).
			Once()

		mockClient.
			On("PostVirtualAccount", ctx, "customer-123", vaRequest).
			Return(nil, bridgeErr).
			Once()

		svc := createService(t, mockClient, models)

		result, err := svc.CreateVirtualAccount(ctx, "user-123", "GCKFBEIYTKP5RDBPFKWYFVQNMZ5KMGMW3RFKAWJ3CCDQPWXEMFXH7YDN")
		assert.EqualError(t, err, "creating virtual account via Bridge API: bridge API error")
		assert.Nil(t, result)
	})

	t.Run("🎉 successfully creates virtual account", func(t *testing.T) {
		data.CleanupBridgeIntegration(t, ctx, dbcp)
		mockClient := NewMockClient(t)

		vaResponse := &VirtualAccountInfo{
			ID:         "va-123",
			CustomerID: "customer-123",
			Status:     VirtualAccountActivated,
			Destination: VirtualAccountDestination{
				PaymentRail:    "stellar",
				Currency:       "usdc",
				Address:        "GCKFBEIYTKP5RDBPFKWYFVQNMZ5KMGMW3RFKAWJ3CCDQPWXEMFXH7YDN",
				BlockchainMemo: "sdp-100680ad546c",
			},
		}

		// Insert integration
		_, err := models.BridgeIntegration.Insert(ctx, models.DBConnectionPool, data.BridgeIntegrationInsert{
			KYCLinkID:  utils.StringPtr("kyc-link-123"),
			CustomerID: "customer-123",
			OptedInBy:  "user-123",
		})
		require.NoError(t, err)

		vaRequest := VirtualAccountRequest{
			Source: VirtualAccountSource{
				Currency: "usd",
			},
			Destination: VirtualAccountDestination{
				PaymentRail:    "stellar",
				Currency:       "usdc",
				Address:        "GCKFBEIYTKP5RDBPFKWYFVQNMZ5KMGMW3RFKAWJ3CCDQPWXEMFXH7YDN",
				BlockchainMemo: "sdp-100680ad546c",
			},
		}

		// KYC Approved
		kycResponse := &KYCLinkInfo{
			ID:        "kyc-link-123",
			KYCStatus: KYCStatusApproved,
			TOSStatus: TOSStatusApproved,
		}
		// Customer is active
		customerResponse := &CustomerInfo{
			ID:     "customer-123",
			Status: CustomerStatusActive,
		}

		mockClient.
			On("GetKYCLink", ctx, "kyc-link-123").
			Return(kycResponse, nil).
			Once()
		mockClient.
			On("GetCustomer", ctx, "customer-123").
			Return(customerResponse, nil).
			Once()

		mockClient.
			On("PostVirtualAccount", ctx, "customer-123", vaRequest).
			Return(vaResponse, nil).
			Once()

		svc := createService(t, mockClient, models)

		result, err := svc.CreateVirtualAccount(ctx, "user-123", "GCKFBEIYTKP5RDBPFKWYFVQNMZ5KMGMW3RFKAWJ3CCDQPWXEMFXH7YDN")
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, data.BridgeIntegrationStatusReadyForDeposit, result.Status)
		assert.Equal(t, "customer-123", *result.CustomerID)
		assert.Equal(t, "user-123", *result.VirtualAccountCreatedBy)
		assert.NotNil(t, result.VirtualAccountCreatedAt)
		assert.Equal(t, vaResponse, result.VirtualAccountDetails)
	})
}

func Test_Service_validateUSDCTrustline(t *testing.T) {
	ctx := context.Background()

	t.Run("distribution account resolver fails", func(t *testing.T) {
		mockDistAccountResolver := sigMocks.NewMockDistributionAccountResolver(t)
		mockDistAccountService := mocks.NewMockDistributionAccountService(t)

		mockDistAccountResolver.
			On("DistributionAccountFromContext", ctx).
			Return(schema.TransactionAccount{}, errors.New("resolver error")).
			Once()

		svc := &Service{
			distributionAccountResolver: mockDistAccountResolver,
			distributionAccountService:  mockDistAccountService,
			networkType:                 utils.TestnetNetworkType,
		}

		err := svc.validateUSDCTrustline(ctx)
		assert.ErrorContains(t, err, "getting distribution account from context: resolver error")
	})

	t.Run("USDC trustline check fails on testnet", func(t *testing.T) {
		mockDistAccountResolver := sigMocks.NewMockDistributionAccountResolver(t)
		mockDistAccountService := mocks.NewMockDistributionAccountService(t)
		testDistAccount := schema.TransactionAccount{
			Address: "GCKFBEIYTKP5RDBPFKWYFVQNMZ5KMGMW3RFKAWJ3CCDQPWXEMFXH7YDN",
		}

		mockDistAccountResolver.
			On("DistributionAccountFromContext", ctx).
			Return(testDistAccount, nil).
			Once()

		mockDistAccountService.
			On("GetBalance", ctx, &testDistAccount, assets.USDCAssetTestnet).
			Return(decimal.Zero, errors.New("trustline not found")).
			Once()

		svc := &Service{
			distributionAccountResolver: mockDistAccountResolver,
			distributionAccountService:  mockDistAccountService,
			networkType:                 utils.TestnetNetworkType,
		}

		err := svc.validateUSDCTrustline(ctx)
		assert.ErrorIs(t, err, ErrBridgeUSDCTrustlineRequired)
		assert.ErrorContains(t, err, "trustline not found")
	})

	t.Run("USDC trustline check fails on pubnet", func(t *testing.T) {
		mockDistAccountResolver := sigMocks.NewMockDistributionAccountResolver(t)
		mockDistAccountService := mocks.NewMockDistributionAccountService(t)
		testDistAccount := schema.TransactionAccount{
			Address: "GCKFBEIYTKP5RDBPFKWYFVQNMZ5KMGMW3RFKAWJ3CCDQPWXEMFXH7YDN",
		}

		mockDistAccountResolver.
			On("DistributionAccountFromContext", ctx).
			Return(testDistAccount, nil).
			Once()

		mockDistAccountService.
			On("GetBalance", ctx, &testDistAccount, assets.USDCAssetPubnet).
			Return(decimal.Zero, errors.New("no trustline exists")).
			Once()

		svc := &Service{
			distributionAccountResolver: mockDistAccountResolver,
			distributionAccountService:  mockDistAccountService,
			networkType:                 utils.PubnetNetworkType,
		}

		err := svc.validateUSDCTrustline(ctx)
		assert.ErrorIs(t, err, ErrBridgeUSDCTrustlineRequired)
		assert.ErrorContains(t, err, "no trustline exists")
	})

	t.Run("🎉 successfully validates USDC trustline on testnet with 0 balance", func(t *testing.T) {
		mockDistAccountResolver := sigMocks.NewMockDistributionAccountResolver(t)
		mockDistAccountService := mocks.NewMockDistributionAccountService(t)
		testDistAccount := schema.TransactionAccount{
			Address: "GCKFBEIYTKP5RDBPFKWYFVQNMZ5KMGMW3RFKAWJ3CCDQPWXEMFXH7YDN",
		}

		mockDistAccountResolver.
			On("DistributionAccountFromContext", ctx).
			Return(testDistAccount, nil).
			Once()

		mockDistAccountService.
			On("GetBalance", ctx, &testDistAccount, assets.USDCAssetTestnet).
			Return(decimal.Zero, nil).
			Once()

		svc := &Service{
			distributionAccountResolver: mockDistAccountResolver,
			distributionAccountService:  mockDistAccountService,
			networkType:                 utils.TestnetNetworkType,
		}

		err := svc.validateUSDCTrustline(ctx)
		assert.NoError(t, err)
	})

	t.Run("🎉 successfully validates USDC trustline on pubnet", func(t *testing.T) {
		mockDistAccountResolver := sigMocks.NewMockDistributionAccountResolver(t)
		mockDistAccountService := mocks.NewMockDistributionAccountService(t)
		testDistAccount := schema.TransactionAccount{
			Address: "GCKFBEIYTKP5RDBPFKWYFVQNMZ5KMGMW3RFKAWJ3CCDQPWXEMFXH7YDN",
		}

		mockDistAccountResolver.
			On("DistributionAccountFromContext", ctx).
			Return(testDistAccount, nil).
			Once()

		mockDistAccountService.
			On("GetBalance", ctx, &testDistAccount, assets.USDCAssetPubnet).
			Return(decimal.NewFromFloat(250.5), nil).
			Once()

		svc := &Service{
			distributionAccountResolver: mockDistAccountResolver,
			distributionAccountService:  mockDistAccountService,
			networkType:                 utils.PubnetNetworkType,
		}

		err := svc.validateUSDCTrustline(ctx)
		assert.NoError(t, err)
	})
}

func createService(t *testing.T, mockClient *MockClient, models *data.Models) *Service {
	t.Helper()

	// Create mock distribution account resolver that returns a test account
	mockDistAccountResolver := sigMocks.NewMockDistributionAccountResolver(t)
	testDistAccount := schema.TransactionAccount{
		Address: "GCKFBEIYTKP5RDBPFKWYFVQNMZ5KMGMW3RFKAWJ3CCDQPWXEMFXH7YDN",
	}
	mockDistAccountResolver.
		On("DistributionAccountFromContext", mock.Anything).
		Return(testDistAccount, nil).
		Maybe()

	// Create mock distribution account service that allows USDC balance check
	mockDistAccountService := mocks.NewMockDistributionAccountService(t)
	mockDistAccountService.
		On("GetBalance", mock.Anything, mock.Anything, assets.USDCAssetTestnet).
		Return(decimal.NewFromFloat(100.0), nil).
		Maybe()

	return &Service{
		client:                      mockClient,
		baseURL:                     "https://api.bridge.example.com",
		apiKey:                      "test-api-key",
		models:                      models,
		distributionAccountResolver: mockDistAccountResolver,
		distributionAccountService:  mockDistAccountService,
		networkType:                 utils.TestnetNetworkType,
	}
}

func Test_Service_OptInForExistingCustomer_Validation(t *testing.T) {
	service := &Service{}
	ctx := context.Background()

	t.Run("empty customer ID", func(t *testing.T) {
		result, err := service.OptInForExistingCustomer(ctx, "", "user-123")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "customer ID is required")
		assert.Nil(t, result)
	})

	t.Run("empty user ID", func(t *testing.T) {
		result, err := service.OptInForExistingCustomer(ctx, "customer-123", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "user ID is required")
		assert.Nil(t, result)
	})
}

func Test_Service_OptInForExistingCustomer_Integration(t *testing.T) {
	models := data.SetupModels(t)
	ctx := sdpcontext.SetTenantInContext(context.Background(), &schema.Tenant{ID: "tenant-id", Name: "test-tenant"})

	t.Run("successful manual opt-in", func(t *testing.T) {
		_, err := models.DBConnectionPool.ExecContext(ctx, "DELETE FROM bridge_integration")
		require.NoError(t, err)

		// Mock client
		mockClient := NewMockClient(t)
		mockClient.
			On("GetCustomer", mock.Anything, "customer-456").
			Return(&CustomerInfo{
				ID:     "customer-456",
				Status: CustomerStatusActive,
			}, nil).
			Once()

		service := createService(t, mockClient, models)

		result, err := service.OptInForExistingCustomer(ctx, "customer-456", "user-123")
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, data.BridgeIntegrationStatusOptedIn, result.Status)
		assert.Equal(t, "customer-456", *result.CustomerID)
		assert.Equal(t, "user-123", *result.OptedInBy)
		assert.Nil(t, result.KYCLinkInfo) // Should be nil for manual onboarding

		// Verify it was actually stored
		retrieved, err := models.BridgeIntegration.Get(ctx)
		require.NoError(t, err)
		assert.Equal(t, data.BridgeIntegrationStatusOptedIn, retrieved.Status)
		assert.Equal(t, "customer-456", *retrieved.CustomerID)
		assert.Equal(t, "user-123", *retrieved.OptedInBy)
		// For manual onboarding, KYCLinkID should be nil since no KYC link is created
		assert.Nil(t, retrieved.KYCLinkID)
	})
}

func Test_Service_OptInForExistingCustomer_AcrossTenants(t *testing.T) {
	// Cleanups run last-in first-out, so every pool closes before the test database is dropped.
	dbt := dbtest.Open(t)
	t.Cleanup(func() { dbt.Close() })
	competitorPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	t.Cleanup(func() { competitorPool.Close() })

	// Each tenant gets a real sdp_<name> schema, its own pool and its own service.
	type tenantEnv struct {
		ctx     context.Context
		models  *data.Models
		service *Service
		client  *MockClient
	}
	newTenantEnv := func(t *testing.T, name string) tenantEnv {
		t.Helper()
		dsn := tenant.PrepareDBForTenant(t, dbt, name)
		pool, poolErr := db.OpenDBConnectionPool(dsn)
		require.NoError(t, poolErr)
		t.Cleanup(func() { pool.Close() })
		models, modelsErr := data.NewModels(pool)
		require.NoError(t, modelsErr)

		client := NewMockClient(t)
		service := createService(t, client, models)

		ctx := sdpcontext.SetTenantInContext(context.Background(), &schema.Tenant{ID: name + "-id", Name: name})
		return tenantEnv{ctx: ctx, models: models, service: service, client: client}
	}
	expectActiveCustomer := func(client *MockClient, customerID string) {
		client.
			On("GetCustomer", mock.Anything, customerID).
			Return(&CustomerInfo{ID: customerID, Status: CustomerStatusActive}, nil).
			Once()
	}

	tenantA := newTenantEnv(t, "tenanta")
	tenantB := newTenantEnv(t, "tenantb")
	tenantC := newTenantEnv(t, "tenantc")

	t.Run("second tenant cannot bind a customer ID another tenant holds", func(t *testing.T) {
		expectActiveCustomer(tenantA.client, "customer-direct")
		result, optInErr := tenantA.service.OptInForExistingCustomer(tenantA.ctx, "customer-direct", "user-a")
		require.NoError(t, optInErr)
		assert.Equal(t, "customer-direct", *result.CustomerID)

		expectActiveCustomer(tenantB.client, "customer-direct")
		result, optInErr = tenantB.service.OptInForExistingCustomer(tenantB.ctx, "customer-direct", "user-b")
		require.ErrorIs(t, optInErr, ErrBridgeCustomerAlreadyBound)
		assert.Nil(t, result)

		_, getErr := tenantB.models.BridgeIntegration.Get(tenantB.ctx)
		assert.ErrorIs(t, getErr, data.ErrRecordNotFound)
	})

	t.Run("a different customer ID is still accepted", func(t *testing.T) {
		expectActiveCustomer(tenantB.client, "customer-other")
		result, optInErr := tenantB.service.OptInForExistingCustomer(tenantB.ctx, "customer-other", "user-b")
		require.NoError(t, optInErr)
		assert.Equal(t, "customer-other", *result.CustomerID)
	})

	t.Run("customer ID created through the KYC link flow cannot be bound by another tenant", func(t *testing.T) {
		// tenantB holds "customer-other"; simulate a KYC-link row the same way OptInToBridge stores it.
		_, execErr := tenantB.models.DBConnectionPool.ExecContext(tenantB.ctx,
			"UPDATE bridge_integration SET kyc_link_id = 'kyc-link-b', customer_id = 'customer-kyc'")
		require.NoError(t, execErr)

		expectActiveCustomer(tenantC.client, "customer-kyc")
		result, optInErr := tenantC.service.OptInForExistingCustomer(tenantC.ctx, "customer-kyc", "user-c")
		require.ErrorIs(t, optInErr, ErrBridgeCustomerAlreadyBound)
		assert.Nil(t, result)
	})

	t.Run("opt-in waits on the customer ID lock and re-checks once it is released", func(t *testing.T) {
		tenantD := newTenantEnv(t, "tenantd")

		// Hold the lock the way a competing opt-in would.
		lockTx, txErr := competitorPool.BeginTxx(context.Background(), nil)
		require.NoError(t, txErr)
		defer func() { _ = lockTx.Rollback() }()
		require.NoError(t, acquireBridgeCustomerLock(context.Background(), lockTx, "customer-race"))

		expectActiveCustomer(tenantC.client, "customer-race")
		done := make(chan error, 1)
		go func() {
			_, optInErr := tenantC.service.OptInForExistingCustomer(tenantC.ctx, "customer-race", "user-c")
			done <- optInErr
		}()

		select {
		case optInErr := <-done:
			require.FailNow(t, "opt-in did not wait for the customer ID lock", "returned: %v", optInErr)
		case <-time.After(500 * time.Millisecond):
		}

		// The competitor commits its row, then releases the lock.
		_, insertErr := tenantD.models.BridgeIntegration.Insert(tenantD.ctx, tenantD.models.DBConnectionPool, data.BridgeIntegrationInsert{
			CustomerID: "customer-race",
			OptedInBy:  "user-d",
		})
		require.NoError(t, insertErr)
		require.NoError(t, lockTx.Commit())

		select {
		case optInErr := <-done:
			require.ErrorIs(t, optInErr, ErrBridgeCustomerAlreadyBound)
		case <-time.After(10 * time.Second):
			require.FailNow(t, "opt-in did not resume after the lock was released")
		}
		_, getErr := tenantC.models.BridgeIntegration.Get(tenantC.ctx)
		assert.ErrorIs(t, getErr, data.ErrRecordNotFound)
	})

	t.Run("stores the canonical ID from Bridge and rejects another spelling of a held customer", func(t *testing.T) {
		tenantF := newTenantEnv(t, "tenantf")
		tenantG := newTenantEnv(t, "tenantg")
		const canonical = "6a1f0b2c-3d4e-4f50-8a6b-7c8d9e0f1a2b"
		const upper = "6A1F0B2C-3D4E-4F50-8A6B-7C8D9E0F1A2B"

		// Bridge resolves the uppercase spelling but answers with its own lowercase ID.
		tenantF.client.
			On("GetCustomer", mock.Anything, upper).
			Return(&CustomerInfo{ID: canonical, Status: CustomerStatusActive}, nil).
			Once()
		result, optInErr := tenantF.service.OptInForExistingCustomer(tenantF.ctx, upper, "user-f")
		require.NoError(t, optInErr)
		assert.Equal(t, canonical, *result.CustomerID)
		stored, getErr := tenantF.models.BridgeIntegration.Get(tenantF.ctx)
		require.NoError(t, getErr)
		assert.Equal(t, canonical, *stored.CustomerID)

		tenantG.client.
			On("GetCustomer", mock.Anything, upper).
			Return(&CustomerInfo{ID: canonical, Status: CustomerStatusActive}, nil).
			Once()
		result, optInErr = tenantG.service.OptInForExistingCustomer(tenantG.ctx, upper, "user-g")
		require.ErrorIs(t, optInErr, ErrBridgeCustomerAlreadyBound)
		assert.Nil(t, result)
	})

	t.Run("rejects the customer when Bridge returns an empty or different ID", func(t *testing.T) {
		tenantH := newTenantEnv(t, "tenanth")
		for name, returned := range map[string]string{"empty": "", "different": "other-customer"} {
			t.Run(name, func(t *testing.T) {
				tenantH.client.
					On("GetCustomer", mock.Anything, "customer-h").
					Return(&CustomerInfo{ID: returned, Status: CustomerStatusActive}, nil).
					Once()
				result, optInErr := tenantH.service.OptInForExistingCustomer(tenantH.ctx, "customer-h", "user-h")
				require.ErrorIs(t, optInErr, ErrBridgeInvalidCustomerID)
				assert.Nil(t, result)
				_, getErr := tenantH.models.BridgeIntegration.Get(tenantH.ctx)
				assert.ErrorIs(t, getErr, data.ErrRecordNotFound)
			})
		}
	})

	t.Run("fails when the tenant is not in the context", func(t *testing.T) {
		tenantE := newTenantEnv(t, "tenante")
		expectActiveCustomer(tenantE.client, "customer-no-tenant")
		result, optInErr := tenantE.service.OptInForExistingCustomer(context.Background(), "customer-no-tenant", "user-e")
		require.ErrorContains(t, optInErr, "getting tenant from context")
		assert.Nil(t, result)
	})
}
