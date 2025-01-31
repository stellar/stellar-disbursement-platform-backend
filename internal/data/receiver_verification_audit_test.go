package data

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

type receiverVerificationAuditEntry struct {
	ReceiverVerification
	Operation string    `db:"operation"`
	ChangedAt time.Time `db:"changed_at"`
}

func Test_ReceiverVerificationsAudit(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	model := ReceiverVerificationModel{}

	testCases := []struct {
		name      string
		operation string
		setup     func(*testing.T, context.Context, db.DBConnectionPool) *ReceiverVerification
	}{
		{
			name:      "🎉 insert operation is logged",
			operation: "INSERT",
			setup: func(t *testing.T, ctx context.Context, db db.DBConnectionPool) *ReceiverVerification {
				receiver := CreateReceiverFixture(t, ctx, db, &Receiver{})
				return CreateReceiverVerificationFixture(t, ctx, db, ReceiverVerificationInsert{
					ReceiverID:        receiver.ID,
					VerificationField: VerificationTypeDateOfBirth,
					VerificationValue: "1990-01-01",
				})
			},
		},
		{
			name:      "🎉 update operation is logged",
			operation: "UPDATE",
			setup: func(t *testing.T, ctx context.Context, db db.DBConnectionPool) *ReceiverVerification {
				receiver := CreateReceiverFixture(t, ctx, db, &Receiver{})
				receiverVerification := CreateReceiverVerificationFixture(t, ctx, db, ReceiverVerificationInsert{
					ReceiverID:        receiver.ID,
					VerificationField: VerificationTypeYearMonth,
					VerificationValue: "1990-01-01",
				})

				err := model.UpdateReceiverVerification(ctx, ReceiverVerificationUpdate{
					ReceiverID:          receiver.ID,
					VerificationField:   VerificationTypeYearMonth,
					Attempts:            utils.IntPtr(3),
					VerificationChannel: message.MessageChannelSMS,
					ConfirmedByType:     ConfirmedByTypeUser,
					ConfirmedByID:       "user-123",
				}, db)
				require.NoError(t, err)

				return receiverVerification
			},
		},
		{
			name:      "🎉 delete operation is logged",
			operation: "DELETE",
			setup: func(t *testing.T, ctx context.Context, db db.DBConnectionPool) *ReceiverVerification {
				receiver := CreateReceiverFixture(t, ctx, db, &Receiver{})
				verification := CreateReceiverVerificationFixture(t, ctx, db, ReceiverVerificationInsert{
					ReceiverID:        receiver.ID,
					VerificationField: VerificationTypeDateOfBirth,
					VerificationValue: "1990-01-01",
				})

				_, err := db.ExecContext(ctx,
					"DELETE FROM receiver_verifications WHERE receiver_id = $1 AND verification_field = $2",
					verification.ReceiverID,
					verification.VerificationField,
				)
				require.NoError(t, err)

				return verification
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			verification := tc.setup(t, ctx, dbConnectionPool)
			latestEntry := getLastReceiverVerificationsAuditEntry(t, ctx, dbConnectionPool,
				verification.ReceiverID,
				verification.VerificationField,
			)

			// ☑️ check audit metadata fields
			assert.Equal(t, tc.operation, latestEntry.Operation)
			assert.WithinDuration(t, time.Now(), latestEntry.ChangedAt, 5*time.Second)

			// ☑️ check receiver verification fields
			assert.Equal(t, verification.ReceiverID, latestEntry.ReceiverID)
			assert.Equal(t, verification.VerificationField, latestEntry.VerificationField)
			assert.Equal(t, verification.Attempts, latestEntry.Attempts)
			assert.Equal(t, verification.ConfirmedAt, latestEntry.ConfirmedAt)
			assert.Equal(t, verification.FailedAt, latestEntry.FailedAt)
			assert.Equal(t, verification.VerificationChannel, latestEntry.VerificationChannel)
			assert.Equal(t, verification.ConfirmedByType, latestEntry.ConfirmedByType)
			assert.Equal(t, verification.ConfirmedByID, latestEntry.ConfirmedByID)
		})
	}
}

// getLastReceiverVerificationsAuditEntry retrieves the last audit entry for a verification
func getLastReceiverVerificationsAuditEntry(
	t *testing.T,
	ctx context.Context,
	db db.DBConnectionPool,
	receiverID string,
	verificationField VerificationType,
) receiverVerificationAuditEntry {
	const query = `
		SELECT *, operation, changed_at 
		FROM receiver_verifications_audit 
		WHERE receiver_id = $1 AND verification_field = $2 
		ORDER BY changed_at DESC
		LIMIT 1
	`

	var lastEntry receiverVerificationAuditEntry
	err := db.GetContext(ctx, &lastEntry, query, receiverID, verificationField)
	require.NoError(t, err)

	return lastEntry
}
