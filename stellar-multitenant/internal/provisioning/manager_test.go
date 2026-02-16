package provisioning

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stellar/go-stellar-sdk/clients/horizonclient"
	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/network"
	"github.com/stellar/go-stellar-sdk/protocols/horizon"
	"github.com/stellar/go-stellar-sdk/support/log"
	"github.com/stellar/go-stellar-sdk/txnbuild"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/db/migrations"
	"github.com/stellar/stellar-disbursement-platform-backend/db/router"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	preconditionsMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/preconditions/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing/mocks"
	sdpUtils "github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

func Test_NewManager(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	mTenantMenager := &tenant.TenantManagerMock{}

	mHorizonClient := &horizonclient.MockClient{}
	mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)
	sigService, _, _ := signing.NewMockSignatureService(t)
	submitterEngine := engine.SubmitterEngine{
		HorizonClient:       mHorizonClient,
		SignatureService:    sigService,
		LedgerNumberTracker: mLedgerNumberTracker,
		MaxBaseFee:          100 * txnbuild.MinBaseFee,
	}

	testCases := []struct {
		name            string
		opts            ManagerOptions
		wantErrContains string
		wantResult      *Manager
	}{
		{
			name:            "DBConnectionPool cannot be nil",
			wantErrContains: "database connection pool cannot be nil",
		},
		{
			name: "TenantManager cannot be nil",
			opts: ManagerOptions{
				DBConnectionPool: dbConnectionPool,
			},
			wantErrContains: "tenant manager cannot be nil",
		},
		{
			name: "validating SubmitterEngine",
			opts: ManagerOptions{
				DBConnectionPool: dbConnectionPool,
				TenantManager:    mTenantMenager,
				SubmitterEngine:  engine.SubmitterEngine{},
			},
			wantErrContains: "validating submitter engine",
		},
		{
			name: "fails if XLM < MINIMUM",
			opts: ManagerOptions{
				DBConnectionPool:           dbConnectionPool,
				TenantManager:              mTenantMenager,
				SubmitterEngine:            submitterEngine,
				NativeAssetBootstrapAmount: tenant.MinTenantDistributionAccountAmount - 1,
			},
			wantErrContains: "the amount of XLM configured (4 XLM) is outside the permitted range",
		},
		{
			name: "fails if XLM > MAXIMUM",
			opts: ManagerOptions{
				DBConnectionPool:           dbConnectionPool,
				TenantManager:              mTenantMenager,
				SubmitterEngine:            submitterEngine,
				NativeAssetBootstrapAmount: tenant.MaxTenantDistributionAccountAmount + 1,
			},
			wantErrContains: "the amount of XLM configured (51 XLM) is outside the permitted range",
		},
		{
			name: "🎉 successfully creates a new manager",
			opts: ManagerOptions{
				DBConnectionPool:           dbConnectionPool,
				TenantManager:              mTenantMenager,
				SubmitterEngine:            submitterEngine,
				NativeAssetBootstrapAmount: tenant.MinTenantDistributionAccountAmount,
			},
			wantResult: &Manager{
				db:                         dbConnectionPool,
				tenantManager:              mTenantMenager,
				SubmitterEngine:            submitterEngine,
				nativeAssetBootstrapAmount: tenant.MinTenantDistributionAccountAmount,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotResult, err := NewManager(tc.opts)
			if tc.wantErrContains != "" {
				assert.ErrorContains(t, err, tc.wantErrContains)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.wantResult, gotResult)
			}
		})
	}
}

