package circle

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stellar/go/keypair"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

func Test_ServiceOptions_Validate(t *testing.T) {
	var clientFactory ClientFactory = func(networkType utils.NetworkType, apiKey string) ClientInterface {
		return nil
	}
	circleClientConfigModel := &ClientConfigModel{}

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
			name: "NetworkType validation fails",
			opts: ServiceOptions{
				ClientFactory:     clientFactory,
				ClientConfigModel: circleClientConfigModel,
				NetworkType:       utils.NetworkType("FOOBAR"),
			},
			expectedErrContains: `validating NetworkType: invalid network type "FOOBAR"`,
		},
		{
			name: "EncryptionPassphrase validation fails",
			opts: ServiceOptions{
				ClientFactory:        clientFactory,
				ClientConfigModel:    circleClientConfigModel,
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
				NetworkType:          utils.TestnetNetworkType,
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
		clientFactory := func(networkType utils.NetworkType, apiKey string) ClientInterface {
			return nil
		}
		clientConfigModel := &ClientConfigModel{}
		networkType := utils.TestnetNetworkType
		encryptionPassphrase := "SCW5I426WV3IDTLSTLQEHC6BMXWI2Z6C4DXAOC4ZA2EIHTAZQ6VD3JI6"

		svc, err := NewService(ServiceOptions{
			ClientFactory:        clientFactory,
			ClientConfigModel:    clientConfigModel,
			NetworkType:          networkType,
			EncryptionPassphrase: encryptionPassphrase,
		})
		assert.NoError(t, err)

		wantService := &Service{
			ClientFactory:        clientFactory,
			ClientConfigModel:    clientConfigModel,
			NetworkType:          networkType,
			EncryptionPassphrase: encryptionPassphrase,
		}
		assert.Equal(t, wantService.ClientFactory(networkType, "FOO BAR"), svc.ClientFactory(networkType, "FOO BAR"))
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
		NetworkType:          networkType,
		EncryptionPassphrase: encryptionPassphrase,
	})
	assert.NoError(t, err)

	circleClient, err := svc.getClient(ctx)
	assert.NoError(t, err)
	wantCircleClient := NewClient(networkType, apiKey)
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
			ClientFactory: func(networkType utils.NetworkType, apiKey string) ClientInterface {
				return mCircleClient
			},
			ClientConfigModel:    clientConfigModel,
			NetworkType:          networkType,
			EncryptionPassphrase: encryptionPassphrase,
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
			Amount: Money{
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

	t.Run("GetWalletByID", func(t *testing.T) {
		mCircleClient := NewMockClient(t)
		mCircleClient.
			On("GetWalletByID", ctx, "wallet-id").
			Return(&Wallet{WalletID: "wallet-id"}, nil).
			Once()
		svc := createService(t, mCircleClient)

		res, err := svc.GetWalletByID(ctx, "wallet-id")
		assert.NoError(t, err)
		assert.Equal(t, &Wallet{WalletID: "wallet-id"}, res)
	})
}

func Test_PaymentRequest_getCircleAssetCode(t *testing.T) {
	tests := []struct {
		name              string
		stellarAssetCode  string
		expectedAssetCode string
		wantErr           string
	}{
		{
			name:              "USDC asset code",
			stellarAssetCode:  "USDC",
			expectedAssetCode: "USD",
			wantErr:           "",
		},
		{
			name:              "EURC asset code",
			stellarAssetCode:  "EURC",
			expectedAssetCode: "EUR",
			wantErr:           "",
		},
		{
			name:              "unsupported asset code",
			stellarAssetCode:  "XYZ",
			expectedAssetCode: "",
			wantErr:           "unsupported asset code: XYZ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := PaymentRequest{
				StellarAssetCode: tt.stellarAssetCode,
			}

			actualAssetCode, actualErr := p.getCircleAssetCode()

			if tt.wantErr != "" {
				assert.ErrorContains(t, actualErr, tt.wantErr)
			} else {
				assert.NoError(t, actualErr)
			}

			assert.Equal(t, tt.expectedAssetCode, actualAssetCode)
		})
	}
}

func Test_PaymentRequest_Validate(t *testing.T) {
	validDestinationAddress := keypair.MustRandom().Address()

	tests := []struct {
		name       string
		paymentReq PaymentRequest
		wantErr    string
	}{
		{
			name: "missing source wallet ID",
			paymentReq: PaymentRequest{
				SourceWalletID:            "",
				DestinationStellarAddress: validDestinationAddress,
				Amount:                    "100.00",
				StellarAssetCode:          "USDC",
				IdempotencyKey:            uuid.New().String(),
			},
			wantErr: "source wallet ID is required",
		},
		{
			name: "invalid destination stellar address",
			paymentReq: PaymentRequest{
				SourceWalletID:            "source_wallet_123",
				DestinationStellarAddress: "invalid_address",
				Amount:                    "100.00",
				StellarAssetCode:          "USDC",
				IdempotencyKey:            uuid.New().String(),
			},
			wantErr: "destination stellar address is not a valid public key",
		},
		{
			name: "invalid amount",
			paymentReq: PaymentRequest{
				SourceWalletID:            "source_wallet_123",
				DestinationStellarAddress: validDestinationAddress,
				Amount:                    "invalid_amount",
				StellarAssetCode:          "USDC",
				IdempotencyKey:            uuid.New().String(),
			},
			wantErr: "amount is not valid",
		},
		{
			name: "missing stellar asset code",
			paymentReq: PaymentRequest{
				SourceWalletID:            "source_wallet_123",
				DestinationStellarAddress: validDestinationAddress,
				Amount:                    "100.00",
				StellarAssetCode:          "",
				IdempotencyKey:            uuid.New().String(),
			},
			wantErr: "stellar asset code is required",
		},
		{
			name: "invalid idempotency key",
			paymentReq: PaymentRequest{
				SourceWalletID:            "source_wallet_123",
				DestinationStellarAddress: validDestinationAddress,
				Amount:                    "100.00",
				StellarAssetCode:          "USDC",
				IdempotencyKey:            "invalid_uuid",
			},
			wantErr: "idempotency key is not valid",
		},
		{
			name: "valid payment request",
			paymentReq: PaymentRequest{
				SourceWalletID:            "source_wallet_123",
				DestinationStellarAddress: validDestinationAddress,
				Amount:                    "100.00",
				StellarAssetCode:          "USDC",
				IdempotencyKey:            uuid.New().String(),
			},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.paymentReq.Validate()

			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
