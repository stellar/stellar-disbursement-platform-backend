package circle

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	monitorMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/monitor/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

func Test_ServiceOptions_Validate(t *testing.T) {
	var clientFactory ClientFactory = func(clientOpts ClientOptions) ClientInterface {
		return nil
	}
	circleClientConfigModel := &ClientConfigModel{}
	mockTenantManager := &tenant.TenantManagerMock{}
	mockMonitorSvc := monitorMocks.NewMockMonitorService(t)

	testCases := []struct {
		name                string
		opts                ServiceOptions
		expectedErrContains string
	}{
		{
			name:                "ClientFactory validation fails",
			opts:                ServiceOptions{},
			expectedErrContains: "ClientFactory is required",
		},
		{
			name:                "ClientConfigModel validation fails",
			opts:                ServiceOptions{ClientFactory: clientFactory},
			expectedErrContains: "ClientConfigModel is required",
		},
		{
			name:                "TenantManager validation fails",
			opts:                ServiceOptions{ClientFactory: clientFactory, ClientConfigModel: circleClientConfigModel},
			expectedErrContains: "TenantManager is required",
		},
		{
			name:                "MonitorService validation fails",
			opts:                ServiceOptions{ClientFactory: clientFactory, ClientConfigModel: circleClientConfigModel, TenantManager: mockTenantManager},
			expectedErrContains: "MonitorService is required",
		},
		{
			name: "NetworkType validation fails",
			opts: ServiceOptions{
				ClientFactory:     clientFactory,
				ClientConfigModel: circleClientConfigModel,
				TenantManager:     mockTenantManager,
				MonitorService:    mockMonitorSvc,
				NetworkType:       utils.NetworkType("FOOBAR"),
			},
			expectedErrContains: `validating NetworkType: invalid network type "FOOBAR"`,
		},
		{
			name: "EncryptionPassphrase validation fails",
			opts: ServiceOptions{
				ClientFactory:        clientFactory,
				ClientConfigModel:    circleClientConfigModel,
				TenantManager:        mockTenantManager,
				MonitorService:       mockMonitorSvc,
				NetworkType:          utils.TestnetNetworkType,
				EncryptionPassphrase: "FOO BAR",
			},
			expectedErrContains: "EncryptionPassphrase is invalid",
		},
		{
			name: "🎉 successfully validates options",
			opts: ServiceOptions{
				ClientFactory:        clientFactory,
				ClientConfigModel:    circleClientConfigModel,
				TenantManager:        mockTenantManager,
				NetworkType:          utils.TestnetNetworkType,
				MonitorService:       mockMonitorSvc,
				EncryptionPassphrase: "SCW5I426WV3IDTLSTLQEHC6BMXWI2Z6C4DXAOC4ZA2EIHTAZQ6VD3JI6",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.opts.Validate()
			if tc.expectedErrContains != "" {
				assert.Contains(t, err.Error(), tc.expectedErrContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_NewService(t *testing.T) {
	t.Run("handle constructor error", func(t *testing.T) {
		svc, err := NewService(ServiceOptions{})
		assert.Empty(t, svc)
		assert.ErrorContains(t, err, "validating circle.Service options: ClientFactory is required")
	})

	t.Run("🎉 successfully creates a new Service", func(t *testing.T) {
		clientFactory := func(clientOptions ClientOptions) ClientInterface {
			return nil
		}
		clientConfigModel := &ClientConfigModel{}
		mockTntManager := &tenant.TenantManagerMock{}
		mockMonitorSvc := monitorMocks.NewMockMonitorService(t)
		networkType := utils.TestnetNetworkType
		encryptionPassphrase := "SCW5I426WV3IDTLSTLQEHC6BMXWI2Z6C4DXAOC4ZA2EIHTAZQ6VD3JI6"

		svc, err := NewService(ServiceOptions{
			ClientFactory:        clientFactory,
			ClientConfigModel:    clientConfigModel,
			TenantManager:        mockTntManager,
			NetworkType:          networkType,
			EncryptionPassphrase: encryptionPassphrase,
			MonitorService:       mockMonitorSvc,
		})
		assert.NoError(t, err)

		wantService := &Service{
			ClientFactory:        clientFactory,
			ClientConfigModel:    clientConfigModel,
			NetworkType:          networkType,
			EncryptionPassphrase: encryptionPassphrase,
		}

		assert.Equal(t, wantService.ClientFactory(ClientOptions{
			NetworkType:    networkType,
			APIKey:         "FOO BAR",
			TenantManager:  mockTntManager,
			MonitorService: mockMonitorSvc,
		}), svc.ClientFactory(ClientOptions{
			NetworkType:    networkType,
			APIKey:         "FOO BAR",
			TenantManager:  mockTntManager,
			MonitorService: mockMonitorSvc,
		}))
		assert.Equal(t, wantService.ClientConfigModel, svc.ClientConfigModel)
		assert.Equal(t, wantService.NetworkType, svc.NetworkType)
		assert.Equal(t, wantService.EncryptionPassphrase, svc.EncryptionPassphrase)
	})
}

func Test_Service_getClient(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	pubKey := "GBFL6FHGHTOSNCAR3GE2MX53Y6BZ3QBCYSTBOCJBSFOWZ35EG2F6T4LG"
	encryptionPassphrase := "SCW5I426WV3IDTLSTLQEHC6BMXWI2Z6C4DXAOC4ZA2EIHTAZQ6VD3JI6"
	apiKey := "api-key"
	encryptedAPIKey := "72TARC5aoKJOEUIMTR9nlITP6+MbugQtS+2faBKSQbCrXic=" // <--- "api-key" encrypted with the encryptionPassphrase.
	networkType := utils.TestnetNetworkType
	clientConfigModel := NewClientConfigModel(dbConnectionPool)
	mockTntManager := &tenant.TenantManagerMock{}
	mMonitorService := monitorMocks.NewMockMonitorService(t)

	// Add a client config to the database.
	err = clientConfigModel.Upsert(ctx, ClientConfigUpdate{
		WalletID:           utils.StringPtr("the_wallet_id"),
		EncryptedAPIKey:    utils.StringPtr(encryptedAPIKey),
		EncrypterPublicKey: utils.StringPtr(pubKey),
	})
	require.NoError(t, err)

	// Create a service.
	svc, err := NewService(ServiceOptions{
		ClientFactory:        NewClient,
		ClientConfigModel:    clientConfigModel,
		TenantManager:        mockTntManager,
		NetworkType:          networkType,
		EncryptionPassphrase: encryptionPassphrase,
		MonitorService:       mMonitorService,
	})
	assert.NoError(t, err)

	circleClient, err := svc.getClientForTenantInContext(ctx)
	assert.NoError(t, err)
	wantCircleClient := NewClient(ClientOptions{
		NetworkType:    networkType,
		APIKey:         apiKey,
		TenantManager:  mockTntManager,
		MonitorService: mMonitorService,
	})
	assert.Equal(t, wantCircleClient, circleClient)
}

func Test_Service_allMethods(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	pubKey := "GBFL6FHGHTOSNCAR3GE2MX53Y6BZ3QBCYSTBOCJBSFOWZ35EG2F6T4LG"
	encryptionPassphrase := "SCW5I426WV3IDTLSTLQEHC6BMXWI2Z6C4DXAOC4ZA2EIHTAZQ6VD3JI6"
	encryptedAPIKey := "72TARC5aoKJOEUIMTR9nlITP6+MbugQtS+2faBKSQbCrXic=" // <--- "api-key" encrypted with the encryptionPassphrase.
	networkType := utils.TestnetNetworkType
	clientConfigModel := NewClientConfigModel(dbConnectionPool)
	mockTntManager := &tenant.TenantManagerMock{}

	// Add a client config to the database.
	err = clientConfigModel.Upsert(ctx, ClientConfigUpdate{
		WalletID:           utils.StringPtr("the_wallet_id"),
		EncryptedAPIKey:    utils.StringPtr(encryptedAPIKey),
		EncrypterPublicKey: utils.StringPtr(pubKey),
	})
	require.NoError(t, err)

	// Method used to spin up a service with a mock client.
	createService := func(t *testing.T, mCircleClient *MockClient) *Service {
		svc, err := NewService(ServiceOptions{
			ClientFactory: func(clientOpts ClientOptions) ClientInterface {
				return mCircleClient
			},
			ClientConfigModel:    clientConfigModel,
			TenantManager:        mockTntManager,
			NetworkType:          networkType,
			EncryptionPassphrase: encryptionPassphrase,
			MonitorService:       monitorMocks.NewMockMonitorService(t),
		})
		require.NoError(t, err)
		return svc
	}

	t.Run("Ping", func(t *testing.T) {
		mCircleClient := NewMockClient(t)
		mCircleClient.
			On("Ping", ctx).
			Return(true, nil).
			Once()
		svc := createService(t, mCircleClient)

		res, err := svc.Ping(ctx)
		assert.NoError(t, err)
		assert.True(t, res)
	})

	t.Run("PostTransfer", func(t *testing.T) {
		mCircleClient := NewMockClient(t)
		transferRequest := TransferRequest{
			Source: TransferAccount{
				Type: TransferAccountTypeWallet,
				ID:   "wallet-id",
			},
			Destination: TransferAccount{
				Type:    TransferAccountTypeWallet,
				Chain:   "XLM",
				Address: pubKey,
			},
			Amount: Balance{
				Amount:   "123.45",
				Currency: "USD",
			},
			IdempotencyKey: "idempotency-key",
		}
		mCircleClient.
			On("PostTransfer", ctx, transferRequest).
			Return(&Transfer{ID: "transfer-id"}, nil).
			Once()
		svc := createService(t, mCircleClient)

		res, err := svc.PostTransfer(ctx, transferRequest)
		assert.NoError(t, err)
		assert.Equal(t, &Transfer{ID: "transfer-id"}, res)
	})

	t.Run("GetTransferByID", func(t *testing.T) {
		mCircleClient := NewMockClient(t)
		mCircleClient.
			On("GetTransferByID", ctx, "transfer-id").
			Return(&Transfer{ID: "transfer-id"}, nil).
			Once()
		svc := createService(t, mCircleClient)

		res, err := svc.GetTransferByID(ctx, "transfer-id")
		assert.NoError(t, err)
		assert.Equal(t, &Transfer{ID: "transfer-id"}, res)
	})

	t.Run("GetAccountConfiguration", func(t *testing.T) {
		mClient := NewMockClient(t)
		mClient.
			On("GetAccountConfiguration", ctx).
			Return(&AccountConfiguration{
				Payments: WalletConfig{
					MasterWalletID: "master-wallet-id",
				},
			}, nil).
			Once()
		svc := createService(t, mClient)

		res, err := svc.GetAccountConfiguration(ctx)
		assert.NoError(t, err)
		assert.Equal(t, &AccountConfiguration{
			Payments: WalletConfig{
				MasterWalletID: "master-wallet-id",
			},
		}, res)
	})

	t.Run("GetBusinessBalances", func(t *testing.T) {
		mCircleClient := NewMockClient(t)
		mCircleClient.
			On("GetBusinessBalances", ctx).
			Return(&Balances{
				Available: []Balance{
					{Currency: "USD", Amount: "1234"},
				},
				Unsettled: []Balance{},
			}, nil).
			Once()
		svc := createService(t, mCircleClient)

		res, err := svc.GetBusinessBalances(ctx)
		assert.NoError(t, err)
		assert.Equal(t, &Balances{
			Available: []Balance{
				{Currency: "USD", Amount: "1234"},
			},
			Unsettled: []Balance{},
		}, res)
	})
}
