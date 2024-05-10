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
	"github.com/stellar/go/txnbuild"
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
	sigService, _, _, _, _ := signing.NewMockSignatureService(t)
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
		sigClientType     signing.SignatureClientType
	}{
		{
			name:              "Testnet with sigClientType=DISTRIBUTION_ACCOUNT_ENV",
			networkPassphrase: network.TestNetworkPassphrase,
			tenantName:        "tenant-testnet-env",
			sigClientType:     signing.DistributionAccountEnvSignatureClientType,
		},
		{
			name:              "Testnet with sigClientType=DISTRIBUTION_ACCOUNT_DB",
			networkPassphrase: network.TestNetworkPassphrase,
			tenantName:        "tenant-testnet-dbvault",
			sigClientType:     signing.DistributionAccountDBSignatureClientType,
		},
		{
			name:              "Pubnet with sigClientType=DISTRIBUTION_ACCOUNT_ENV",
			networkPassphrase: network.PublicNetworkPassphrase,
			tenantName:        "tenant-pubnet-env",
			sigClientType:     signing.DistributionAccountEnvSignatureClientType,
		},
		{
			name:              "Pubnet with sigClientType=DISTRIBUTION_ACCOUNT_DB",
			networkPassphrase: network.PublicNetworkPassphrase,
			tenantName:        "tenant-pubnet-dbvault",
			sigClientType:     signing.DistributionAccountDBSignatureClientType,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer tenant.DeleteAllTenantsFixture(t, ctx, dbConnectionPool)

			hostAccountKP := keypair.MustRandom()
			var distAccSigClient signing.SignatureClient
			var err error
			var wantDistAccAddress string

			// STEP 1: create mocks:
			mHorizonClient := &horizonclient.MockClient{}

			chAccSigClient := mocks.NewMockSignatureClient(t)
			chAccSigClient.On("NetworkPassphrase").Return(tc.networkPassphrase).Maybe()

			hostAccSigClient := mocks.NewMockSignatureClient(t)
			hostAccSigClient.On("NetworkPassphrase").Return(tc.networkPassphrase).Maybe()

			distAccResolver := mocks.NewMockDistributionAccountResolver(t)
			distAccResolver.On("HostDistributionAccount").Return(hostAccountKP.Address()).Once()

			// STEP 2: create DistSigner
			switch tc.sigClientType {
			case signing.DistributionAccountEnvSignatureClientType:
				distAccSigClient, err = signing.NewSignatureClient(signing.DistributionAccountEnvSignatureClientType, signing.SignatureClientOptions{
					DistributionPrivateKey: hostAccountKP.Seed(),
					NetworkPassphrase:      tc.networkPassphrase,
				})
				wantDistAccAddress = hostAccountKP.Address()
				require.NoError(t, err)

			case signing.DistributionAccountDBSignatureClientType:
				distAccSigClient, err = signing.NewSignatureClient(signing.DistributionAccountDBSignatureClientType, signing.SignatureClientOptions{
					DBConnectionPool:            dbConnectionPool,
					DistAccEncryptionPassphrase: keypair.MustRandom().Seed(),
					NetworkPassphrase:           tc.networkPassphrase,
				})
				require.NoError(t, err)

				tenantAccountKP := keypair.MustRandom()

				// STEP 2.1 - Mock calls that are exclusively for DistributionAccountDBSignatureClientType
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
					Once()
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
					Once()

			default:
				require.Failf(t, "invalid sigClientType=%s", string(tc.sigClientType))
			}

			// STEP 3: create Submitter Engine
			mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)
			submitterEngine := engine.SubmitterEngine{
				HorizonClient: mHorizonClient,
				SignatureService: signing.SignatureService{
					ChAccountSigner:             chAccSigClient,
					DistAccountSigner:           distAccSigClient,
					HostAccountSigner:           hostAccSigClient,
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
				Name:          tc.tenantName,
				UserFirstName: userFirstName,
				UserLastName:  userLastName,
				UserEmail:     userEmail,
				OrgName:       userOrgName,
				NetworkType:   string(networkType),
				UiBaseURL:     sdpUIBaseURL,
				BaseURL:       baseURL,
			})
			require.NoError(t, err)

			// STEP 6: assert the result
			assert.Equal(t, tc.tenantName, tnt.Name)
			assert.Equal(t, tenant.ProvisionedTenantStatus, tnt.Status)
			assert.Equal(t, wantDistAccAddress, *tnt.DistributionAccountAddress)
			assert.Equal(t, sdpUIBaseURL, *tnt.SDPUIBaseURL)
			assert.Equal(t, baseURL, *tnt.BaseURL)
			if tc.sigClientType == signing.DistributionAccountEnvSignatureClientType {
				assert.Equal(t, hostAccountKP.Address(), *tnt.DistributionAccountAddress)
			} else {
				assert.NotEqual(t, hostAccountKP.Address(), *tnt.DistributionAccountAddress)
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
			assetsSlice, ok := services.DefaultAssetsNetworkMap[networkType]
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

	mHorizonClient := &horizonclient.MockClient{}
	mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)
	sigService, _, _, _, _ := signing.NewMockSignatureService(t)
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

	tenantName := "myorg1"
	orgName := "My Org"
	firstName := "First"
	lastName := "Last"
	email := "first.last@email.com"
	networkType := sdpUtils.TestnetNetworkType
	tnt := tenant.Tenant{Name: tenantName, ID: "abc"}
	sdpUIBaseURL := "https://sdp-ui.stellar.org"
	baseURL := "https://sdp-api.stellar.org"

	tenantDSN, err := router.GetDSNForTenant(dbt.DSN, tenantName)
	require.NoError(t, err)

	testCases := []struct {
		name             string
		mockTntManagerFn func(tntManagerMock *tenant.TenantManagerMock, hostAccSigClient, distAccSigClient *mocks.MockSignatureClient, mDistAccResolver *mocks.MockDistributionAccountResolver, mHorizonClient *horizonclient.MockClient)
		expectedErr      error
	}{
		{
			name: "when AddTenant fails return an error",
			mockTntManagerFn: func(tntManagerMock *tenant.TenantManagerMock, _ *mocks.MockSignatureClient, _ *mocks.MockSignatureClient, _ *mocks.MockDistributionAccountResolver, _ *horizonclient.MockClient) {
				// needed for AddTenant:
				tntManagerMock.On("AddTenant", ctx, tenantName).Return(nil, errors.New("foobar")).Once()
			},
			expectedErr: ErrTenantCreationFailed,
		},
		{
			name: "when createSchemaAndRunMigrations fails, rollback and return an error",
			mockTntManagerFn: func(tntManagerMock *tenant.TenantManagerMock, _ *mocks.MockSignatureClient, _ *mocks.MockSignatureClient, _ *mocks.MockDistributionAccountResolver, _ *horizonclient.MockClient) {
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
			mockTntManagerFn: func(tntManagerMock *tenant.TenantManagerMock, hostAccSigClient, distAccSigClient *mocks.MockSignatureClient, _ *mocks.MockDistributionAccountResolver, _ *horizonclient.MockClient) {
				// Needed for AddTenant:
				tntManagerMock.On("AddTenant", ctx, tenantName).Return(&tnt, nil).Once()

				// Needed for createSchemaAndRunMigrations:
				tntManagerMock.On("GetDSNForTenant", ctx, tenantName).Return(tenantDSN, nil).Once()
				tntManagerMock.On("CreateTenantSchema", ctx, tenantName).Return(nil).Once()

				// Needed for setupTenantData (this one cannot be mocked):
				_, err = dbConnectionPool.ExecContext(ctx, fmt.Sprintf("CREATE SCHEMA %s", fmt.Sprintf("sdp_%s", tenantName)))
				require.NoError(t, err)

				// Needed for provisionDistributionAccount:
				distAcc := keypair.MustRandom().Address()
				distAccSigClient.
					On("BatchInsert", ctx, 1).Return([]string{distAcc}, nil).Once().
					On("Type").Return(string(signing.DistributionAccountEnvSignatureClientType))

				// Needed for UpdateTenantConfig:
				tStatus := tenant.ProvisionedTenantStatus
				updatedTnt := tnt
				updatedTnt.DistributionAccountAddress = &distAcc
				tntManagerMock.
					On("UpdateTenantConfig", ctx, &tenant.TenantUpdate{
						ID:                         updatedTnt.ID,
						DistributionAccountAddress: distAcc,
						DistributionAccountType:    schema.DistributionAccountStellarEnv,
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
				distAccSigClient.On("Delete", ctx, distAcc).Return(nil).Once()
			},
			expectedErr: ErrUpdateTenantFailed,
		},
		{
			name: "when fundTenantDistributionAccount fails, rollback and return an error",
			mockTntManagerFn: func(tntManagerMock *tenant.TenantManagerMock, hostAccSigClient, distAccSigClient *mocks.MockSignatureClient, mDistAccResolver *mocks.MockDistributionAccountResolver, mHorizonClient *horizonclient.MockClient) {
				// Needed for AddTenant:
				tntManagerMock.On("AddTenant", ctx, tenantName).Return(&tnt, nil).Once()

				// Needed for createSchemaAndRunMigrations:
				tntManagerMock.On("GetDSNForTenant", ctx, tenantName).Return(tenantDSN, nil).Once()
				tntManagerMock.On("CreateTenantSchema", ctx, tenantName).Return(nil).Once()

				// Needed for setupTenantData (this one cannot be mocked):
				_, err = dbConnectionPool.ExecContext(ctx, fmt.Sprintf("CREATE SCHEMA %s", fmt.Sprintf("sdp_%s", tenantName)))
				require.NoError(t, err)

				// Needed for provisionDistributionAccount:
				distAcc := keypair.MustRandom().Address()
				distAccSigClient.
					On("BatchInsert", ctx, 1).Return([]string{distAcc}, nil).Once().
					On("Type").Return(string(signing.DistributionAccountEnvSignatureClientType))

				// Needed for UpdateTenantConfig:
				tStatus := tenant.ProvisionedTenantStatus
				updatedTnt := tnt
				updatedTnt.DistributionAccountAddress = &distAcc
				updatedTnt.DistributionAccountType = schema.DistributionAccountStellarEnv
				updatedTnt.DistributionAccountStatus = schema.AccountStatusActive
				updatedTnt.Status = tStatus
				tntManagerMock.
					On("UpdateTenantConfig", ctx, &tenant.TenantUpdate{
						ID:                         updatedTnt.ID,
						DistributionAccountAddress: distAcc,
						DistributionAccountType:    schema.DistributionAccountStellarEnv,
						DistributionAccountStatus:  schema.AccountStatusActive,
						Status:                     &tStatus,
						SDPUIBaseURL:               &sdpUIBaseURL,
						BaseURL:                    &baseURL,
					}).
					Return(&updatedTnt, nil).
					Once()

				// Needed for fundTenantDistributionAccount:
				hostAccountKP := keypair.MustRandom()
				mDistAccResolver.On("HostDistributionAccount").Return(hostAccountKP.Address()).Once()
				mHorizonClient.
					On("AccountDetail", horizonclient.AccountRequest{AccountID: hostAccountKP.Address()}).
					Return(horizon.Account{}, errors.New("some horizon error"))

				// ROLLBACK: [tenant_creation, schema_creation, distribution_account_creation]
				tntManagerMock.On("DropTenantSchema", ctx, tenantName).Return(nil).Once()
				tntManagerMock.On("DeleteTenantByName", ctx, tenantName).Return(nil).Once()
				distAccSigClient.On("Delete", ctx, distAcc).Return(nil).Once()
			},
			expectedErr: ErrUpdateTenantFailed,
		},
		{
			name: "when provisioning succeeds, no rollback occurs",
			mockTntManagerFn: func(tntManagerMock *tenant.TenantManagerMock, hostAccSigClient, distAccSigClient *mocks.MockSignatureClient, mDistAccResolver *mocks.MockDistributionAccountResolver, mHorizonClient *horizonclient.MockClient) {
				// Needed for AddTenant:
				tntManagerMock.On("AddTenant", ctx, tenantName).Return(&tnt, nil).Once()

				// Needed for createSchemaAndRunMigrations:
				tntManagerMock.On("GetDSNForTenant", ctx, tenantName).Return(tenantDSN, nil).Once()
				tntManagerMock.On("CreateTenantSchema", ctx, tenantName).Return(nil).Once()

				// Needed for setupTenantData (this one cannot be mocked):
				_, err = dbConnectionPool.ExecContext(ctx, fmt.Sprintf("CREATE SCHEMA %s", fmt.Sprintf("sdp_%s", tenantName)))
				require.NoError(t, err)

				// Needed for provisionDistributionAccount:
				distAcc := keypair.MustRandom().Address()
				distAccSigClient.
					On("BatchInsert", ctx, 1).Return([]string{distAcc}, nil).Once().
					On("Type").Return(string(signing.DistributionAccountEnvSignatureClientType))

				// Needed for UpdateTenantConfig:
				tStatus := tenant.ProvisionedTenantStatus
				updatedTnt := tnt
				updatedTnt.DistributionAccountAddress = &distAcc
				updatedTnt.DistributionAccountType = schema.DistributionAccountStellarEnv
				updatedTnt.DistributionAccountStatus = schema.AccountStatusActive
				updatedTnt.Status = tStatus
				tntManagerMock.
					On("UpdateTenantConfig", ctx, &tenant.TenantUpdate{
						ID:                         updatedTnt.ID,
						DistributionAccountAddress: distAcc,
						DistributionAccountType:    schema.DistributionAccountStellarEnv,
						DistributionAccountStatus:  schema.AccountStatusActive,
						Status:                     &tStatus,
						SDPUIBaseURL:               &sdpUIBaseURL,
						BaseURL:                    &baseURL,
					}).
					Return(&updatedTnt, nil).
					Once()

				// Needed for fundTenantDistributionAccount:
				hostAccountKP := keypair.MustRandom()
				tenantAccountKP := keypair.MustRandom()
				mDistAccResolver.On("HostDistributionAccount").Return(hostAccountKP.Address()).Once()
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
					Once()
				mHorizonClient.
					On("AccountDetail", mock.AnythingOfType("horizonclient.AccountRequest")).
					Return(horizon.Account{
						AccountID: tenantAccountKP.Address(),
						Sequence:  1,
					}, nil).
					Once()
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer tenant.DeleteAllTenantsFixture(t, ctx, dbConnectionPool)

			// Create Mocks:
			mHorizonClient := &horizonclient.MockClient{}
			mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)
			sigService, _, distAccSigClient, hostAccSigClient, distAccResolver := signing.NewMockSignatureService(t)

			tenantManagerMock := &tenant.TenantManagerMock{}
			tc.mockTntManagerFn(tenantManagerMock, hostAccSigClient, distAccSigClient, distAccResolver, mHorizonClient)

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
				Name:          tenantName,
				UserFirstName: firstName,
				UserLastName:  lastName,
				UserEmail:     email,
				OrgName:       orgName,
				NetworkType:   string(networkType),
				UiBaseURL:     sdpUIBaseURL,
				BaseURL:       baseURL,
			})

			// Assertions
			if tc.expectedErr != nil {
				assert.ErrorContains(t, err, tc.expectedErr.Error())
			} else {
				require.NoError(t, err)
			}

			mHorizonClient.AssertExpectations(t)
			tenantManagerMock.AssertExpectations(t)
		})
	}
}