func Test_Manager_ProvisionNewTenant(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	userFirstName := "First"
	userLastName := "Last"
	userEmail := "email@email.com"
	userOrgName := "My Org"
	sdpUIBaseURL := "https://sdp-ui.stellar.org"
	baseURL := "https://sdp-api.stellar.org"

	tenantManager := tenant.NewManager(tenant.WithDatabase(dbConnectionPool))

	testCases := []struct {
		name              string
		networkPassphrase string
		tenantName        string
		accountType       schema.AccountType
	}{
		{
			name:              "[Testnet] accountType=DISTRIBUTION_ACCOUNT.STELLAR.ENV",
			networkPassphrase: network.TestNetworkPassphrase,
			tenantName:        "testnet-stellar-env",
			accountType:       schema.DistributionAccountStellarEnv,
		},
		{
			name:              "[Testnet] accountType=DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT",
			networkPassphrase: network.TestNetworkPassphrase,
			tenantName:        "testnet-stellar-dbvault",
			accountType:       schema.DistributionAccountStellarDBVault,
		},
		{
			name:              "[Testnet] accountType=DISTRIBUTION_ACCOUNT.CIRCLE.DB_VAULT",
			networkPassphrase: network.TestNetworkPassphrase,
			tenantName:        "testnet-circle-dbvault",
			accountType:       schema.DistributionAccountCircleDBVault,
		},
		{
			name:              "[Pubnet] accountType=DISTRIBUTION_ACCOUNT.STELLAR.ENV",
			networkPassphrase: network.PublicNetworkPassphrase,
			tenantName:        "pubnet-stellar-env",
			accountType:       schema.DistributionAccountStellarEnv,
		},
		{
			name:              "[Pubnet] accountType=DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT",
			networkPassphrase: network.PublicNetworkPassphrase,
			tenantName:        "pubnet-stellar-dbvault",
			accountType:       schema.DistributionAccountStellarDBVault,
		},
		{
			name:              "[Pubnet] accountType=DISTRIBUTION_ACCOUNT.CIRCLE.DB_VAULT",
			networkPassphrase: network.PublicNetworkPassphrase,
			tenantName:        "pubnet-circle-dbvault",
			accountType:       schema.DistributionAccountCircleDBVault,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer tenant.DeleteAllTenantsFixture(t, ctx, dbConnectionPool)

			hostAccountKP := keypair.MustRandom()
			hostAccount := schema.NewDefaultHostAccount(hostAccountKP.Address())
			var wantDistAccAddress string

			// STEP 1: create mocks:
			mHorizonClient := &horizonclient.MockClient{}

			chAccSigClient := mocks.NewMockSignatureClient(t)
			chAccSigClient.On("NetworkPassphrase").Return(tc.networkPassphrase).Maybe()

			hostAccSigClient := mocks.NewMockSignatureClient(t)
			hostAccSigClient.On("NetworkPassphrase").Return(tc.networkPassphrase).Maybe()

			distAccResolver := mocks.NewMockDistributionAccountResolver(t)
			distAccResolver.On("HostDistributionAccount").Return(hostAccount).Maybe()

			signatureStrategies := map[schema.AccountType]signing.SignatureClient{
				schema.HostStellarEnv:          hostAccSigClient,
				schema.ChannelAccountStellarDB: chAccSigClient,
			}

			// STEP 2: create sigRouter
			switch tc.accountType {
			case schema.DistributionAccountCircleDBVault:
				t.Log(tc.accountType)

			case schema.DistributionAccountStellarEnv:
				distAccSigClient, err := signing.NewSignatureClient(schema.DistributionAccountStellarEnv, signing.SignatureClientOptions{
					DistributionPrivateKey: hostAccountKP.Seed(),
					NetworkPassphrase:      tc.networkPassphrase,
				})
				require.NoError(t, err)
				wantDistAccAddress = hostAccountKP.Address()

				signatureStrategies[tc.accountType] = distAccSigClient

				mHorizonClient.
					On("AccountDetail", horizonclient.AccountRequest{AccountID: hostAccountKP.Address()}).
					Return(horizon.Account{
						AccountID: hostAccountKP.Address(),
						Sequence:  1,
					}, nil).
					Once()

				mHorizonClient.
					On("SubmitTransactionWithOptions", mock.AnythingOfType("*txnbuild.Transaction"), horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true}).
					Return(horizon.Transaction{}, nil).
					Once()

			case schema.DistributionAccountStellarDBVault:
				distAccSigClient, err := signing.NewSignatureClient(schema.DistributionAccountStellarDBVault, signing.SignatureClientOptions{
					DBConnectionPool:            dbConnectionPool,
					DistAccEncryptionPassphrase: keypair.MustRandom().Seed(),
					NetworkPassphrase:           tc.networkPassphrase,
				})
				require.NoError(t, err)

				tenantAccountKP := keypair.MustRandom()

				// STEP 2.1 - Mock calls that are exclusively for DistributionAccountStellarDBVault
				mHorizonClient.
					On("AccountDetail", horizonclient.AccountRequest{AccountID: hostAccountKP.Address()}).
					Return(horizon.Account{
						AccountID: hostAccountKP.Address(),
						Sequence:  1,
					}, nil).
					Once()
				hostAccSigClient.
					On("SignStellarTransaction", ctx, mock.AnythingOfType("*txnbuild.Transaction"), hostAccountKP.Address()).
					Return(&txnbuild.Transaction{}, nil).
					Once()
				mHorizonClient.
					On("SubmitTransactionWithOptions", mock.AnythingOfType("*txnbuild.Transaction"), horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true}).
					Return(horizon.Transaction{}, nil).
					Times(2)
				mHorizonClient.
					On("AccountDetail", mock.AnythingOfType("horizonclient.AccountRequest")).
					Run(func(args mock.Arguments) {
						gotAccountRequest, ok := args.Get(0).(horizonclient.AccountRequest)
						require.True(t, ok)
						wantDistAccAddress = gotAccountRequest.AccountID // <--- this is the distribution account address
					}).
					Return(horizon.Account{
						AccountID: tenantAccountKP.Address(),
						Sequence:  1,
					}, nil).
					Times(2)

				signatureStrategies[tc.accountType] = distAccSigClient

			default:
				require.Failf(t, "invalid sigClientType=%s", string(tc.accountType))
			}

			sigRouter := signing.NewSignerRouterImpl(network.TestNetworkPassphrase, signatureStrategies)

			// STEP 3: create Submitter Engine
			mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)
			submitterEngine := engine.SubmitterEngine{
				HorizonClient: mHorizonClient,
				SignatureService: signing.SignatureService{
					SignerRouter:                &sigRouter,
					DistributionAccountResolver: distAccResolver,
				},
				LedgerNumberTracker: mLedgerNumberTracker,
				MaxBaseFee:          100 * txnbuild.MinBaseFee,
			}

			// STEP 4: create provisioning Manager
			p, err := NewManager(ManagerOptions{
				DBConnectionPool:           dbConnectionPool,
				TenantManager:              tenantManager,
				SubmitterEngine:            submitterEngine,
				NativeAssetBootstrapAmount: tenant.MinTenantDistributionAccountAmount,
			})
			require.NoError(t, err)

			// STEP 5: provision the tenant
			networkType, err := sdpUtils.GetNetworkTypeFromNetworkPassphrase(tc.networkPassphrase)
			require.NoError(t, err)
			tnt, err := p.ProvisionNewTenant(ctx, ProvisionTenant{
				Name:                    tc.tenantName,
				UserFirstName:           userFirstName,
				UserLastName:            userLastName,
				UserEmail:               userEmail,
				OrgName:                 userOrgName,
				NetworkType:             string(networkType),
				UIBaseURL:               sdpUIBaseURL,
				BaseURL:                 baseURL,
				DistributionAccountType: tc.accountType,
			})
			require.NoError(t, err)

			// STEP 6: assert the result
			assert.Equal(t, tc.tenantName, tnt.Name)
			assert.Equal(t, schema.ProvisionedTenantStatus, tnt.Status)
			assert.Equal(t, sdpUIBaseURL, *tnt.SDPUIBaseURL)
			assert.Equal(t, baseURL, *tnt.BaseURL)
			switch tc.accountType {
			case schema.DistributionAccountStellarEnv:
				assert.Equal(t, hostAccountKP.Address(), *tnt.DistributionAccountAddress)
				assert.Equal(t, wantDistAccAddress, *tnt.DistributionAccountAddress)
			case schema.DistributionAccountStellarDBVault:
				assert.NotEqual(t, hostAccountKP.Address(), *tnt.DistributionAccountAddress)
				assert.Equal(t, wantDistAccAddress, *tnt.DistributionAccountAddress)
			case schema.DistributionAccountCircleDBVault:
				assert.Nil(t, tnt.DistributionAccountAddress)
			default:
				require.Failf(t, "invalid accountType=%s", string(tc.accountType))
			}

			// STEP 7: assert the mocks
			mHorizonClient.AssertExpectations(t)

			// STEP 8: assert the fixtures
			tenantDSN, err := router.GetDSNForTenant(dbt.DSN, tc.tenantName)
			require.NoError(t, err)
			tenantDBConnectionPool, err := db.OpenDBConnectionPool(tenantDSN)
			require.NoError(t, err)
			defer tenantDBConnectionPool.Close()

			// STEP 8.1: assert the schema
			schemaName := fmt.Sprintf("%s%s", router.SDPSchemaNamePrefix, tc.tenantName)
			assert.True(t, tenant.CheckSchemaExistsFixture(t, ctx, tenantDBConnectionPool, schemaName))

			// STEP 8.2: assert the tables exist in the schema
			tenant.TenantSchemaMatchTablesFixture(t, ctx, tenantDBConnectionPool, schemaName, getExpectedTablesAfterMigrationsApplied())

			// STEP 8.3: assert the user has been registered
			tenant.AssertRegisteredUserFixture(t, ctx, tenantDBConnectionPool, userFirstName, userLastName, userEmail)

			// STEP 8.4: assert the assets have been registered
			assetsSlice, ok := services.AssetsNetworkByPlatformMap[tc.accountType.Platform()][networkType]
			require.True(t, ok)
			var assetsStrSlice []string
			for _, asset := range assetsSlice {
				assetsStrSlice = append(assetsStrSlice, fmt.Sprintf("%s:%s", asset.Code, asset.Issuer))
			}
			tenant.AssertRegisteredAssetsFixture(t, ctx, tenantDBConnectionPool, assetsStrSlice)

			// STEP 8.5: assert the wallets have been registered
			walletsSlice, ok := services.DefaultWalletsNetworkMap[networkType]
			require.True(t, ok)
			var walletsNames []string
			for _, wallet := range walletsSlice {
				walletsNames = append(walletsNames, wallet.Name)
			}
			tenant.AssertRegisteredWalletsFixture(t, ctx, tenantDBConnectionPool, walletsNames)
		})
	}
}

