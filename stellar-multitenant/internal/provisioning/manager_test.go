package provisioning

import (
	"context"
	"errors"
	"fmt"
	"testing"

	migrate "github.com/rubenv/sql-migrate"
	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/network"
	"github.com/stellar/go/protocols/horizon"
	"github.com/stellar/go/strkey"
	"github.com/stellar/go/support/log"
	"github.com/stellar/go/txnbuild"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/slices"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/db/migrations"
	"github.com/stellar/stellar-disbursement-platform-backend/db/router"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	preconditionsMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/preconditions/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

func Test_Manager_ProvisionNewTenant(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	messengerClientMock := message.MessengerClientMock{}
	messengerClientMock.
		On("SendMessage", mock.AnythingOfType("message.Message")).
		Return(nil)

	tenantManager := tenant.NewManager(tenant.WithDatabase(dbConnectionPool))

	pubnetAssets := []string{"USDC:GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN", "XLM:"}
	testnetAssets := []string{"USDC:GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5", "XLM:"}

	pubnetWallets := []string{"Freedom Wallet", "Vibrant Assist RC", "Vibrant Assist"}
	testnetWallets := []string{"Demo Wallet", "Vibrant Assist"}

	user := struct {
		FirstName string
		LastName  string
		Email     string
		OrgName   string
	}{
		FirstName: "First",
		LastName:  "Last",
		Email:     "email@email.com",
		OrgName:   "My Org",
	}

	assertFixtures := func(tenantName string, isTestnet bool) {
		tenantDSN, err := router.GetDSNForTenant(dbt.DSN, tenantName)
		require.NoError(t, err)

		tenantDBConnectionPool, err := db.OpenDBConnectionPool(tenantDSN)
		require.NoError(t, err)
		defer tenantDBConnectionPool.Close()

		schemaName := fmt.Sprintf("%s%s", router.SDPSchemaNamePrefix, tenantName)
		assert.True(t, tenant.CheckSchemaExistsFixture(t, ctx, tenantDBConnectionPool, schemaName))

		tenant.TenantSchemaMatchTablesFixture(t, ctx, tenantDBConnectionPool, schemaName, getExpectedTablesAfterMigrationsApplied())

		assets := pubnetAssets
		wallets := pubnetWallets
		if isTestnet {
			assets = testnetAssets
			wallets = testnetWallets
		}

		tenant.AssertRegisteredAssetsFixture(t, ctx, tenantDBConnectionPool, assets)
		tenant.AssertRegisteredWalletsFixture(t, ctx, tenantDBConnectionPool, wallets)

		tenant.AssertRegisteredUserFixture(t, ctx, tenantDBConnectionPool, user.FirstName, user.LastName, user.Email)

		tenant.TenantSchemaMatchTablesFixture(t, context.Background(), tenantDBConnectionPool, schemaName, getExpectedTablesAfterMigrationsApplied())
	}

	provisionAndValidateNewTenant := func(
		tenantName string,
		isTestnet bool,
		sigClientType signing.SignatureClientType,
	) {
		require.True(t, slices.Contains(signing.DistributionSignatureClientTypes(), sigClientType))

		networkType := utils.PubnetNetworkType
		networkPassphrase := network.PublicNetworkPassphrase
		if isTestnet {
			networkType = utils.TestnetNetworkType
			networkPassphrase = network.TestNetworkPassphrase
		}

		mHorizonClient := &horizonclient.MockClient{}
		mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)

		sigService, _, _, hostAccSigClient, distAccResolver := signing.NewMockSignatureService(t)

		distAcc := keypair.MustRandom()
		distAccPrivKey := distAcc.Seed()
		distAccPubKey := distAcc.Address()
		distAccSigClient, err := signing.NewSignatureClient(signing.DistributionAccountEnvSignatureClientType, signing.SignatureClientOptions{
			NetworkPassphrase:      networkPassphrase,
			DistributionPrivateKey: distAccPrivKey,
		})
		require.NoError(t, err)

		if sigClientType == signing.DistributionAccountDBSignatureClientType {
			distAccSigClient, err = signing.NewSignatureClient(signing.DistributionAccountDBSignatureClientType, signing.SignatureClientOptions{
				NetworkPassphrase:           networkPassphrase,
				DistAccEncryptionPassphrase: keypair.MustRandom().Seed(),
				DBConnectionPool:            dbConnectionPool,
			})
			require.NoError(t, err)
		}

		if sigClientType == signing.DistributionAccountEnvSignatureClientType {
			assert.IsType(t, &signing.DistributionAccountEnvSignatureClient{}, distAccSigClient)
		} else {
			assert.IsType(t, &signing.DistributionAccountDBSignatureClient{}, distAccSigClient)
		}
		sigService.DistAccountSigner = distAccSigClient

		tenantAcc := keypair.MustRandom()
		tenantAccPubKey := tenantAcc.Address()
		distAccResolver.On("HostDistributionAccount").Return(distAccPubKey, nil).Once()

		if sigClientType == signing.DistributionAccountDBSignatureClientType {
			mHorizonClient.
				On("AccountDetail", horizonclient.AccountRequest{AccountID: distAccPubKey}).
				Return(horizon.Account{
					AccountID: distAccPubKey,
					Sequence:  1,
				}, nil).
				Once()
			hostAccSigClient.On(
				"SignStellarTransaction",
				ctx,
				mock.AnythingOfType("*txnbuild.Transaction"),
				distAccPubKey).Return(&txnbuild.Transaction{}, nil).Once()
			mHorizonClient.On(
				"SubmitTransactionWithOptions",
				mock.AnythingOfType("*txnbuild.Transaction"),
				horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true},
			).Return(horizon.Transaction{}, nil).Once()
			mHorizonClient.
				On("AccountDetail", mock.AnythingOfType("horizonclient.AccountRequest")).
				Return(horizon.Account{
					AccountID: tenantAccPubKey,
					Sequence:  1,
				}, nil).
				Once()
		}

		submitterEngine := engine.SubmitterEngine{
			HorizonClient:       mHorizonClient,
			SignatureService:    sigService,
			LedgerNumberTracker: mLedgerNumberTracker,
			MaxBaseFee:          100 * txnbuild.MinBaseFee,
		}

		p := NewManager(
			WithDatabase(dbConnectionPool),
			WithMessengerClient(&messengerClientMock),
			WithTenantManager(tenantManager),
			WithSubmitterEngine(submitterEngine),
			WithNativeAssetBootstrapAmount(tenant.MinTenantDistributionAccountAmount),
		)

		//uiBaseURL := "http://localhost:3000"
		tnt, err := p.ProvisionNewTenant(ctx, tenantName, user.FirstName, user.LastName, user.Email, user.OrgName, string(networkType))
		require.NoError(t, err)

		assert.Equal(t, tenantName, tnt.Name)
		//assert.Equal(t, uiBaseURL, *tnt.SDPUIBaseURL)
		if sigClientType == signing.DistributionAccountEnvSignatureClientType {
			assert.Equal(t, distAcc.Address(), *tnt.DistributionAccount)
		} else {
			assert.True(t, strkey.IsValidEd25519PublicKey(*tnt.DistributionAccount))
		}
		assert.Equal(t, tenant.ProvisionedTenantStatus, tnt.Status)

		mHorizonClient.AssertExpectations(t)
	}

	t.Run("provision a new tenant for the testnet", func(t *testing.T) {
		tenantName1 := "myorg-ukraine"
		tenantName2 := "myorg-poland"

		t.Run("provision key using type DISTRIBUTION_ACCOUNT_ENV", func(t *testing.T) {
			getEntries := log.DefaultLogger.StartTest(log.WarnLevel)
			provisionAndValidateNewTenant(tenantName1, true, signing.DistributionAccountEnvSignatureClientType)
			entries := getEntries()
			require.Len(t, entries, 2)
			assert.Contains(t, entries[0].Message, "Account provisioning not needed for distribution account signature client type")

			assertFixtures(tenantName1, true)
		})

		t.Run("provision key using type DISTRIBUTION_ACCOUNT_DB", func(t *testing.T) {
			provisionAndValidateNewTenant(tenantName2, true, signing.DistributionAccountDBSignatureClientType)
			assertFixtures(tenantName2, true)
		})
	})

	t.Run("provision a new tenant for the pubnet", func(t *testing.T) {
		tenantName1 := "myorg-us"
		tenantName2 := "myorg-canada"

		t.Run("provision key using type DISTRIBUTION_ACCOUNT_ENV", func(t *testing.T) {
			getEntries := log.DefaultLogger.StartTest(log.WarnLevel)
			provisionAndValidateNewTenant(tenantName1, false, signing.DistributionAccountEnvSignatureClientType)
			entries := getEntries()
			require.Len(t, entries, 2)
			assert.Contains(t, entries[0].Message, "Account provisioning not needed for distribution account signature client type")

			assertFixtures(tenantName1, false)
		})

		t.Run("provision key using type DISTRIBUTION_ACCOUNT_DB", func(t *testing.T) {
			provisionAndValidateNewTenant(tenantName2, false, signing.DistributionAccountDBSignatureClientType)
			assertFixtures(tenantName2, false)
		})
	})

	messengerClientMock.AssertExpectations(t)
}

