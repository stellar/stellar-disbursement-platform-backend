package data

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

type receiverAuditEntry struct {
	Receiver
	Operation string    `db:"operation"`
	ChangedAt time.Time `db:"changed_at"`
}

func Test_ReceiversAudit(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	receiverModel := ReceiverModel{}

	testCases := []struct {
		name      string
		operation string
		setup     func(*testing.T, context.Context, db.DBConnectionPool) *Receiver
	}{
		{
			name:      "🎉 insert operation is logged",
			operation: "INSERT",
			setup: func(t *testing.T, ctx context.Context, db db.DBConnectionPool) *Receiver {
				return CreateReceiverFixture(t, ctx, db, &Receiver{
					Email:       "insert@stellar.org",
					PhoneNumber: "+1111111111",
					ExternalID:  "audit-test-insert",
				})
			},
		},
		{
			name:      "🎉 update operation is logged",
			operation: "UPDATE",
			setup: func(t *testing.T, ctx context.Context, db db.DBConnectionPool) *Receiver {
				receiver := CreateReceiverFixture(t, ctx, db, &Receiver{
					Email:       "update@stellar.org",
					PhoneNumber: "+2222222222",
					ExternalID:  "audit-test-update",
				})

				err := receiverModel.Update(ctx, db, receiver.ID, ReceiverUpdate{
					Email: utils.StringPtr("updated@stellar.org"),
				})
				require.NoError(t, err)

				return receiver
			},
		},
		{
			name:      "🎉 delete operation is logged",
			operation: "DELETE",
			setup: func(t *testing.T, ctx context.Context, db db.DBConnectionPool) *Receiver {
				receiver := CreateReceiverFixture(t, ctx, db, &Receiver{
					Email:       "delete@stellar.org",
					PhoneNumber: "+3333333333",
					ExternalID:  "audit-test-delete",
				})

				_, err := db.ExecContext(ctx, "DELETE FROM receivers WHERE id = $1", receiver.ID)
				require.NoError(t, err)

				return receiver
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			receiver := tc.setup(t, ctx, dbConnectionPool)
			latestEntry := getLastReceiversAuditEntries(t, ctx, dbConnectionPool, receiver.ID)

			// ☑️ check audit metadata fields
			assert.Equal(t, tc.operation, latestEntry.Operation)
			assert.WithinDuration(t, time.Now(), latestEntry.ChangedAt, 5*time.Second)

			// ☑️ check receiver fields
			assert.Equal(t, receiver.ID, latestEntry.ID)
			assert.Equal(t, receiver.Email, latestEntry.Email)
			assert.Equal(t, receiver.PhoneNumber, latestEntry.PhoneNumber)
			assert.Equal(t, receiver.ExternalID, latestEntry.ExternalID)
		})
	}
}

// getLastReceiversAuditEntries retrieves audit entries for a receiver ordered by most recent first
func getLastReceiversAuditEntries(t *testing.T, ctx context.Context, db db.DBConnectionPool, receiverID string) receiverAuditEntry {
	const query = `
		SELECT *, operation, changed_at 
		FROM receivers_audit 
		WHERE id = $1 
		ORDER BY changed_at DESC
		LIMIT 1
	`

	var lastEntry receiverAuditEntry
	err := db.GetContext(ctx, &lastEntry, query, receiverID)
	require.NoError(t, err)

	return lastEntry
}