func Test_Manager_applyTenantMigrations(t *testing.T) {
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

	mHorizonClient := &horizonclient.MockClient{}
	mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)
	sigService, _, _ := signing.NewMockSignatureService(t)
	submitterEngine := engine.SubmitterEngine{
		HorizonClient:       mHorizonClient,
		SignatureService:    sigService,
		LedgerNumberTracker: mLedgerNumberTracker,
		MaxBaseFee:          100 * txnbuild.MinBaseFee,
	}
	p, err := NewManager(ManagerOptions{
		DBConnectionPool:           dbConnectionPool,
		TenantManager:              &tenant.TenantManagerMock{},
		SubmitterEngine:            submitterEngine,
		NativeAssetBootstrapAmount: tenant.MinTenantDistributionAccountAmount,
	})
	require.NoError(t, err)
	err = p.applyTenantMigrations(ctx, tenant1DSN, migrations.SDPMigrationRouter)
	require.NoError(t, err)
	err = p.applyTenantMigrations(ctx, tenant1DSN, migrations.AuthMigrationRouter)
	require.NoError(t, err)

	tenant.TenantSchemaMatchTablesFixture(t, ctx, dbConnectionPool, tnt1SchemaName, getExpectedTablesAfterMigrationsApplied())

	// Asserting if the Tenant 2 DB Schema wasn't affected by Tenant 1 schema migrations
	tenant.TenantSchemaMatchTablesFixture(t, ctx, dbConnectionPool, tnt2SchemaName, []string{})

	err = p.applyTenantMigrations(ctx, tenant2DSN, migrations.SDPMigrationRouter)
	require.NoError(t, err)
	err = p.applyTenantMigrations(ctx, tenant2DSN, migrations.AuthMigrationRouter)
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
		"bridge_integration",
		"circle_client_config",
		"circle_recipients",
		"circle_transfer_requests",
		"disbursements",
		"embedded_wallets",
		"messages",
		"organizations",
		"passkey_sessions",
		"payments",
		"receiver_verifications",
		"receiver_verifications_audit",
		"receiver_wallets",
		"receiver_wallets_audit",
		"receivers",
		"receivers_audit",
		"sdp_migrations",
		"short_urls",
		"sponsored_transactions",
		"wallets",
		"wallets_assets",
		"receiver_registration_attempts",
		"sep_nonces",
		"api_keys",
		"api_keys_audit",
	}
}