func Test_Manager_RunMigrationsForTenant(t *testing.T) {
	dbt := dbtest.OpenWithAdminMigrationsOnly(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	m := tenant.NewManager(tenant.WithDatabase(dbConnectionPool))
	tnt1, err := m.AddTenant(ctx, "myorg1")
	require.NoError(t, err)
	tnt2, err := m.AddTenant(ctx, "myorg2")
	require.NoError(t, err)

	tnt1SchemaName := fmt.Sprintf("sdp_%s", tnt1.Name)
	tnt2SchemaName := fmt.Sprintf("sdp_%s", tnt2.Name)

	// Creating DB Schemas
	_, err = dbConnectionPool.ExecContext(ctx, fmt.Sprintf("CREATE SCHEMA %s", tnt1SchemaName))
	require.NoError(t, err)
	_, err = dbConnectionPool.ExecContext(ctx, fmt.Sprintf("CREATE SCHEMA %s", tnt2SchemaName))
	require.NoError(t, err)

	// Tenant 1 DB connection
	tenant1DSN, err := router.GetDSNForTenant(dbt.DSN, tnt1.Name)
	require.NoError(t, err)

	tenant1DB, err := db.OpenDBConnectionPool(tenant1DSN)
	require.NoError(t, err)
	defer tenant1DB.Close()

	// Tenant 2 DB connection
	tenant2DSN, err := router.GetDSNForTenant(dbt.DSN, tnt2.Name)
	require.NoError(t, err)

	tenant2DB, err := db.OpenDBConnectionPool(tenant2DSN)
	require.NoError(t, err)
	defer tenant2DB.Close()

	p := NewManager(WithDatabase(tenant1DB))
	err = p.runMigrationsForTenant(ctx, tenant1DSN, migrate.Up, 0, migrations.SDPMigrationRouter)
	require.NoError(t, err)
	err = p.runMigrationsForTenant(ctx, tenant1DSN, migrate.Up, 0, migrations.AuthMigrationRouter)
	require.NoError(t, err)

	tenant.TenantSchemaMatchTablesFixture(t, ctx, dbConnectionPool, tnt1SchemaName, getExpectedTablesAfterMigrationsApplied())

	// Asserting if the Tenant 2 DB Schema wasn't affected by Tenant 1 schema migrations
	tenant.TenantSchemaMatchTablesFixture(t, ctx, dbConnectionPool, tnt2SchemaName, []string{})

	err = p.runMigrationsForTenant(ctx, tenant2DSN, migrate.Up, 0, migrations.SDPMigrationRouter)
	require.NoError(t, err)
	err = p.runMigrationsForTenant(ctx, tenant2DSN, migrate.Up, 0, migrations.AuthMigrationRouter)
	require.NoError(t, err)

	tenant.TenantSchemaMatchTablesFixture(t, ctx, dbConnectionPool, tnt2SchemaName, getExpectedTablesAfterMigrationsApplied())
}

func getExpectedTablesAfterMigrationsApplied() []string {
	return []string{
		"assets",
		"auth_migrations",
		"auth_user_mfa_codes",
		"auth_user_password_reset",
		"auth_users",
		"countries",
		"disbursements",
		"sdp_migrations",
		"messages",
		"organizations",
		"payments",
		"receiver_verifications",
		"receiver_wallets",
		"receivers",
		"wallets",
		"wallets_assets",
	}
}

func Test_Manager_RollbackOnErrors(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	messengerClientMock := message.MessengerClientMock{}
	messengerClientMock.
		On("SendMessage", mock.AnythingOfType("message.Message")).
		Return(nil)

	mHorizonClient := &horizonclient.MockClient{}
	mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)
	sigService, _, distAccSigClient, _, _ := signing.NewMockSignatureService(t)

	submitterEngine := engine.SubmitterEngine{
		HorizonClient:       mHorizonClient,
		SignatureService:    sigService,
		LedgerNumberTracker: mLedgerNumberTracker,
		MaxBaseFee:          100 * txnbuild.MinBaseFee,
	}

	tenantManagerMock := tenant.TenantManagerMock{}
	tenantManager := NewManager(
		WithDatabase(dbConnectionPool),
		WithMessengerClient(&messengerClientMock),
		WithTenantManager(&tenantManagerMock),
		WithSubmitterEngine(submitterEngine),
		WithNativeAssetBootstrapAmount(tenant.MinTenantDistributionAccountAmount),
	)

	tenantName := "myorg1"
	orgName := "My Org"
	firstName := "First"
	lastName := "Last"
	email := "first.last@email.com"
	uiBaseURL := "http://localhost:3000"
	networkType := utils.TestnetNetworkType

	tenantDSN, err := router.GetDSNForTenant(dbt.DSN, tenantName)

	require.NoError(t, err)

	testCases := []struct {
		name             string
		mockTntManagerFn func(tntManagerMock *tenant.TenantManagerMock)
		expectedErr      error
	}{
		{
			name: "return error when failing to add tenant",
			mockTntManagerFn: func(tntManagerMock *tenant.TenantManagerMock) {
				tntManagerMock.On("AddTenant", ctx, tenantName).Return(nil, errors.New("foobar")).Once()
			},
			expectedErr: ErrTenantCreationFailed,
		},
		{
			name: "rollback and return error when failing to create tenant schema",
			mockTntManagerFn: func(tntManagerMock *tenant.TenantManagerMock) {
				tntManagerMock.On("AddTenant", ctx, tenantName).
					Return(&tenant.Tenant{Name: tenantName, ID: "abc"}, nil).Once()
				tntManagerMock.On("GetDSNForTenant", ctx, tenantName).Return(tenantDSN, nil).Once()
				tntManagerMock.On("CreateTenantSchema", ctx, tenantName).Return(errors.New("foobar")).Once()

				// expected rollback operations
				tntManagerMock.On("DropTenantSchema", ctx, tenantName).Return(nil).Once()
				tntManagerMock.On("DeleteTenantByName", ctx, tenantName).Return(nil).Once()
			},
			expectedErr: ErrTenantSchemaFailed,
		},
		{
			name: "rollback and return error when failing to update tenant record",
			mockTntManagerFn: func(tntManagerMock *tenant.TenantManagerMock) {
				_, err = dbConnectionPool.ExecContext(ctx, fmt.Sprintf("CREATE SCHEMA %s", fmt.Sprintf("sdp_%s", tenantName)))
				require.NoError(t, err)

				tnt := tenant.Tenant{Name: tenantName, ID: "abc"}
				tntManagerMock.On("AddTenant", ctx, tenantName).
					Return(&tnt, nil).Once()
				tntManagerMock.On("GetDSNForTenant", ctx, tenantName).Return(tenantDSN, nil).Once()
				tntManagerMock.On("CreateTenantSchema", ctx, tenantName).Return(nil).Once()
				distAcc := keypair.MustRandom().Address()
				distAccSigClient.On("BatchInsert", ctx, 1).Return([]string{distAcc}, nil)

				tStatus := tenant.ProvisionedTenantStatus
				tnt.DistributionAccount = &distAcc
				tntManagerMock.On("UpdateTenantConfig", ctx, &tenant.TenantUpdate{
					ID:                  tnt.ID,
					DistributionAccount: &distAcc,
					SDPUIBaseURL:        &uiBaseURL,
					Status:              &tStatus,
				}).Return(&tnt, errors.New("foobar")).Once()

				// expected rollback operations
				tntManagerMock.On("DropTenantSchema", ctx, tenantName).Return(nil).Once()
				tntManagerMock.On("DeleteTenantByName", ctx, tenantName).Return(nil).Once()
				distAccSigClient.On("Delete", ctx, distAcc).Return(nil).Once()
			},
			expectedErr: ErrUpdateTenantFailed,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.mockTntManagerFn(&tenantManagerMock)

			_, err := tenantManager.ProvisionNewTenant(ctx, tenantName, firstName, lastName, email, orgName, string(networkType))
			if tc.expectedErr != nil {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.expectedErr.Error())
			} else {
				require.NoError(t, err)
			}
		})

		tenantManagerMock.AssertExpectations(t)
	}
}
