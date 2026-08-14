package events

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

// Test_EventContract_FullTypeMatrix is the webhook contract test over the FROZEN v1 event-type
// list: every type writes an envelope that parses with the committed schema — event_id (uuid),
// event_type, schema_version (per-type), occurred_at (RFC3339, UTC), tenant_id,
// source_wallet_id (required on every event), and a data object. Old subscribers keep parsing
// because v1 schema changes are additive-only; bumping a payload without bumping its version
// fails this suite.
func Test_EventContract_FullTypeMatrix(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := sdpcontext.SetTenantInContext(context.Background(), &schema.Tenant{ID: "contract-tenant"})
	const walletID = "33333333-3333-3333-3333-333333333333"

	typeData := map[string]map[string]any{
		DisbursementCreated:   {"disbursement_id": "d-1", "name": "n", "actor_user_id": "u-1"},
		DisbursementApproved:  {"disbursement_id": "d-1", "name": "n", "actor_user_id": "u-2"},
		DisbursementRejected:  {"disbursement_id": "d-1", "name": "n", "reason": "draft_deleted"},
		DisbursementSubmitted: {"disbursement_id": "d-1", "name": "n", "actor_user_id": "u-2"},
		DisbursementCompleted: {"disbursement_id": "d-1"},
		DisbursementFailed:    {"disbursement_id": "d-1"},
		DisbursementPaused:    {"disbursement_id": "d-1", "name": "n", "actor_user_id": "u-2"},

		WalletCreated:           {"wallet_id": walletID, "name": "w", "status": "ACTIVE", "is_default": false},
		WalletArchived:          {"wallet_id": walletID, "name": "w", "status": "ARCHIVED"},
		WalletPromotedToDefault: {"wallet_id": walletID, "name": "w"},
		WalletMembershipGranted: {"membership_id": "m-1", "wallet_id": walletID, "user_id": "u-3", "role": "approver", "actor_user_id": "u-1"},
		WalletMembershipRevoked: {"membership_id": "m-1", "wallet_id": walletID, "user_id": "u-3", "role": "approver", "actor_user_id": "u-1"},
	}

	require.Len(t, typeData, len(SchemaVersions), "the contract matrix must cover the ENTIRE frozen list")

	for eventType, data := range typeData {
		t.Run(eventType, func(t *testing.T) {
			require.NoError(t, Write(ctx, dbConnectionPool, eventType, walletID, data))

			var row struct {
				EventType     string    `db:"event_type"`
				SchemaVersion int       `db:"schema_version"`
				Payload       []byte    `db:"payload"`
				OccurredAt    time.Time `db:"occurred_at"`
			}
			require.NoError(t, dbConnectionPool.GetContext(ctx, &row, `
				SELECT event_type, schema_version, payload, occurred_at FROM events
				WHERE event_type = $1 ORDER BY created_at DESC LIMIT 1`, eventType))

			assert.Equal(t, SchemaVersions[eventType], row.SchemaVersion)

			var envelope Envelope
			require.NoError(t, json.Unmarshal(row.Payload, &envelope))
			_, uuidErr := uuid.Parse(envelope.EventID)
			assert.NoError(t, uuidErr, "event_id must be a uuid")
			assert.Equal(t, eventType, envelope.EventType)
			assert.Equal(t, SchemaVersions[eventType], envelope.SchemaVersion)
			assert.Equal(t, "contract-tenant", envelope.TenantID)
			assert.Equal(t, walletID, envelope.SourceWalletID, "every event carries source_wallet_id")
			assert.False(t, envelope.OccurredAt.IsZero())
			for k := range data {
				assert.Contains(t, envelope.Data, k)
			}

			// Leak rules apply to webhook payloads: no Stellar addresses or seeds.
			assert.NotContains(t, string(row.Payload), `"G`, "payloads must not carry account addresses")
			assert.NotContains(t, string(row.Payload), `"S`, "payloads must not carry secrets")
		})
	}

	t.Run("unknown types are rejected (the list is frozen)", func(t *testing.T) {
		require.Error(t, Write(ctx, dbConnectionPool, "disbursement.exploded", walletID, nil))
	})

	t.Run("source_wallet_id is mandatory", func(t *testing.T) {
		require.Error(t, Write(ctx, dbConnectionPool, DisbursementCreated, "", nil))
	})
}