func Test_Manager_RollbackOnErrors(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	accountType := schema.DistributionAccountStellarDBVault

	tenantName := "myorg1"
	orgName := "My Org"
	firstName := "First"
	lastName := "Last"
	email := "first.last@email.com"
	networkType := sdpUtils.TestnetNetworkType
	tnt := schema.Tenant{Name: tenantName, ID: "abc"}
	sdpUIBaseURL := "https://sdp-ui.stellar.org"
	baseURL := "https://sdp-api.stellar.org"

	tenantDSN, err := router.GetDSNForTenant(dbt.DSN, tenantName)
	require.NoError(t, err)

	testCases := []struct {
		name             string
		mockTntManagerFn func(tntManagerMock *tenant.TenantManagerMock, sigRouter *mocks.MockSignerRouter, mDistAccResolver *mocks.MockDistributionAccountResolver, mHorizonClient *horizonclient.MockClient)
		expectedErr      error
	}{
		{
			name: "when AddTenant fails return an error",
			mockTntManagerFn: func(tntManagerMock *tenant.TenantManagerMock, _ *mocks.MockSignerRouter, _ *mocks.MockDistributionAccountResolver, _ *horizonclient.MockClient) {
				// needed for AddTenant:
				tntManagerMock.On("AddTenant", ctx, tenantName).Return(nil, errors.New("foobar")).Once()
			},
			expectedErr: ErrTenantCreationFailed,
		},
		{
			name: "when createSchemaAndRunMigrations fails, rollback and return an error",
			mockTntManagerFn: func(tntManagerMock *tenant.TenantManagerMock, _ *mocks.MockSignerRouter, _ *mocks.MockDistributionAccountResolver, _ *horizonclient.MockClient) {
				// Needed for AddTenant:
				tntManagerMock.On("AddTenant", ctx, tenantName).Return(&tnt, nil).Once()

				// Needed for createSchemaAndRunMigrations:
				tntManagerMock.On("GetDSNForTenant", ctx, tenantName).Return(tenantDSN, nil).Once()
				tntManagerMock.On("CreateTenantSchema", ctx, tenantName).Return(errors.New("foobar")).Once()

				// ROLLBACK: [tenant_creation, schema_creation]
				tntManagerMock.On("DropTenantSchema", ctx, tenantName).Return(nil).Once()
				tntManagerMock.On("DeleteTenantByName", ctx, tenantName).Return(nil).Once()
			},
			expectedErr: ErrTenantSchemaFailed,
		},
		{
			name: "when UpdateTenantConfig fails, rollback and return an error",
			mockTntManagerFn: func(tntManagerMock *tenant.TenantManagerMock, sigRouter *mocks.MockSignerRouter, _ *mocks.MockDistributionAccountResolver, _ *horizonclient.MockClient) {
				// Needed for AddTenant:
				tntManagerMock.On("AddTenant", ctx, tenantName).Return(&tnt, nil).Once()

				// Needed for createSchemaAndRunMigrations:
				tntManagerMock.On("GetDSNForTenant", ctx, tenantName).Return(tenantDSN, nil).Once()
				tntManagerMock.On("CreateTenantSchema", ctx, tenantName).Return(nil).Once()

				// Needed for setupTenantData (this one cannot be mocked):
				_, err = dbConnectionPool.ExecContext(ctx, fmt.Sprintf("CREATE SCHEMA %s", fmt.Sprintf("sdp_%s", tenantName)))
				require.NoError(t, err)

				// Needed for provisionDistributionAccount:
				distAccAddress := keypair.MustRandom().Address()
				distAccount := schema.TransactionAccount{
					Address: distAccAddress,
					Type:    accountType,
					Status:  schema.AccountStatusActive,
				}
				sigRouter.
					On("BatchInsert", ctx, accountType, 1).
					Return([]schema.TransactionAccount{distAccount}, nil)

				// Needed for UpdateTenantConfig:
				tStatus := schema.ProvisionedTenantStatus
				updatedTnt := tnt
				tntManagerMock.
					On("UpdateTenantConfig", ctx, &tenant.TenantUpdate{
						ID:                         updatedTnt.ID,
						DistributionAccountAddress: distAccAddress,
						DistributionAccountType:    accountType,
						DistributionAccountStatus:  schema.AccountStatusActive,
						Status:                     &tStatus,
						SDPUIBaseURL:               &sdpUIBaseURL,
						BaseURL:                    &baseURL,
					}).
					Return(nil, errors.New("foobar")).
					Once()

				// ROLLBACK: [tenant_creation, schema_creation, distribution_account_creation]
				tntManagerMock.On("DropTenantSchema", ctx, tenantName).Return(nil).Once()
				tntManagerMock.On("DeleteTenantByName", ctx, tenantName).Return(nil).Once()
				sigRouter.On("Delete", ctx, distAccount).Return(nil).Once()
			},
			expectedErr: ErrUpdateTenantFailed,
		},
		{
			name: "when fundTenantDistributionStellarAccountIfNeeded fails, rollback and return an error",
			mockTntManagerFn: func(tntManagerMock *tenant.TenantManagerMock, sigRouter *mocks.MockSignerRouter, mDistAccResolver *mocks.MockDistributionAccountResolver, mHorizonClient *horizonclient.MockClient) {
				// Needed for AddTenant:
				tntManagerMock.On("AddTenant", ctx, tenantName).Return(&tnt, nil).Once()

				// Needed for createSchemaAndRunMigrations:
				tntManagerMock.On("GetDSNForTenant", ctx, tenantName).Return(tenantDSN, nil).Once()
				tntManagerMock.On("CreateTenantSchema", ctx, tenantName).Return(nil).Once()

				// Needed for setupTenantData (this one cannot be mocked):
				_, err = dbConnectionPool.ExecContext(ctx, fmt.Sprintf("CREATE SCHEMA %s", fmt.Sprintf("sdp_%s", tenantName)))
				require.NoError(t, err)

				// Needed for provisionDistributionAccount:
				distAccAddress := keypair.MustRandom().Address()
				distAccount := schema.TransactionAccount{
					Address: distAccAddress,
					Type:    accountType,
					Status:  schema.AccountStatusActive,
				}
				sigRouter.
					On("BatchInsert", ctx, accountType, 1).
					Return([]schema.TransactionAccount{distAccount}, nil)

				// Needed for UpdateTenantConfig:
				tStatus := schema.ProvisionedTenantStatus
				updatedTnt := tnt
				updatedTnt.DistributionAccountAddress = &distAccAddress
				updatedTnt.DistributionAccountType = schema.DistributionAccountStellarDBVault
				updatedTnt.DistributionAccountStatus = schema.AccountStatusActive
				updatedTnt.Status = tStatus
				tntManagerMock.
					On("UpdateTenantConfig", ctx, &tenant.TenantUpdate{
						ID:                         updatedTnt.ID,
						DistributionAccountAddress: distAccAddress,
						DistributionAccountType:    accountType,
						DistributionAccountStatus:  schema.AccountStatusActive,
						Status:                     &tStatus,
						SDPUIBaseURL:               &sdpUIBaseURL,
						BaseURL:                    &baseURL,
					}).
					Return(&updatedTnt, nil).
					Once()

				// Needed for fundTenantDistributionStellarAccountIfNeeded:
				hostAccountKP := keypair.MustRandom()
				hostAccount := schema.NewDefaultHostAccount(hostAccountKP.Address())
				mDistAccResolver.
					On("HostDistributionAccount").
					Return(hostAccount)
				mHorizonClient.
					On("AccountDetail", horizonclient.AccountRequest{AccountID: hostAccountKP.Address()}).
					Return(horizon.Account{}, errors.New("some horizon error"))

				// ROLLBACK: [tenant_creation, schema_creation, distribution_account_creation]
				tntManagerMock.On("DropTenantSchema", ctx, tenantName).Return(nil).Once()
				tntManagerMock.On("DeleteTenantByName", ctx, tenantName).Return(nil).Once()
				sigRouter.On("Delete", ctx, distAccount).Return(nil).Once()
			},
			expectedErr: ErrUpdateTenantFailed,
		},
		{
			name: "when provisioning succeeds, no rollback occurs",
			mockTntManagerFn: func(tntManagerMock *tenant.TenantManagerMock, sigRouter *mocks.MockSignerRouter, mDistAccResolver *mocks.MockDistributionAccountResolver, mHorizonClient *horizonclient.MockClient) {
				// Needed for AddTenant:
				tntManagerMock.On("AddTenant", ctx, tenantName).Return(&tnt, nil).Once()

				// Needed for createSchemaAndRunMigrations:
				tntManagerMock.On("GetDSNForTenant", ctx, tenantName).Return(tenantDSN, nil).Twice()
				tntManagerMock.On("CreateTenantSchema", ctx, tenantName).Return(nil).Once()

				// Needed for setupTenantData (this one cannot be mocked):
				_, err = dbConnectionPool.ExecContext(ctx, fmt.Sprintf("CREATE SCHEMA %s", fmt.Sprintf("sdp_%s", tenantName)))
				require.NoError(t, err)

				// Needed for provisionDistributionAccount:
				distAccAddress := keypair.MustRandom().Address()
				distAccount := schema.TransactionAccount{
					Address: distAccAddress,
					Type:    accountType,
					Status:  schema.AccountStatusActive,
				}
				sigRouter.
					On("BatchInsert", ctx, accountType, 1).
					Return([]schema.TransactionAccount{distAccount}, nil)

				// Needed for UpdateTenantConfig:
				tStatus := schema.ProvisionedTenantStatus
				updatedTnt := tnt
				updatedTnt.DistributionAccountAddress = &distAccAddress
				updatedTnt.DistributionAccountType = schema.DistributionAccountStellarDBVault
				updatedTnt.DistributionAccountStatus = schema.AccountStatusActive
				updatedTnt.Status = tStatus
				tntManagerMock.
					On("UpdateTenantConfig", ctx, &tenant.TenantUpdate{
						ID:                         updatedTnt.ID,
						DistributionAccountAddress: distAccAddress,
						DistributionAccountType:    accountType,
						DistributionAccountStatus:  schema.AccountStatusActive,
						Status:                     &tStatus,
						SDPUIBaseURL:               &sdpUIBaseURL,
						BaseURL:                    &baseURL,
					}).
					Return(&updatedTnt, nil).
					Once()

				// Needed for fundTenantDistributionStellarAccountIfNeeded:
				hostAccountKP := keypair.MustRandom()
				hostAccount := schema.NewDefaultHostAccount(hostAccountKP.Address())
				mDistAccResolver.
					On("HostDistributionAccount").
					Return(hostAccount)
				mHorizonClient.
					On("AccountDetail", horizonclient.AccountRequest{AccountID: hostAccountKP.Address()}).
					Return(horizon.Account{
						AccountID: hostAccountKP.Address(),
						Sequence:  1,
					}, nil).
					Once()

				sigRouter.
					On("SignStellarTransaction", ctx, mock.AnythingOfType("*txnbuild.Transaction"), hostAccount).
					Return(&txnbuild.Transaction{}, nil).
					Once()
				sigRouter.
					On("SignStellarTransaction", ctx, mock.AnythingOfType("*txnbuild.Transaction"), distAccount).
					Return(&txnbuild.Transaction{}, nil).
					Once()
				mHorizonClient.
					On("SubmitTransactionWithOptions", mock.AnythingOfType("*txnbuild.Transaction"), horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true}).
					Return(horizon.Transaction{}, nil).
					Twice()
				mHorizonClient.
					On("AccountDetail", mock.AnythingOfType("horizonclient.AccountRequest")).
					Return(horizon.Account{
						AccountID: distAccAddress,
						Sequence:  1,
					}, nil).
					Twice()
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer tenant.DeleteAllTenantsFixture(t, ctx, dbConnectionPool)

			// Create Mocks:
			mHorizonClient := &horizonclient.MockClient{}
			mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)
			sigService, sigRouter, distAccResolver := signing.NewMockSignatureService(t)

			tenantManagerMock := &tenant.TenantManagerMock{}
			tc.mockTntManagerFn(tenantManagerMock, sigRouter, distAccResolver, mHorizonClient)

			// Create tenant manager
			provisioningManager, err := NewManager(ManagerOptions{
				DBConnectionPool: dbConnectionPool,
				TenantManager:    tenantManagerMock,
				SubmitterEngine: engine.SubmitterEngine{
					HorizonClient:       mHorizonClient,
					SignatureService:    sigService,
					LedgerNumberTracker: mLedgerNumberTracker,
					MaxBaseFee:          100 * txnbuild.MinBaseFee,
				},
				NativeAssetBootstrapAmount: tenant.MinTenantDistributionAccountAmount,
			})
			require.NoError(t, err)

			// Provision the tenant
			_, err = provisioningManager.ProvisionNewTenant(ctx, ProvisionTenant{
				Name:                    tenantName,
				UserFirstName:           firstName,
				UserLastName:            lastName,
				UserEmail:               email,
				OrgName:                 orgName,
				NetworkType:             string(networkType),
				UIBaseURL:               sdpUIBaseURL,
				BaseURL:                 baseURL,
				DistributionAccountType: accountType,
			})

			// Assertions
			if tc.expectedErr != nil {
				assert.ErrorContains(t, err, tc.expectedErr.Error())
			} else {
				require.NoError(t, err)

				// Verify that the organization has link shortener enabled by default
				tenantSchemaConnectionPool, models, connErr := GetTenantSchemaDBConnectionAndModels(tenantDSN)
				require.NoError(t, connErr)
				defer tenantSchemaConnectionPool.Close()

				org, orgErr := models.Organizations.Get(ctx)
				require.NoError(t, orgErr)
				assert.True(t, org.IsLinkShortenerEnabled, "link shortener should be enabled by default for new tenants")
			}

			mHorizonClient.AssertExpectations(t)
			tenantManagerMock.AssertExpectations(t)
		})
	}
}

