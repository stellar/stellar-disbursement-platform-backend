package bridge

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
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
			KYCType:     KYCTypeBusiness,
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
			KYCType:     KYCTypeBusiness,
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
			KYCType:     KYCTypeBusiness,
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
			KYCType:     KYCTypeBusiness,
		})
		assert.EqualError(t, err, "validating opt-in options: email is required to opt into Bridge integration")
		assert.Nil(t, result)
	})

	t.Run("missing KYCType", func(t *testing.T) {
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
		assert.EqualError(t, err, "validating opt-in options: KYCType must be either 'individual' or 'business'")
		assert.Nil(t, result)
	})

	t.Run("already opted in", func(t *testing.T) {
		data.CleanupBridgeIntegration(t, ctx, dbcp)
		mockClient := NewMockClient(t)
		svc := createService(t, mockClient, models)

		// Insert existing integration
		_, err := models.BridgeIntegration.Insert(ctx, data.BridgeIntegrationInsert{
			KYCLinkID:  "existing-kyc-id",
			CustomerID: "existing-customer-id",
			OptedInBy:  "existing-user",
		})
		require.NoError(t, err)

		result, err := svc.OptInToBridge(ctx, OptInOptions{
			UserID:      "user-123",
			FullName:    fullName,
			Email:       email,
			RedirectURL: redirectURL,
			KYCType:     KYCTypeBusiness,
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
				Type:        KYCTypeBusiness,
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
			KYCType:     KYCTypeBusiness,
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
			KYCType:     KYCTypeBusiness,
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
			Return(0.0, errors.New("no trustline found")).
			Once()
		svc.distributionAccountService = mockDistAccountService

		result, err := svc.OptInToBridge(ctx, OptInOptions{
			UserID:      "user-123",
			FullName:    fullName,
			Email:       email,
			RedirectURL: redirectURL,
			KYCType:     KYCTypeBusiness,
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
			Return(50.0, nil).
			Once()

		kycResponse := &KYCLinkInfo{
			ID:         "kyc-link-123",
			CustomerID: "customer-123",
			FullName:   fullName,
			Email:      email,
			Type:       KYCTypeBusiness,
			KYCStatus:  KYCStatusNotStarted,
			TOSStatus:  TOSStatusPending,
		}

		mockClient.
			On("PostKYCLink", ctx, KYCLinkRequest{
				FullName:    fullName,
				Email:       email,
				Type:        KYCTypeBusiness,
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
			KYCType:     KYCTypeBusiness,
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
			Type:       KYCTypeBusiness,
			KYCStatus:  KYCStatusNotStarted,
			TOSStatus:  TOSStatusPending,
		}

		mockClient.
			On("PostKYCLink", ctx, KYCLinkRequest{
				FullName:    fullName,
				Email:       email,
				Type:        KYCTypeBusiness,
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
			KYCType:     KYCTypeBusiness,
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
		kycResponse := &KYCLinkInfo{
			ID:         "kyc-link-123",
			CustomerID: "customer-123",
			KYCStatus:  KYCStatusApproved,
			TOSStatus:  TOSStatusApproved,
		}

		// Insert integration
		integration, err := models.BridgeIntegration.Insert(ctx, data.BridgeIntegrationInsert{
			KYCLinkID:  "kyc-link-123",
			CustomerID: "customer-123",
			OptedInBy:  "user-123",
		})
		require.NoError(t, err)

		mockClient.
			On("GetKYCLink", ctx, "kyc-link-123").
			Return(kycResponse, nil).
			Once()

		svc := createService(t, mockClient, models)

		result, err := svc.GetBridgeIntegration(ctx)
		assert.NoError(t, err)
		assert.Equal(t, integration.Status, result.Status)
		assert.Equal(t, integration.CustomerID, result.CustomerID)
		assert.Equal(t, integration.OptedInBy, result.OptedInBy)
		assert.Equal(t, kycResponse, result.KYCLinkInfo)
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

		mockClient.
			On("GetKYCLink", ctx, "kyc-link-123").
			Return(&KYCLinkInfo{ID: "kyc-link-123"}, nil).
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

	tnt := tenant.Tenant{
		ID:      "test-tenant",
		BaseURL: utils.Ptr("https://example.com"),
	}
	ctx = tenant.SaveTenantInContext(ctx, &tnt)

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
		_, err := models.BridgeIntegration.Insert(ctx, data.BridgeIntegrationInsert{
			KYCLinkID:  "kyc-link-123",
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
		_, err := models.BridgeIntegration.Insert(ctx, data.BridgeIntegrationInsert{
			KYCLinkID:  "kyc-link-123",
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
		kycResponse := &KYCLinkInfo{
			ID:        "kyc-link-123",
			KYCStatus: KYCStatusApproved,
			TOSStatus: TOSStatusApproved,
		}

		// Insert integration
		_, err := models.BridgeIntegration.Insert(ctx, data.BridgeIntegrationInsert{
			KYCLinkID:  "kyc-link-123",
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
		kycResponse := &KYCLinkInfo{
			ID:        "kyc-link-123",
			KYCStatus: KYCStatusApproved,
			TOSStatus: TOSStatusApproved,
		}

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
		_, err := models.BridgeIntegration.Insert(ctx, data.BridgeIntegrationInsert{
			KYCLinkID:  "kyc-link-123",
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

		mockClient.
			On("GetKYCLink", ctx, "kyc-link-123").
			Return(kycResponse, nil).
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
			Return(0.0, errors.New("trustline not found")).
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
			Return(0.0, errors.New("no trustline exists")).
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
			Return(0.0, nil).
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
			Return(250.5, nil).
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
		Return(100.0, nil).
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
