package circle

import (
	"context"
	"testing"

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
	clientConfigModel.Upsert(ctx, ClientConfigUpdate{})
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