func Test_Manager_fundTenantDistributionStellarAccountIfNeeded(t *testing.T) {
	ctx := context.Background()
	distAccAddress := keypair.MustRandom().Address()
	hostAccount := schema.NewDefaultHostAccount(keypair.MustRandom().Address())

	testCases := []struct {
		name              string
		accountType       schema.AccountType
		prepareMocksFn    func(t *testing.T, mDistAccResolver *mocks.MockDistributionAccountResolver, mHorizonClient *horizonclient.MockClient, mSigRouter *mocks.MockSignerRouter)
		wantLogContains   string
		wantErrorContains string
	}{
		{
			name:              "❌ HOST account type.STELLAR.ENV is not supported",
			accountType:       schema.HostStellarEnv,
			wantErrorContains: fmt.Sprintf("unsupported accountType=%s", schema.HostStellarEnv),
		},
		{
			name:              "❌ CHANNEL_ACCOUNT account type.STELLAR.DB is not supported",
			accountType:       schema.ChannelAccountStellarDB,
			wantErrorContains: fmt.Sprintf("unsupported accountType=%s", schema.ChannelAccountStellarDB),
		},
		{
			name:            "🟢✍🏽 DISTRIBUTION_ACCOUNT.STELLAR.ENV is NO-OP and logs warnings accordingly",
			accountType:     schema.DistributionAccountStellarEnv,
			wantLogContains: fmt.Sprintf("Tenant distribution account is configured to use accountType=%s, no need to initiate funding.", schema.DistributionAccountStellarEnv),
		},
		{
			name:        "🟢✅ DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT gets inserted in DBVault",
			accountType: schema.DistributionAccountStellarDBVault,
			prepareMocksFn: func(t *testing.T, mDistAccResolver *mocks.MockDistributionAccountResolver, mHorizonClient *horizonclient.MockClient, mSigRouter *mocks.MockSignerRouter) {
				mDistAccResolver.On("HostDistributionAccount").Return(hostAccount)

				mHorizonClient.
					On("AccountDetail", horizonclient.AccountRequest{AccountID: hostAccount.Address}).
					Return(horizon.Account{
						AccountID: hostAccount.Address,
						Sequence:  1,
					}, nil).
					Once()
				mSigRouter.
					On("SignStellarTransaction", ctx, mock.AnythingOfType("*txnbuild.Transaction"), hostAccount).
					Return(&txnbuild.Transaction{}, nil).
					Once()
				mHorizonClient.
					On("SubmitTransactionWithOptions", mock.AnythingOfType("*txnbuild.Transaction"), horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true}).
					Return(horizon.Transaction{}, nil).
					Once()
				mHorizonClient.
					On("AccountDetail", horizonclient.AccountRequest{AccountID: distAccAddress}).
					Return(horizon.Account{AccountID: distAccAddress, Sequence: 1}, nil).
					Once()
			},
		},
		{
			name:        "❌ DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT errors are handled accordingly",
			accountType: schema.DistributionAccountStellarDBVault,
			prepareMocksFn: func(t *testing.T, mDistAccResolver *mocks.MockDistributionAccountResolver, mHorizonClient *horizonclient.MockClient, mSigRouter *mocks.MockSignerRouter) {
				mDistAccResolver.On("HostDistributionAccount").Return(hostAccount)

				mHorizonClient.
					On("AccountDetail", horizonclient.AccountRequest{AccountID: hostAccount.Address}).
					Return(horizon.Account{}, errors.New("horizon error"))
			},
			wantErrorContains: "bootstrapping tenant distribution account with native asset",
		},
		{
			name:            "🟢✍🏽 DISTRIBUTION_ACCOUNT.CIRCLE.DB_VAULT is NO-OP and logs warnings accordingly",
			accountType:     schema.DistributionAccountCircleDBVault,
			wantLogContains: fmt.Sprintf("Tenant distribution account is configured to use accountType=%s, the tenant will need to complete the setup through the UI.", schema.DistributionAccountCircleDBVault),
		},
		{
			name:              "❌ INVALID account type will return an error",
			accountType:       schema.AccountType("INVALID"),
			wantErrorContains: "unsupported accountType=INVALID",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			m := Manager{}
			tnt := schema.Tenant{
				ID:                         "foo-bar",
				Name:                       "test",
				DistributionAccountAddress: &distAccAddress,
				DistributionAccountType:    tc.accountType,
			}

			getEntries := log.DefaultLogger.StartTest(log.WarnLevel)

			if tc.prepareMocksFn != nil {
				mHorizonClient := &horizonclient.MockClient{}
				defer mHorizonClient.AssertExpectations(t)
				mSigRouter := mocks.NewMockSignerRouter(t)
				mDistAccResolver := mocks.NewMockDistributionAccountResolver(t)
				tc.prepareMocksFn(t, mDistAccResolver, mHorizonClient, mSigRouter)

				m.SubmitterEngine = engine.SubmitterEngine{
					HorizonClient: mHorizonClient,
					SignatureService: signing.SignatureService{
						SignerRouter:                mSigRouter,
						DistributionAccountResolver: mDistAccResolver,
					},
				}
			}

			err := m.fundTenantDistributionStellarAccountIfNeeded(ctx, tnt)
			if tc.wantErrorContains != "" {
				assert.ErrorContains(t, err, tc.wantErrorContains)
			} else {
				require.NoError(t, err)
			}

			entries := getEntries()
			var aggregatedMessages []string
			if tc.wantLogContains != "" {
				for _, entry := range entries {
					aggregatedMessages = append(aggregatedMessages, entry.Message)
				}
				assert.Contains(t, aggregatedMessages, tc.wantLogContains)
			}
		})
	}
}

