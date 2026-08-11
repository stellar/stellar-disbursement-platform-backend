package services

import (
	"context"
	"testing"

	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	migrate "github.com/rubenv/sql-migrate"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"

	"github.com/stellar/stellar-disbursement-platform-backend/db/migrations"
	"github.com/stellar/stellar-disbursement-platform-backend/db/router"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	tssSvc "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/services"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

// Test_DistributionWallet_lifecycle covers the archive-don't-delete, atomic promotion, and
// zero-active-wallets acceptance criteria at the service + DB layers.
func Test_DistributionWallet_lifecycle(t *testing.T) {
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
	// Lifecycle operations never touch the network; an empty engine suffices.
	svc, err := NewDistributionWalletManagementService(models, engine.SubmitterEngine{}, walletKeyService, 5)
	require.NoError(t, err)

	// Seed: default wallet + two extra active wallets (raw inserts; provisioning is covered
	// by the CreateWallet tests).
	defaultWallet := data.EnsureDefaultDistributionWalletFixture(t, ctx, dbConnectionPool)
	insertWallet := func(name string) *data.DistributionWallet {
		w, iErr := models.DistributionWallets.Insert(ctx, dbConnectionPool, data.DistributionWalletInsert{
			Name:        name,
			AccountType: "DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT",
		})
		require.NoError(t, iErr)
		addr := keypair.MustRandom().Address()
		w, iErr = models.DistributionWallets.UpdateAddress(ctx, dbConnectionPool, w.ID, addr)
		require.NoError(t, iErr)
		// Insert leaves the wallet PENDING (see CreateWallet); this suite is about
		// archive/promote, which only operate on ACTIVE wallets.
		w, iErr = models.DistributionWallets.Activate(ctx, dbConnectionPool, w.ID)
		require.NoError(t, iErr)
		return w
	}
	walletB := insertWallet("wallet-b")
	walletC := insertWallet("wallet-c")

	t.Run("archive blocks new disbursements but leaves history intact", func(t *testing.T) {
		// History first: a disbursement sourced from wallet B while it is active.
		existing := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
			SourceWalletID: walletB.ID,
		})

		archived, aErr := svc.ArchiveWallet(ctx, walletB.ID)
		require.NoError(t, aErr)
		assert.Equal(t, data.ArchivedDistributionWalletStatus, archived.Status)
		require.NotNil(t, archived.ArchivedAt)

		// New disbursements from the archived wallet are rejected at the DB layer.
		_, iErr := models.Disbursements.Insert(ctx, dbConnectionPool, &data.Disbursement{
			Name:                    "post-archive disbursement",
			Status:                  data.DraftDisbursementStatus,
			StatusHistory:           []data.DisbursementStatusHistoryEntry{{Status: data.DraftDisbursementStatus}},
			Wallet:                  existing.Wallet,
			Asset:                   existing.Asset,
			VerificationField:       data.VerificationTypeDateOfBirth,
			RegistrationContactType: data.RegistrationContactTypePhone,
			SourceWalletID:          walletB.ID,
		})
		require.Error(t, iErr)
		// The guard rejects any non-ACTIVE status, not ARCHIVED specifically, so the message
		// names the status rather than hardcoding "archived".
		assert.ErrorContains(t, iErr, "is not active")
		assert.ErrorContains(t, iErr, "ARCHIVED")

		// History intact: the pre-archive disbursement still exists and references B.
		got, gErr := models.Disbursements.Get(ctx, dbConnectionPool, existing.ID)
		require.NoError(t, gErr)
		assert.Equal(t, walletB.ID, got.SourceWalletID)

		// Key material is preserved (never hard-deleted once it has history).
		_, kErr := models.DistributionWallets.Get(ctx, dbConnectionPool, walletB.ID)
		require.NoError(t, kErr)

		// Archived wallets are hidden from the operational list, visible in admin view.
		operational, lErr := svc.ListWallets(ctx, false)
		require.NoError(t, lErr)
		for _, w := range operational {
			assert.NotEqual(t, walletB.ID, w.ID)
		}
		all, lErr := svc.ListWallets(ctx, true)
		require.NoError(t, lErr)
		found := false
		for _, w := range all {
			found = found || w.ID == walletB.ID
		}
		assert.True(t, found)
	})

	t.Run("archiving the default wallet is rejected until another is promoted", func(t *testing.T) {
		_, aErr := svc.ArchiveWallet(ctx, defaultWallet.ID)
		require.ErrorIs(t, aErr, ErrCannotArchiveDefaultWallet)

		// Direct DB attempt also rejected (CHECK from the table migration).
		_, dbErr := dbConnectionPool.ExecContext(ctx,
			`UPDATE distribution_wallets SET status = 'ARCHIVED', archived_at = NOW() WHERE id = $1`, defaultWallet.ID)
		require.Error(t, dbErr)
		assert.ErrorContains(t, dbErr, "distribution_wallets_default_must_be_active")
	})

	t.Run("promotion is atomic: success path moves the default pointer", func(t *testing.T) {
		promoted, pErr := svc.PromoteToDefault(ctx, walletC.ID)
		require.NoError(t, pErr)
		assert.True(t, promoted.IsDefault)

		gotDefault, dErr := models.DistributionWallets.GetDefault(ctx, dbConnectionPool)
		require.NoError(t, dErr)
		assert.Equal(t, walletC.ID, gotDefault.ID)

		// Exactly one default exists.
		var defaults int
		require.NoError(t, dbConnectionPool.GetContext(ctx, &defaults,
			`SELECT COUNT(*) FROM distribution_wallets WHERE is_default`))
		assert.Equal(t, 1, defaults)

		// And the old default can now be archived.
		_, aErr := svc.ArchiveWallet(ctx, defaultWallet.ID)
		require.NoError(t, aErr)
	})

	t.Run("promotion partial failure rolls back the demotion", func(t *testing.T) {
		// walletB is ARCHIVED: promoting it must fail AND leave walletC as default —
		// proving the demote step inside the transaction rolled back.
		_, pErr := svc.PromoteToDefault(ctx, walletB.ID)
		require.ErrorIs(t, pErr, ErrCannotPromoteWallet)

		gotDefault, dErr := models.DistributionWallets.GetDefault(ctx, dbConnectionPool)
		require.NoError(t, dErr, "the demotion must have been rolled back — a default must still exist")
		assert.Equal(t, walletC.ID, gotDefault.ID)
	})

	t.Run("no sequence of archive/promote calls reaches zero active wallets", func(t *testing.T) {
		// Current state: walletC = default+ACTIVE (the only active); default+B archived.
		// Exhaust every lever the API offers against the last active wallet:
		_, aErr := svc.ArchiveWallet(ctx, walletC.ID)
		require.ErrorIs(t, aErr, ErrCannotArchiveDefaultWallet, "last active is the default → default rule fires")

		// Re-promote an archived wallet to free C for archival? Promotion of archived is blocked.
		_, pErr := svc.PromoteToDefault(ctx, walletB.ID)
		require.ErrorIs(t, pErr, ErrCannotPromoteWallet)

		// Add a fresh wallet, promote it, archive C, then try to drain to zero.
		walletD := insertWallet("wallet-d")
		_, pErr = svc.PromoteToDefault(ctx, walletD.ID)
		require.NoError(t, pErr)
		_, aErr = svc.ArchiveWallet(ctx, walletC.ID)
		require.NoError(t, aErr)

		// walletD is now the last active wallet: both the service rule and the DB trigger
		// must refuse to archive it, even when bypassing the service.
		_, aErr = svc.ArchiveWallet(ctx, walletD.ID)
		require.ErrorIs(t, aErr, ErrCannotArchiveDefaultWallet)

		_, dbErr := dbConnectionPool.ExecContext(ctx, `
			UPDATE distribution_wallets SET is_default = FALSE WHERE id = $1`, walletD.ID)
		require.NoError(t, dbErr, "demote directly to sidestep the default rule")
		_, dbErr = dbConnectionPool.ExecContext(ctx, `
			UPDATE distribution_wallets SET status = 'ARCHIVED', archived_at = NOW() WHERE id = $1`, walletD.ID)
		require.Error(t, dbErr, "DB trigger must hold the zero-active line even without a default")
		assert.ErrorContains(t, dbErr, "at least one active wallet")

		var active int
		require.NoError(t, dbConnectionPool.GetContext(ctx, &active,
			`SELECT COUNT(*) FROM distribution_wallets WHERE status = 'ACTIVE'`))
		assert.GreaterOrEqual(t, active, 1, "zero-active is unreachable")
	})
}

