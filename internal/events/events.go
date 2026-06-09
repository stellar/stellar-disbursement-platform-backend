// Package events implements the transactional outbox for the multi-wallet workstream's
// webhook/internal events (spec: specs/W3-event-types.md, accepted). Event rows are written in
// the same DB transaction as the state change they describe; a scheduler job delivers them.
package events

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
)

// The frozen v1 event-type list. Adding a type is a spec change order.
const (
	DisbursementCreated   = "disbursement.created"
	DisbursementApproved  = "disbursement.approved"
	DisbursementRejected  = "disbursement.rejected"
	DisbursementSubmitted = "disbursement.submitted"
	DisbursementCompleted = "disbursement.completed"
	DisbursementFailed    = "disbursement.failed"
	DisbursementPaused    = "disbursement.paused"

	WalletCreated           = "wallet.created"
	WalletArchived          = "wallet.archived"
	WalletPromotedToDefault = "wallet.promoted_to_default"
	WalletMembershipGranted = "wallet.membership_granted"
	WalletMembershipRevoked = "wallet.membership_revoked"
)

// SchemaVersions maps each frozen type to its current payload schema version. Any payload
// change bumps the type's version; v1 changes are additive-only so existing consumers keep
// parsing (the contract test suite enforces this).
var SchemaVersions = map[string]int{
	DisbursementCreated:   1,
	DisbursementApproved:  1,
	DisbursementRejected:  1,
	DisbursementSubmitted: 1,
	DisbursementCompleted: 1,
	DisbursementFailed:    1,
	DisbursementPaused:    1,

	WalletCreated:           1,
	WalletArchived:          1,
	WalletPromotedToDefault: 1,
	WalletMembershipGranted: 1,
	WalletMembershipRevoked: 1,
}

// Envelope is the versioned payload wrapper. Every disbursement.* event carries
// source_wallet_id (the W3 commitment); wallet.* events carry the wallet in both the
// source_wallet_id and data.wallet_id fields. No addresses or key material ever appear in
// payloads (leak rules apply to webhooks too).
type Envelope struct {
	EventID        string         `json:"event_id"`
	EventType      string         `json:"event_type"`
	SchemaVersion  int            `json:"schema_version"`
	OccurredAt     time.Time      `json:"occurred_at"`
	TenantID       string         `json:"tenant_id,omitempty"`
	SourceWalletID string         `json:"source_wallet_id"`
	Data           map[string]any `json:"data"`
}

// Write inserts one outbox event. Call it with the SAME SQLExecuter (transaction) as the state
// change it describes — the outbox guarantee is "event exists iff the change committed". For
// the few non-transactional sites (e.g. wallet provisioning), pass the pool and treat errors
// per-site.
func Write(ctx context.Context, sqlExec db.SQLExecuter, eventType, sourceWalletID string, data map[string]any) error {
	version, ok := SchemaVersions[eventType]
	if !ok {
		return fmt.Errorf("event type %q is not in the frozen v1 list", eventType)
	}
	if sourceWalletID == "" {
		return fmt.Errorf("source wallet ID is required on every %q event", eventType)
	}
	if data == nil {
		data = map[string]any{}
	}

	envelope := Envelope{
		EventID:        uuid.NewString(),
		EventType:      eventType,
		SchemaVersion:  version,
		OccurredAt:     time.Now().UTC(),
		SourceWalletID: sourceWalletID,
		Data:           data,
	}
	if tnt, err := sdpcontext.GetTenantFromContext(ctx); err == nil && tnt != nil {
		envelope.TenantID = tnt.ID
	}

	payload, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("marshaling %q event payload: %w", eventType, err)
	}

	_, err = sqlExec.ExecContext(ctx, `
		INSERT INTO events (id, event_type, schema_version, payload, occurred_at)
		VALUES ($1, $2, $3, $4, $5)`,
		envelope.EventID, envelope.EventType, envelope.SchemaVersion, payload, envelope.OccurredAt)
	if err != nil {
		return fmt.Errorf("writing %q event to the outbox: %w", eventType, err)
	}

	return nil
}
