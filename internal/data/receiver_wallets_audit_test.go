package data

import (
	"context"
	"testing"
	"time"

	"github.com/stellar/go/keypair"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
)

type receiverWalletsAuditEntry struct {
	ReceiverWallet
	Operation string    `db:"operation"`
	ChangedAt time.Time `db:"changed_at"`
}

func Test_ReceiverWalletsAudit(t *testing.T) {
	ctx := context.Background()
	models := SetupModels(t)
	wallet := CreateDefaultWalletFixture(t, ctx, models.DBConnectionPool)

	testCases := []struct {
		name      string
		operation string
		setup     func(*testing.T, context.Context, db.DBConnectionPool) *ReceiverWallet
	}{
		{
			name:      "🎉 insert operation is logged",
			operation: "INSERT",
			setup: func(t *testing.T, ctx context.Context, db db.DBConnectionPool) *ReceiverWallet {
				receiver := CreateReceiverFixture(t, ctx, models.DBConnectionPool, &Receiver{})
				return CreateReceiverWalletFixture(t, ctx, db, receiver.ID, wallet.ID, ReadyReceiversWalletStatus)
			},
		},
		{
			name:      "🎉 update operation is logged",
			operation: "UPDATE",
			setup: func(t *testing.T, ctx context.Context, db db.DBConnectionPool) *ReceiverWallet {
				receiver := CreateReceiverFixture(t, ctx, models.DBConnectionPool, &Receiver{})
				rw := CreateReceiverWalletFixture(t, ctx, db, receiver.ID, wallet.ID, ReadyReceiversWalletStatus)

				err := models.ReceiverWallet.Update(ctx, rw.ID, ReceiverWalletUpdate{
					StellarAddress: keypair.MustRandom().Address(),
				}, models.DBConnectionPool)
				require.NoError(t, err)

				return rw
			},
		},
		{
			name:      "🎉 delete operation is logged",
			operation: "DELETE",
			setup: func(t *testing.T, ctx context.Context, db db.DBConnectionPool) *ReceiverWallet {
				receiver := CreateReceiverFixture(t, ctx, models.DBConnectionPool, &Receiver{})
				rw := CreateReceiverWalletFixture(t, ctx, db, receiver.ID, wallet.ID, ReadyReceiversWalletStatus)
				_, err := db.ExecContext(ctx, `DELETE FROM receiver_wallets WHERE id = $1`, rw.ID)
				require.NoError(t, err)
				return rw
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rw := tc.setup(t, ctx, models.DBConnectionPool)
			latestEntry := getLastReceiverWalletsAuditEntry(t, ctx, models.DBConnectionPool, rw.ID)

			// ☑️ check audit metadata fields
			assert.Equal(t, tc.operation, latestEntry.Operation)
			assert.WithinDuration(t, time.Now(), latestEntry.ChangedAt, 5*time.Second)

			// ☑️ check receiver wallet fields
			assert.Equal(t, rw.Status, latestEntry.Status)
			assert.Equal(t, rw.Receiver.ID, latestEntry.Receiver.ID)
		})
	}
}

// getLastReceiverWalletsAuditEntry retrieves the last audit entry for a receiver wallet
func getLastReceiverWalletsAuditEntry(
	t *testing.T,
	ctx context.Context,
	db db.DBConnectionPool,
	receiverWalletID string,
) receiverWalletsAuditEntry {
	t.Helper()

	query := `
		SELECT operation, changed_at,
			` + ReceiverWalletColumnNames("rw", "") + `,
			` + WalletColumnNames("w", "wallet", false) + `
		FROM
			receiver_wallets_audit rw
		JOIN
			wallets w ON rw.wallet_id = w.id
		WHERE rw.id = $1  
		ORDER BY rw.changed_at DESC
	`

	var lastEntry receiverWalletsAuditEntry
	err := db.GetContext(ctx, &lastEntry, query, receiverWalletID)
	require.NoError(t, err)

	return lastEntry
}
