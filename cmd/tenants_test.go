package cmd

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

func Test_TenantService_ensureDefaultTenant(t *testing.T) {
	ctx := context.Background()
	mockTenantManager := tenant.NewTenantManagerMock(t)
	
	// Set up test cases
	testCases := []struct {
		name                string
		setupMocks          func(*mockTenantManager, *mockDBMigrator, *mockAuthManager, db.DBConnectionPool)
		expectError         bool
		expectedErrorString string
	}{
		{
			name: "default tenant already exists",
			setupMocks: func(tm *mockTenantManager, dbm *mockDBMigrator, am *mockAuthManager, dbcp db.DBConnectionPool) {
				defaultTenant := &tenant.Tenant{
					ID:   "tenant-123",
					Name: "default",
				}
				tm.On("GetDefault", ctx).Return(defaultTenant, nil).Once()
			},
			expectError: false,
		},
		{
			name: "multiple default tenants error",
			setupMocks: func(tm *mockTenantManager, dbm *mockDBMigrator, am *mockAuthManager, dbcp db.DBConnectionPool) {
				tm.On("GetDefault", ctx).Return(nil, tenant.ErrTooManyDefaultTenants).Once()
			},
			expectError:         true,
			expectedErrorString: "multiple default tenants found; please resolve manually",
		},
		{
			name: "tenant exists but not set as default",
			setupMocks: func(tm *mockTenantManager, dbm *mockDBMigrator, am *mockAuthManager, dbcp db.DBConnectionPool) {
				existingTenant := &tenant.Tenant{
					ID:   "tenant-123",
					Name: "default",
				}
				defaultTenant := &tenant.Tenant{
					ID:        "tenant-123",
					Name:      "default",
					IsDefault: true,
				}

				tm.On("GetDefault", ctx).Return(nil, tenant.ErrTenantDoesNotExist).Once()
				tm.On("GetTenantByName", ctx, "default").Return(existingTenant, nil).Once()
				tm.On("SetDefault", ctx, dbcp, existingTenant.ID).Return(defaultTenant, nil).Once()
			},
			expectError: false,
		},
		{
			name: "create new tenant and set as default",
			setupMocks: func(tm *mockTenantManager, dbm *mockDBMigrator, am *mockAuthManager, dbcp db.DBConnectionPool) {
				// Create a keypair for a test distribution account
				kp, _ := keypair.Random()
				distributionAccount := kp.Address()

				// No default tenant exists
				tm.On("GetDefault", ctx).Return(nil, tenant.ErrTenantDoesNotExist).Once()

				// No tenant named "default" exists
				tm.On("GetTenantByName", ctx, "default").Return(nil, tenant.ErrTenantDoesNotExist).Once()

				// Create a new tenant
				newTenant := &tenant.Tenant{
					ID:   "tenant-123",
					Name: "default",
				}
				tm.On("AddTenant", ctx, "default").Return(newTenant, nil).Once()

				// Create tenant schema
				tm.On("CreateTenantSchema", ctx, newTenant.Name).Return(nil).Once()

				// Get DSN for tenant
				tm.On("GetDSNForTenant", ctx, newTenant.Name).Return("postgres://tenant:password@localhost:5432/sdp_default", nil).Once()

				// Update tenant config
				updatedTenant := &tenant.Tenant{
					ID:                      "tenant-123",
					Name:                    "default",
					Status:                  tenant.ProvisionedTenantStatus,
					DistributionAccountType: schema.DistributionAccountStellarDBVault,
				}

				// We need to capture the actual tenant update to verify it
				tm.On("UpdateTenantConfig", ctx, mock.MatchedBy(func(tu *tenant.TenantUpdate) bool {
					return tu.ID == newTenant.ID &&
						*tu.Status == tenant.ProvisionedTenantStatus &&
						tu.DistributionAccountType == schema.DistributionAccountStellarDBVault
				})).Return(updatedTenant, nil).Once()

				// Set as default
				defaultTenant := &tenant.Tenant{
					ID:        "tenant-123",
					Name:      "default",
					IsDefault: true,
					Status:    tenant.ProvisionedTenantStatus,
				}
				tm.On("SetDefault", ctx, dbcp, newTenant.ID).Return(defaultTenant, nil).Once()
			},
			expectError: false,
		},
		{
			name: "error creating tenant",
			setupMocks: func(tm *mockTenantManager, dbm *mockDBMigrator, am *mockAuthManager, dbcp db.DBConnectionPool) {
				// No default tenant exists
				tm.On("GetDefault", ctx).Return(nil, tenant.ErrTenantDoesNotExist).Once()

				// No tenant named "default" exists
				tm.On("GetTenantByName", ctx, "default").Return(nil, tenant.ErrTenantDoesNotExist).Once()

				// Create a new tenant - Error
				tm.On("AddTenant", ctx, "default").Return(nil, fmt.Errorf("database error")).Once()
			},
			expectError:         true,
			expectedErrorString: "error creating default tenant: database error",
		},
	}

	// Create a stub DB connection pool
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	// Run test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create mocks
			mockTM := &mockTenantManager{}
			mockDBM := &mockDBMigrator{}
			mockAM := &mockAuthManager{}

			// Setup mocks
			tc.setupMocks(mockTM, mockDBM, mockAM, dbConnectionPool)

			// Create service
			service := TenantService{
				tenantManager:           mockTM,
				dbConnectionPool:        dbConnectionPool,
				ownerEmail:              "admin@example.com",
				ownerFirstName:          "Admin",
				ownerLastName:           "User",
				distributionAccountType: schema.DistributionAccountStellarDBVault,
			}

			// Call function
			err := service.ensureDefaultTenant(ctx)

			// Check results
			if tc.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErrorString)
			} else {
				require.NoError(t, err)
			}

			// Verify all expectations were met
			mockTM.AssertExpectations(t)
			mockDBM.AssertExpectations(t)
			mockAM.AssertExpectations(t)
		})
	}
}

func Test_TenantsCmdService_EnsureDefaultTenant(t *testing.T) {
	ctx := context.Background()

	// Create mock service for verification
	mockService := &TenantService{}

	// Create the command service
	cmdService := &TenantsCmdService{}

	// Call the function with the mock service
	err := cmdService.EnsureDefaultTenant(ctx, *mockService)

	// Verify it calls through to the service
	require.NoError(t, err)
}