// Test_DistributionWallet_promotion_reassigns_tenant_account proves the promotion AC against
// the production schema layout: the tenant row's distribution-account fields (the
// default-bound association read by legacy single-wallet resolution) move to the new default
// atomically, and a failed promotion rolls back both the pointer and the association.
func Test_DistributionWallet_promotion_reassigns_tenant_account(t *testing.T) {
	dbt := dbtest.OpenWithoutMigrations(t)
	defer dbt.Close()
	adminPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer adminPool.Close()

	ctx := context.Background()

	// Production-like layout: admin schema + one migrated tenant schema.
	_, err = adminPool.ExecContext(ctx, "CREATE SCHEMA admin")
	require.NoError(t, err)
	adminDSN, err := router.GetDSNForAdmin(dbt.DSN)
	require.NoError(t, err)
	_, err = db.Migrate(adminDSN, migrate.Up, 0, migrations.AdminMigrationRouter)
	require.NoError(t, err)

	adminSchemaPool, err := db.OpenDBConnectionPool(adminDSN)
	require.NoError(t, err)
	defer adminSchemaPool.Close()

	originalAddr := keypair.MustRandom().Address()
	tnt := tenant.CreateTenantFixture(t, ctx, adminSchemaPool, "promo-tenant", originalAddr)
	tenant.PrepareDBForTenant(t, dbt, tnt.Name)

	tenantDSN, err := router.GetDSNForTenant(dbt.DSN, tnt.Name)
	require.NoError(t, err)
	tenantPool, err := db.OpenDBConnectionPool(tenantDSN)
	require.NoError(t, err)
	defer tenantPool.Close()

	models, err := data.NewModels(tenantPool)
	require.NoError(t, err)
	walletKeyService, err := tssSvc.NewDistributionWalletKeyService(tenantPool, keypair.MustRandom().Seed())
	require.NoError(t, err)
	svc, err := NewDistributionWalletManagementService(models, engine.SubmitterEngine{}, walletKeyService, 5)
	require.NoError(t, err)

	ctx = sdpcontext.SetTenantInContext(ctx, tnt)

	// Default wallet mirroring the tenant's original account + a candidate + a spare (so the
	// old default can be archived later without tripping the zero-active rule).
	_, err = models.DistributionWallets.EnsureDefaultWallet(ctx, tenantPool, &originalAddr,
		"DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT", "ACTIVE")
	require.NoError(t, err)
	candidate, err := models.DistributionWallets.Insert(ctx, tenantPool, data.DistributionWalletInsert{
		Name:        "promo-candidate",
		AccountType: "DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT",
	})
	require.NoError(t, err)
	candidateAddr := keypair.MustRandom().Address()
	candidate, err = models.DistributionWallets.UpdateAddress(ctx, tenantPool, candidate.ID, candidateAddr)
	require.NoError(t, err)
	candidate, err = models.DistributionWallets.Activate(ctx, tenantPool, candidate.ID)
	require.NoError(t, err)

	readTenantAccount := func() string {
		var addr string
		require.NoError(t, adminPool.GetContext(ctx, &addr,
			`SELECT distribution_account_address FROM admin.tenants WHERE id = $1`, tnt.ID))
		return addr
	}

	t.Run("promotion reassigns the tenant's distribution-account fields", func(t *testing.T) {
		require.Equal(t, originalAddr, readTenantAccount())

		promoted, pErr := svc.PromoteToDefault(ctx, candidate.ID)
		require.NoError(t, pErr)
		assert.True(t, promoted.IsDefault)

		assert.Equal(t, candidateAddr, readTenantAccount(),
			"admin.tenants must point at the new default wallet's account")
	})

	t.Run("failed promotion rolls back pointer AND association", func(t *testing.T) {
		// The old default (now non-default, ACTIVE) gets archived; promoting an archived
		// wallet fails mid-transaction, after the demotion already ran.
		oldDefault, gErr := models.DistributionWallets.GetByName(ctx, tenantPool, data.DefaultDistributionWalletName)
		require.NoError(t, gErr)
		archived, aErr := models.DistributionWallets.Archive(ctx, tenantPool, oldDefault.ID)
		require.NoError(t, aErr)

		_, pErr := svc.PromoteToDefault(ctx, archived.ID)
		require.ErrorIs(t, pErr, ErrCannotPromoteWallet)

		// Default pointer unchanged…
		gotDefault, dErr := models.DistributionWallets.GetDefault(ctx, tenantPool)
		require.NoError(t, dErr)
		assert.Equal(t, candidate.ID, gotDefault.ID)
		// …and the association untouched.
		assert.Equal(t, candidateAddr, readTenantAccount())
	})
}
