package data

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
)

func TestInsertReceiverRegistrationAttempt(t *testing.T) {
	tests := []struct {
		name    string
		attempt ReceiverRegistrationAttempt
	}{
		{
			name: "phone OTP attempt record",
			attempt: ReceiverRegistrationAttempt{
				PhoneNumber:   "+1000000000000",
				Email:         "",
				ClientDomain:  "macragge.ultra",
				TransactionID: "TX-ULTRAMAR",
				WalletAddress: "ULTRAMARINESCHAPTER",
				WalletMemo:    "MKVII",
			},
		},
		{
			name: "email OTP attempt record",
			attempt: ReceiverRegistrationAttempt{
				PhoneNumber:   "",
				Email:         "baal@system.net",
				ClientDomain:  "baal.system",
				TransactionID: "TX-BLOODANGELS",
				WalletAddress: "BAALSAGA",
				WalletMemo:    "VETERAN",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			models, ctx := setupModels(t)

			now := time.Now().UTC()
			tc.attempt.AttemptTs = now

			err := models.ReceiverRegistrationAttempt.InsertReceiverRegistrationAttempt(ctx, tc.attempt)
			require.NoError(t, err)

			var phone, email, clientDomain, txID, address, memo string
			var ts time.Time
			row := models.DBConnectionPool.QueryRowxContext(ctx, `
                SELECT phone_number, email, client_domain, transaction_id, wallet_address, wallet_memo, attempt_ts
                FROM receiver_registration_attempts
                LIMIT 1
            `)
			err = row.Scan(&phone, &email, &clientDomain, &txID, &address, &memo, &ts)
			require.NoError(t, err)

			assert.Equal(t, tc.attempt.PhoneNumber, phone)
			assert.Equal(t, tc.attempt.Email, email)
			assert.Equal(t, tc.attempt.ClientDomain, clientDomain)
			assert.Equal(t, tc.attempt.TransactionID, txID)
			assert.Equal(t, tc.attempt.WalletAddress, address)
			assert.Equal(t, tc.attempt.WalletMemo, memo)
			assert.WithinDuration(t, now, ts.UTC(), time.Second)

			// clean up for next subtest
			_, _ = models.DBConnectionPool.ExecContext(ctx, `DELETE FROM receiver_registration_attempts`)
		})
	}
}

func setupModels(t *testing.T) (*Models, context.Context) {
	dbt := dbtest.Open(t)
	t.Cleanup(func() { dbt.Close() })

	pool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })

	models, err := NewModels(pool)
	require.NoError(t, err)

	return models, context.Background()
}