func Test_Manager_provisionDistributionAccount(t *testing.T) {
	ctx := context.Background()
	distAccAddress := keypair.MustRandom().Address()

	testCases := []struct {
		name              string
		accountType       schema.AccountType
		prepareMocksFn    func(t *testing.T, mSigRouter *mocks.MockSignerRouter)
		wantErrorContains string
		wantLogContains   string
		wantTnt           schema.Tenant
	}{
		{
			name:              "HOST.STELLAR.ENV is not supported",
			accountType:       schema.HostStellarEnv,
			wantTnt:           schema.Tenant{ID: "foo-bar", Name: "test"},
			wantErrorContains: fmt.Sprintf("%v: unsupported accountType=%s", ErrProvisionTenantDistributionAccountFailed, schema.HostStellarEnv),
		},
		{
			name:              "CHANNEL_ACCOUNT.STELLAR.DB is not supported",
			accountType:       schema.ChannelAccountStellarDB,
			wantTnt:           schema.Tenant{ID: "foo-bar", Name: "test"},
			wantErrorContains: fmt.Sprintf("%v: unsupported accountType=%s", ErrProvisionTenantDistributionAccountFailed, schema.ChannelAccountStellarDB),
		},
		{
			name:        "DISTRIBUTION_ACCOUNT.STELLAR.ENV is NO-OP and logs warnings accordingly",
			accountType: schema.DistributionAccountStellarEnv,
			prepareMocksFn: func(t *testing.T, mSigRouter *mocks.MockSignerRouter) {
				distAccount := schema.TransactionAccount{
					Address: distAccAddress,
					Type:    schema.DistributionAccountStellarEnv,
				}
				mSigRouter.On("BatchInsert", ctx, schema.DistributionAccountStellarEnv, 1).
					Return([]schema.TransactionAccount{distAccount}, signing.ErrUnsupportedCommand).
					Once()
			},
			wantTnt: schema.Tenant{
				ID:                         "foo-bar",
				Name:                       "test",
				DistributionAccountAddress: &distAccAddress,
				DistributionAccountType:    schema.DistributionAccountStellarEnv,
				DistributionAccountStatus:  schema.AccountStatusActive,
			},
		},
		{
			name:        "DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT gets inserted in DBVault",
			accountType: schema.DistributionAccountStellarDBVault,
			prepareMocksFn: func(t *testing.T, mSigRouter *mocks.MockSignerRouter) {
				distAccount := schema.TransactionAccount{
					Address: distAccAddress,
					Type:    schema.DistributionAccountStellarDBVault,
				}
				mSigRouter.On("BatchInsert", ctx, schema.DistributionAccountStellarDBVault, 1).
					Return([]schema.TransactionAccount{distAccount}, nil).
					Once()
			},
			wantTnt: schema.Tenant{
				ID:                         "foo-bar",
				Name:                       "test",
				DistributionAccountAddress: &distAccAddress,
				DistributionAccountType:    schema.DistributionAccountStellarDBVault,
				DistributionAccountStatus:  schema.AccountStatusActive,
			},
		},
		{
			name:        "DISTRIBUTION_ACCOUNT.CIRCLE.DB_VAULT is NO-OP and logs warnings accordingly",
			accountType: schema.DistributionAccountCircleDBVault,
			wantTnt: schema.Tenant{
				ID:                        "foo-bar",
				Name:                      "test",
				DistributionAccountType:   schema.DistributionAccountCircleDBVault,
				DistributionAccountStatus: schema.AccountStatusPendingUserActivation,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			m := Manager{}
			tnt := &schema.Tenant{ID: "foo-bar", Name: "test"}

			if tc.prepareMocksFn != nil {
				mSigRouter := mocks.NewMockSignerRouter(t)
				m.SubmitterEngine.SignatureService.SignerRouter = mSigRouter
				tc.prepareMocksFn(t, mSigRouter)
			}

			err := m.provisionDistributionAccount(ctx, tnt, tc.accountType)
			if tc.wantErrorContains != "" {
				assert.ErrorContains(t, err, tc.wantErrorContains)
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, tc.wantTnt, *tnt)
		})
	}
}
