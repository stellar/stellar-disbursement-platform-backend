package data

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/lib/pq"
	migrate "github.com/rubenv/sql-migrate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/db/migrations"
	"github.com/stellar/stellar-disbursement-platform-backend/db/router"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

func Test_BridgeIntegrationInsert_Validate(t *testing.T) {
	testCases := []struct {
		name          string
		insert        BridgeIntegrationInsert
		expectedError string
	}{
		{
			name: "returns error if KYCLinkID is empty string",
			insert: BridgeIntegrationInsert{
				KYCLinkID:  utils.StringPtr(""),
				CustomerID: "customer-123",
				OptedInBy:  "user@example.com",
			},
			expectedError: "KYCLinkID is empty",
		},
		{
			name: "returns error if CustomerID is empty",
			insert: BridgeIntegrationInsert{
				KYCLinkID: utils.StringPtr("kyc-link-123"),
				OptedInBy: "user@example.com",
			},
			expectedError: "CustomerID is required",
		},
		{
			name: "returns error if OptedInBy is empty",
			insert: BridgeIntegrationInsert{
				KYCLinkID:  utils.StringPtr("kyc-link-123"),
				CustomerID: "customer-123",
			},
			expectedError: "OptedInBy is required",
		},
		{
			name: "🎉 validation passes with all required fields",
			insert: BridgeIntegrationInsert{
				KYCLinkID:  utils.StringPtr("kyc-link-123"),
				CustomerID: "customer-123",
				OptedInBy:  "user@example.com",
			},
			expectedError: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.insert.Validate()

			if tc.expectedError != "" {
				assert.EqualError(t, err, tc.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_BridgeIntegrationModel_Get(t *testing.T) {
	dbConnectionPool := SetupDBCP(t)
	ctx := context.Background()
	m := &BridgeIntegrationModel{dbConnectionPool: dbConnectionPool}

	t.Run("returns ErrRecordNotFound when no record exists", func(t *testing.T) {
		integration, err := m.Get(ctx)
		assert.ErrorIs(t, err, ErrRecordNotFound)
		assert.Nil(t, integration)
	})

	t.Run("🎉 successfully retrieves bridge integration record", func(t *testing.T) {
		insertData := BridgeIntegrationInsert{
			KYCLinkID:  utils.StringPtr("kyc-link-123"),
			CustomerID: "customer-123",
			OptedInBy:  "user@example.com",
		}

		insertedIntegration, err := m.Insert(ctx, dbConnectionPool, insertData)
		require.NoError(t, err)
		defer deleteBridgeIntegrationFixture(t, ctx, dbConnectionPool)

		fetchedIntegration, err := m.Get(ctx)
		assert.NoError(t, err)
		assert.NotNil(t, fetchedIntegration)

		assert.Equal(t, insertedIntegration.Status, fetchedIntegration.Status)
		assert.Equal(t, insertedIntegration.KYCLinkID, fetchedIntegration.KYCLinkID)
		assert.Equal(t, insertedIntegration.CustomerID, fetchedIntegration.CustomerID)
		assert.Equal(t, insertedIntegration.OptedInBy, fetchedIntegration.OptedInBy)
		assert.NotEmpty(t, fetchedIntegration.CreatedAt)
		assert.NotEmpty(t, fetchedIntegration.UpdatedAt)
	})
}

func Test_BridgeIntegrationModel_Insert(t *testing.T) {
	dbConnectionPool := SetupDBCP(t)

	ctx := context.Background()
	m := &BridgeIntegrationModel{dbConnectionPool: dbConnectionPool}

	t.Run("returns error when validation fails", func(t *testing.T) {
		insert := BridgeIntegrationInsert{
			CustomerID: "customer-123",
		}

		integration, err := m.Insert(ctx, dbConnectionPool, insert)
		assert.Error(t, err)
		assert.ErrorContains(t, err, "OptedInBy is required")
		assert.Nil(t, integration)
	})

	t.Run("🎉 successfully inserts bridge integration record", func(t *testing.T) {
		insert := BridgeIntegrationInsert{
			KYCLinkID:  utils.StringPtr("kyc-link-123"),
			CustomerID: "customer-123",
			OptedInBy:  "user@example.com",
		}

		integration, err := m.Insert(ctx, dbConnectionPool, insert)
		assert.NoError(t, err)
		assert.NotNil(t, integration)
		defer deleteBridgeIntegrationFixture(t, ctx, dbConnectionPool)

		assert.Equal(t, BridgeIntegrationStatusOptedIn, integration.Status)
		assert.Equal(t, insert.KYCLinkID, integration.KYCLinkID)
		assert.Equal(t, &insert.CustomerID, integration.CustomerID)
		assert.Equal(t, &insert.OptedInBy, integration.OptedInBy)
		assert.NotNil(t, integration.OptedInAt)
		assert.NotEmpty(t, integration.CreatedAt)
		assert.NotEmpty(t, integration.UpdatedAt)
		assert.Nil(t, integration.VirtualAccountID)
		assert.Nil(t, integration.VirtualAccountCreatedBy)
		assert.Nil(t, integration.VirtualAccountCreatedAt)
		assert.Nil(t, integration.ErrorMessage)
	})

	t.Run("database constraint prevents duplicate records", func(t *testing.T) {
		insert := BridgeIntegrationInsert{
			KYCLinkID:  utils.StringPtr("kyc-link-456"),
			CustomerID: "customer-456",
			OptedInBy:  "user2@example.com",
		}

		_, err := m.Insert(ctx, dbConnectionPool, insert)
		assert.NoError(t, err)
		defer deleteBridgeIntegrationFixture(t, ctx, dbConnectionPool)

		_, err = m.Insert(ctx, dbConnectionPool, insert)
		assert.Error(t, err)
		assert.ErrorContains(t, err, "idx_bridge_integration_singleton")
	})
}

func Test_BridgeIntegrationModel_Update(t *testing.T) {
	dbConnectionPool := SetupDBCP(t)

	ctx := context.Background()
	m := &BridgeIntegrationModel{dbConnectionPool: dbConnectionPool}

	setupIntegration := func(t *testing.T) *BridgeIntegration {
		insert := BridgeIntegrationInsert{
			KYCLinkID:  utils.StringPtr("kyc-link-789"),
			CustomerID: "customer-789",
			OptedInBy:  "user3@example.com",
		}
		integration, err := m.Insert(ctx, dbConnectionPool, insert)
		require.NoError(t, err)
		return integration
	}

	t.Run("returns error when no fields to update", func(t *testing.T) {
		setupIntegration(t)
		defer deleteBridgeIntegrationFixture(t, ctx, dbConnectionPool)

		update := BridgeIntegrationUpdate{}
		updatedIntegration, err := m.Update(ctx, update)
		assert.Error(t, err)
		assert.EqualError(t, err, "no fields to update")
		assert.Nil(t, updatedIntegration)
	})

	t.Run("returns ErrRecordNotFound when no record exists", func(t *testing.T) {
		status := BridgeIntegrationStatusError
		update := BridgeIntegrationUpdate{
			Status: &status,
		}

		integration, err := m.Update(ctx, update)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrRecordNotFound)
		assert.Nil(t, integration)
	})

	t.Run("🎉 successfully updates status to ERROR", func(t *testing.T) {
		originalIntegration := setupIntegration(t)
		defer deleteBridgeIntegrationFixture(t, ctx, dbConnectionPool)

		newStatus := BridgeIntegrationStatusError
		errorMessage := "Test error message"
		update := BridgeIntegrationUpdate{
			Status:       &newStatus,
			ErrorMessage: &errorMessage,
		}

		updatedIntegration, err := m.Update(ctx, update)
		assert.NoError(t, err)
		assert.NotNil(t, updatedIntegration)

		assert.Equal(t, newStatus, updatedIntegration.Status)
		assert.Equal(t, &errorMessage, updatedIntegration.ErrorMessage)
		assert.Equal(t, originalIntegration.KYCLinkID, updatedIntegration.KYCLinkID)
		assert.Equal(t, originalIntegration.CustomerID, updatedIntegration.CustomerID)
		assert.True(t, updatedIntegration.UpdatedAt.After(originalIntegration.UpdatedAt))
	})

	t.Run("🎉 successfully updates to READY_FOR_DEPOSIT with all required fields", func(t *testing.T) {
		setupIntegration(t)
		defer deleteBridgeIntegrationFixture(t, ctx, dbConnectionPool)

		virtualAccountID := "virtual-account-123"
		virtualAccountCreatedBy := "system@example.com"
		virtualAccountCreatedAt := time.Now()
		status := BridgeIntegrationStatusReadyForDeposit

		update := BridgeIntegrationUpdate{
			Status:                  &status,
			VirtualAccountID:        &virtualAccountID,
			VirtualAccountCreatedBy: &virtualAccountCreatedBy,
			VirtualAccountCreatedAt: &virtualAccountCreatedAt,
		}

		updatedIntegration, err := m.Update(ctx, update)
		assert.NoError(t, err)
		assert.NotNil(t, updatedIntegration)

		assert.Equal(t, status, updatedIntegration.Status)
		assert.Equal(t, &virtualAccountID, updatedIntegration.VirtualAccountID)
		assert.Equal(t, &virtualAccountCreatedBy, updatedIntegration.VirtualAccountCreatedBy)
		assert.NotNil(t, updatedIntegration.VirtualAccountCreatedAt)
		assert.Equal(t, virtualAccountCreatedAt.Unix(), updatedIntegration.VirtualAccountCreatedAt.Unix())
	})

	t.Run("returns error when READY_FOR_DEPOSIT constraints are violated", func(t *testing.T) {
		setupIntegration(t)
		defer deleteBridgeIntegrationFixture(t, ctx, dbConnectionPool)

		status := BridgeIntegrationStatusReadyForDeposit
		update := BridgeIntegrationUpdate{
			Status: &status,
		}

		updatedIntegration, err := m.Update(ctx, update)
		assert.Error(t, err)
		assert.ErrorContains(t, err, "bridge_integration_ready_for_deposit_check")
		assert.Nil(t, updatedIntegration)
	})

	t.Run("returns error when ERROR status constraint is violated", func(t *testing.T) {
		setupIntegration(t)
		defer deleteBridgeIntegrationFixture(t, ctx, dbConnectionPool)

		status := BridgeIntegrationStatusError
		update := BridgeIntegrationUpdate{
			Status: &status,
		}

		updatedIntegration, err := m.Update(ctx, update)
		assert.Error(t, err)
		assert.ErrorContains(t, err, "bridge_integration_error_check")
		assert.Nil(t, updatedIntegration)
	})

	t.Run("🎉 successfully updates error message", func(t *testing.T) {
		setupIntegration(t)
		defer deleteBridgeIntegrationFixture(t, ctx, dbConnectionPool)

		errorMessage := "Failed to create virtual account"
		status := BridgeIntegrationStatusError

		update := BridgeIntegrationUpdate{
			Status:       &status,
			ErrorMessage: &errorMessage,
		}

		updatedIntegration, err := m.Update(ctx, update)
		assert.NoError(t, err)
		assert.NotNil(t, updatedIntegration)

		assert.Equal(t, status, updatedIntegration.Status)
		assert.Equal(t, &errorMessage, updatedIntegration.ErrorMessage)
	})

	t.Run("🎉 successfully updates all optional fields", func(t *testing.T) {
		setupIntegration(t)
		defer deleteBridgeIntegrationFixture(t, ctx, dbConnectionPool)

		newKYCLinkID := "new-kyc-link-id"
		newCustomerID := "new-customer-id"
		newOptedInBy := "newuser@example.com"
		newOptedInAt := time.Now().Add(-1 * time.Hour)
		virtualAccountID := "virtual-account-456"
		virtualAccountCreatedBy := "admin@example.com"
		virtualAccountCreatedAt := time.Now()
		errorMessage := "Test error message"
		status := BridgeIntegrationStatusError

		update := BridgeIntegrationUpdate{
			Status:                  &status,
			KYCLinkID:               &newKYCLinkID,
			CustomerID:              &newCustomerID,
			OptedInBy:               &newOptedInBy,
			OptedInAt:               &newOptedInAt,
			VirtualAccountID:        &virtualAccountID,
			VirtualAccountCreatedBy: &virtualAccountCreatedBy,
			VirtualAccountCreatedAt: &virtualAccountCreatedAt,
			ErrorMessage:            &errorMessage,
		}

		updatedIntegration, err := m.Update(ctx, update)
		assert.NoError(t, err)
		assert.NotNil(t, updatedIntegration)

		assert.Equal(t, status, updatedIntegration.Status)
		assert.Equal(t, &newKYCLinkID, updatedIntegration.KYCLinkID)
		assert.Equal(t, &newCustomerID, updatedIntegration.CustomerID)
		assert.Equal(t, &newOptedInBy, updatedIntegration.OptedInBy)
		assert.NotNil(t, updatedIntegration.OptedInAt)
		assert.Equal(t, &virtualAccountID, updatedIntegration.VirtualAccountID)
		assert.Equal(t, &virtualAccountCreatedBy, updatedIntegration.VirtualAccountCreatedBy)
		assert.NotNil(t, updatedIntegration.VirtualAccountCreatedAt)
		assert.Equal(t, &errorMessage, updatedIntegration.ErrorMessage)
	})
}

func Test_BridgeIntegrationStatus_Constants(t *testing.T) {
	t.Run("verify status constants", func(t *testing.T) {
		assert.Equal(t, BridgeIntegrationStatus("NOT_ENABLED"), BridgeIntegrationStatusNotEnabled)
		assert.Equal(t, BridgeIntegrationStatus("NOT_OPTED_IN"), BridgeIntegrationStatusNotOptedIn)
		assert.Equal(t, BridgeIntegrationStatus("OPTED_IN"), BridgeIntegrationStatusOptedIn)
		assert.Equal(t, BridgeIntegrationStatus("READY_FOR_DEPOSIT"), BridgeIntegrationStatusReadyForDeposit)
		assert.Equal(t, BridgeIntegrationStatus("ERROR"), BridgeIntegrationStatusError)
	})
}

func Test_BridgeIntegrationStatus_Flow(t *testing.T) {
	dbConnectionPool := SetupDBCP(t)

	ctx := context.Background()
	m := &BridgeIntegrationModel{dbConnectionPool: dbConnectionPool}

	t.Run("🎉 demonstrates typical status flow", func(t *testing.T) {
		insert := BridgeIntegrationInsert{
			KYCLinkID:  utils.StringPtr("flow-kyc-link"),
			CustomerID: "flow-customer",
			OptedInBy:  "flow@example.com",
		}

		integration, err := m.Insert(ctx, dbConnectionPool, insert)
		require.NoError(t, err)
		defer deleteBridgeIntegrationFixture(t, ctx, dbConnectionPool)

		assert.Equal(t, BridgeIntegrationStatusOptedIn, integration.Status)

		readyStatus := BridgeIntegrationStatusReadyForDeposit
		virtualAccountID := "virtual-123"
		virtualAccountCreatedBy := "system"
		virtualAccountCreatedAt := time.Now()

		update := BridgeIntegrationUpdate{
			Status:                  &readyStatus,
			VirtualAccountID:        &virtualAccountID,
			VirtualAccountCreatedBy: &virtualAccountCreatedBy,
			VirtualAccountCreatedAt: &virtualAccountCreatedAt,
		}

		updatedIntegration, err := m.Update(ctx, update)
		assert.NoError(t, err)

		assert.Equal(t, BridgeIntegrationStatusReadyForDeposit, updatedIntegration.Status)
		assert.Equal(t, &virtualAccountID, updatedIntegration.VirtualAccountID)
		assert.Equal(t, &virtualAccountCreatedBy, updatedIntegration.VirtualAccountCreatedBy)
		assert.NotNil(t, updatedIntegration.VirtualAccountCreatedAt)

		errorStatus := BridgeIntegrationStatusError
		errorMessage := "Something went wrong"
		errorUpdate := BridgeIntegrationUpdate{
			Status:       &errorStatus,
			ErrorMessage: &errorMessage,
		}

		errorIntegration, err := m.Update(ctx, errorUpdate)
		assert.NoError(t, err)

		assert.Equal(t, BridgeIntegrationStatusError, errorIntegration.Status)
		assert.Equal(t, &errorMessage, errorIntegration.ErrorMessage)
	})
}

func deleteBridgeIntegrationFixture(t *testing.T, ctx context.Context, dbConnectionPool db.DBConnectionPool) {
	t.Helper()

	query := "DELETE FROM bridge_integration"
	_, err := dbConnectionPool.ExecContext(ctx, query)
	require.NoError(t, err)
}

func Test_BridgeIntegrationModel_OtherTenantSchemaFor(t *testing.T) {
	// Cleanups run last-in first-out, so every pool closes before the test database is dropped.
	dbt := dbtest.Open(t)
	t.Cleanup(func() { dbt.Close() })
	adminPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	t.Cleanup(func() { adminPool.Close() })
	ctx := context.Background()

	lookup := &BridgeIntegrationModel{dbConnectionPool: adminPool}

	// The tenant package imports this one, so its fixtures are out of reach here.
	createTenantSchema := func(t *testing.T, tenantName string, withMigrations bool) *Models {
		t.Helper()
		schemaName := fmt.Sprintf("sdp_%s", tenantName)
		_, execErr := adminPool.ExecContext(ctx, fmt.Sprintf("CREATE SCHEMA %s", pq.QuoteIdentifier(schemaName)))
		require.NoError(t, execErr)
		if !withMigrations {
			return nil
		}

		dsn, dsnErr := router.GetDSNForTenant(dbt.DSN, tenantName)
		require.NoError(t, dsnErr)
		_, migrateErr := db.Migrate(dsn, migrate.Up, 0, migrations.SDPMigrationRouter)
		require.NoError(t, migrateErr)

		pool, poolErr := db.OpenDBConnectionPool(dsn)
		require.NoError(t, poolErr)
		t.Cleanup(func() { pool.Close() })
		models, modelsErr := NewModels(pool)
		require.NoError(t, modelsErr)
		return models
	}

	t.Run("returns ErrRecordNotFound when no tenant schema exists", func(t *testing.T) {
		holder, lookupErr := lookup.OtherTenantSchemaFor(ctx, adminPool, "customer-123", "sdp_caller")
		require.ErrorIs(t, lookupErr, ErrRecordNotFound)
		assert.Empty(t, holder)
	})

	modelsA := createTenantSchema(t, "tenanta", true)
	modelsB := createTenantSchema(t, "tenant-b", true) // needs identifier quoting
	createTenantSchema(t, "unmigrated", false)

	_, err = modelsA.BridgeIntegration.Insert(ctx, modelsA.DBConnectionPool, BridgeIntegrationInsert{CustomerID: "customer-a", OptedInBy: "user-a"})
	require.NoError(t, err)
	_, err = modelsB.BridgeIntegration.Insert(ctx, modelsB.DBConnectionPool, BridgeIntegrationInsert{CustomerID: "customer-b", OptedInBy: "user-b"})
	require.NoError(t, err)

	t.Run("returns ErrRecordNotFound when no tenant holds the customer", func(t *testing.T) {
		holder, lookupErr := lookup.OtherTenantSchemaFor(ctx, adminPool, "customer-unknown", "sdp_caller")
		require.ErrorIs(t, lookupErr, ErrRecordNotFound)
		assert.Empty(t, holder)
	})

	t.Run("returns the schema of the tenant holding the customer", func(t *testing.T) {
		holder, lookupErr := lookup.OtherTenantSchemaFor(ctx, adminPool, "customer-a", "sdp_caller")
		require.NoError(t, lookupErr)
		assert.Equal(t, "sdp_tenanta", holder)

		holder, lookupErr = lookup.OtherTenantSchemaFor(ctx, adminPool, "customer-b", "sdp_tenanta")
		require.NoError(t, lookupErr)
		assert.Equal(t, "sdp_tenant-b", holder)
	})

	t.Run("ignores the excluded schema", func(t *testing.T) {
		holder, lookupErr := lookup.OtherTenantSchemaFor(ctx, adminPool, "customer-a", "sdp_tenanta")
		require.ErrorIs(t, lookupErr, ErrRecordNotFound)
		assert.Empty(t, holder)
	})

	t.Run("runs on the given transaction", func(t *testing.T) {
		holder, lookupErr := db.RunInTransactionWithResult(ctx, adminPool, nil, func(dbTx db.DBTransaction) (string, error) {
			return lookup.OtherTenantSchemaFor(ctx, dbTx, "customer-a", "sdp_caller")
		})
		require.NoError(t, lookupErr)
		assert.Equal(t, "sdp_tenanta", holder)
	})
}
