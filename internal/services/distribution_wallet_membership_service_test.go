package services

import (
	"context"
	"testing"

	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	tssSvc "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/services"
)

func Test_DistributionWalletManagementService_memberships(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)
	walletKeyService, err := tssSvc.NewDistributionWalletKeyService(dbConnectionPool, keypair.MustRandom().Seed())
	require.NoError(t, err)
	svc, err := NewDistributionWalletManagementService(models, engine.SubmitterEngine{}, walletKeyService, 5)
	require.NoError(t, err)

	defaultWallet := data.EnsureDefaultDistributionWalletFixture(t, ctx, dbConnectionPool)
	var walletB, archivedWallet string
	require.NoError(t, dbConnectionPool.GetContext(ctx, &walletB, `
		INSERT INTO distribution_wallets (name, distribution_account_type)
		VALUES ('membership-svc-b', 'DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT') RETURNING id`))
	require.NoError(t, dbConnectionPool.GetContext(ctx, &archivedWallet, `
		INSERT INTO distribution_wallets (name, distribution_account_type, status, archived_at)
		VALUES ('membership-svc-archived', 'DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT', 'ARCHIVED', NOW()) RETURNING id`))

	t.Run("grant → list → revoke round trip with grantor attribution", func(t *testing.T) {
		granted, gErr := svc.GrantMembership(ctx, defaultWallet.ID, "user-x", data.InitiatorUserRole, "owner-1")
		require.NoError(t, gErr)
		require.NotNil(t, granted.GrantedBy)
		assert.Equal(t, "owner-1", *granted.GrantedBy)

		memberships, lErr := svc.ListMemberships(ctx, defaultWallet.ID)
		require.NoError(t, lErr)
		require.Len(t, memberships, 1)
		assert.Equal(t, granted.ID, memberships[0].ID)

		require.NoError(t, svc.RevokeMembership(ctx, defaultWallet.ID, granted.ID, "owner-1"))
		memberships, lErr = svc.ListMemberships(ctx, defaultWallet.ID)
		require.NoError(t, lErr)
		assert.Empty(t, memberships)
	})

	t.Run("grants on archived wallets are rejected (→409)", func(t *testing.T) {
		_, gErr := svc.GrantMembership(ctx, archivedWallet, "user-x", data.ApproverUserRole, "owner-1")
		require.ErrorIs(t, gErr, data.ErrWalletArchivedForMembership)
	})

	t.Run("archived wallets' memberships remain listable", func(t *testing.T) {
		// Grant BEFORE archival is the only path; simulate with direct insert (bypass trigger
		// is impossible — instead grant on an active wallet then archive it).
		var tempWallet string
		require.NoError(t, dbConnectionPool.GetContext(ctx, &tempWallet, `
			INSERT INTO distribution_wallets (name, distribution_account_type)
			VALUES ('membership-svc-temp', 'DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT') RETURNING id`))
		granted, gErr := svc.GrantMembership(ctx, tempWallet, "user-y", data.ApproverUserRole, "owner-1")
		require.NoError(t, gErr)
		_, aErr := models.DistributionWallets.Archive(ctx, dbConnectionPool, tempWallet)
		require.NoError(t, aErr)

		memberships, lErr := svc.ListMemberships(ctx, tempWallet)
		require.NoError(t, lErr)
		require.Len(t, memberships, 1)
		assert.Equal(t, granted.ID, memberships[0].ID)
	})

	t.Run("revoking a membership through the wrong wallet is not-found (no disclosure)", func(t *testing.T) {
		granted, gErr := svc.GrantMembership(ctx, defaultWallet.ID, "user-z", data.BusinessUserRole, "owner-1")
		require.NoError(t, gErr)

		rErr := svc.RevokeMembership(ctx, walletB, granted.ID, "owner-1")
		require.ErrorIs(t, rErr, data.ErrRecordNotFound)

		// Still intact on the right wallet.
		memberships, lErr := svc.ListMemberships(ctx, defaultWallet.ID)
		require.NoError(t, lErr)
		require.Len(t, memberships, 1)
	})

	t.Run("unknown wallet is not-found for list and grant", func(t *testing.T) {
		_, lErr := svc.ListMemberships(ctx, "11111111-1111-1111-1111-111111111111")
		require.ErrorIs(t, lErr, data.ErrRecordNotFound)
		_, gErr := svc.GrantMembership(ctx, "11111111-1111-1111-1111-111111111111", "user-x", data.ApproverUserRole, "o")
		require.ErrorIs(t, gErr, data.ErrRecordNotFound)
	})
}
