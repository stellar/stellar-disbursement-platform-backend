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
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	tssSvc "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/services"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

// Test_OutboxEmission_RealFlows asserts the business flows write their outbox events in the
// same operation: pause → disbursement.paused; archive → wallet.archived; promote →
// wallet.promoted_to_default; grant/revoke → membership events.
func Test_OutboxEmission_RealFlows(t *testing.T) {
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
	walletSvc, err := NewDistributionWalletManagementService(models, engine.SubmitterEngine{}, walletKeyService, 5)
	require.NoError(t, err)
	disbursementSvc := &DisbursementManagementService{Models: models}

	countEvents := func(eventType, walletID string) int {
		var n int
		require.NoError(t, dbConnectionPool.GetContext(ctx, &n, `
			SELECT COUNT(*) FROM events WHERE event_type = $1 AND payload->>'source_wallet_id' = $2`,
			eventType, walletID))
		return n
	}

	defaultWallet := data.EnsureDefaultDistributionWalletFixture(t, ctx, dbConnectionPool)
	owner := &auth.User{ID: "emit-owner", Email: "o@emit.test", IsOwner: true}

	t.Run("pause emits disbursement.paused in the same flow", func(t *testing.T) {
		disbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
			Name: "emit-pause", Status: data.StartedDisbursementStatus, SourceWalletID: defaultWallet.ID,
		})
		require.NoError(t, disbursementSvc.PauseDisbursement(ctx, disbursement.ID, owner))
		assert.Equal(t, 1, countEvents(events.DisbursementPaused, defaultWallet.ID))
	})

	t.Run("membership grant + revoke emit with actor attribution", func(t *testing.T) {
		granted, gErr := walletSvc.GrantMembership(ctx, defaultWallet.ID, "emit-user", data.ApproverUserRole, "emit-owner")
		require.NoError(t, gErr)
		assert.Equal(t, 1, countEvents(events.WalletMembershipGranted, defaultWallet.ID))

		require.NoError(t, walletSvc.RevokeMembership(ctx, defaultWallet.ID, granted.ID, "emit-owner"))
		assert.Equal(t, 1, countEvents(events.WalletMembershipRevoked, defaultWallet.ID))

		var actor string
		require.NoError(t, dbConnectionPool.GetContext(ctx, &actor, `
			SELECT payload->'data'->>'actor_user_id' FROM events WHERE event_type = $1`, events.WalletMembershipRevoked))
		assert.Equal(t, "emit-owner", actor, "revoker attribution now lives in the event stream")
	})

	t.Run("promote + archive emit in their transactions", func(t *testing.T) {
		var walletBID string
		require.NoError(t, dbConnectionPool.GetContext(ctx, &walletBID, `
			INSERT INTO distribution_wallets (name, distribution_account_type)
			VALUES ('emit-wallet-b', 'DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT') RETURNING id`))

		_, pErr := walletSvc.PromoteToDefault(ctx, walletBID)
		require.NoError(t, pErr)
		assert.Equal(t, 1, countEvents(events.WalletPromotedToDefault, walletBID))

		_, aErr := walletSvc.ArchiveWallet(ctx, defaultWallet.ID)
		require.NoError(t, aErr)
		assert.Equal(t, 1, countEvents(events.WalletArchived, defaultWallet.ID))
	})

	t.Run("failed promotion writes no event (outbox rolls back with the transaction)", func(t *testing.T) {
		preCount := countEvents(events.WalletPromotedToDefault, defaultWallet.ID)
		_, pErr := walletSvc.PromoteToDefault(ctx, defaultWallet.ID) // archived → fails mid-tx
		require.Error(t, pErr)
		assert.Equal(t, preCount, countEvents(events.WalletPromotedToDefault, defaultWallet.ID),
			"the outbox guarantee: no event without a committed change")
	})
}
