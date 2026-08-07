package jobs

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
)

func Test_EventDeliveryJob(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	wallet := data.EnsureDefaultDistributionWalletFixture(t, ctx, dbConnectionPool)
	setWebhook := func(url *string) {
		_, uErr := dbConnectionPool.ExecContext(ctx, `UPDATE organizations SET webhook_url = $1`, url)
		require.NoError(t, uErr)
	}
	eventState := func(id string) (delivered bool, attempts int) {
		var row struct {
			Delivered bool `db:"delivered"`
			Attempts  int  `db:"delivery_attempts"`
		}
		require.NoError(t, dbConnectionPool.GetContext(ctx, &row, `
			SELECT delivered_at IS NOT NULL AS delivered, delivery_attempts FROM events WHERE id = $1`, id))
		return row.Delivered, row.Attempts
	}
	writeEvent := func() string {
		require.NoError(t, events.Write(ctx, dbConnectionPool, events.WalletCreated, wallet.ID, map[string]any{"wallet_id": wallet.ID}))
		var id string
		require.NoError(t, dbConnectionPool.GetContext(ctx, &id,
			`SELECT id FROM events WHERE delivered_at IS NULL ORDER BY created_at DESC LIMIT 1`))
		return id
	}

	// Constructed directly (bypassing NewEventDeliveryJob's SSRF-hardened client) so these
	// subtests can deliver to a local httptest.Server — real production wiring always goes
	// through NewEventDeliveryJob, which refuses to dial loopback/private addresses.
	job := &eventDeliveryJob{models: models, httpClient: &http.Client{Timeout: eventDeliveryHTTPTimeout}}

	var webhookSecret string
	require.NoError(t, dbConnectionPool.GetContext(ctx, &webhookSecret, `SELECT webhook_secret FROM organizations LIMIT 1`))

	t.Run("no webhook configured → skipped, untouched", func(t *testing.T) {
		setWebhook(nil)
		eventID := writeEvent()
		require.NoError(t, job.Execute(ctx))
		delivered, attempts := eventState(eventID)
		assert.False(t, delivered)
		assert.Zero(t, attempts)
	})

	t.Run("successful delivery posts the intact envelope, signed, and marks delivered", func(t *testing.T) {
		var received atomic.Int32
		var lastBody []byte
		var lastSignature, lastTimestamp string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, readErr := io.ReadAll(r.Body)
			if readErr != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			lastBody = body
			lastSignature = r.Header.Get("X-SDP-Signature")
			lastTimestamp = r.Header.Get("X-SDP-Timestamp")
			received.Add(1)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()
		setWebhook(&server.URL)

		eventID := writeEvent()
		require.NoError(t, job.Execute(ctx))

		assert.GreaterOrEqual(t, received.Load(), int32(1))
		delivered, attempts := eventState(eventID)
		assert.True(t, delivered)
		assert.GreaterOrEqual(t, attempts, 1)

		var envelope events.Envelope
		require.NoError(t, json.Unmarshal(lastBody, &envelope))
		assert.Equal(t, events.WalletCreated, envelope.EventType)
		assert.Equal(t, wallet.ID, envelope.SourceWalletID)

		sentAt, tsErr := strconv.ParseInt(lastTimestamp, 10, 64)
		require.NoError(t, tsErr, "consumers need a parseable timestamp to bound replay")
		assert.WithinDuration(t, time.Now(), time.Unix(sentAt, 0), time.Minute)

		mac := hmac.New(sha256.New, []byte(webhookSecret))
		mac.Write([]byte(lastTimestamp + "."))
		mac.Write(lastBody)
		assert.Equal(t, "sha256="+hex.EncodeToString(mac.Sum(nil)), lastSignature,
			"consumers must be able to verify the delivery actually came from SDP")

		// The timestamp must be *inside* the MAC, not merely alongside it — otherwise a replayer
		// could hand the consumer a fresh timestamp with the captured body and signature.
		bodyOnly := hmac.New(sha256.New, []byte(webhookSecret))
		bodyOnly.Write(lastBody)
		assert.NotEqual(t, "sha256="+hex.EncodeToString(bodyOnly.Sum(nil)), lastSignature,
			"a signature over the body alone would leave the timestamp forgeable")

		// Idempotence: a second run has nothing to deliver.
		before := received.Load()
		require.NoError(t, job.Execute(ctx))
		assert.Equal(t, before, received.Load(), "delivered events must not be re-sent")
	})

	t.Run("failed delivery increments attempts and stays undelivered (at-least-once)", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		}))
		defer server.Close()
		setWebhook(&server.URL)

		eventID := writeEvent()
		require.NoError(t, job.Execute(ctx))
		delivered, attempts := eventState(eventID)
		assert.False(t, delivered)
		assert.Equal(t, 1, attempts)

		require.NoError(t, job.Execute(ctx))
		_, attempts = eventState(eventID)
		assert.Equal(t, 2, attempts, "each run retries undelivered events")
	})

	t.Run("poisoned events (attempts at cap) are never selected, even with a healthy webhook", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()
		setWebhook(&server.URL)

		eventID := writeEvent()
		// Drive the event to the poison cap; the delivery SELECT filters on delivery_attempts < max.
		_, uErr := dbConnectionPool.ExecContext(ctx,
			`UPDATE events SET delivery_attempts = $2 WHERE id = $1`, eventID, eventDeliveryMaxAttempts)
		require.NoError(t, uErr)

		require.NoError(t, job.Execute(ctx))

		delivered, attempts := eventState(eventID)
		assert.False(t, delivered, "a poisoned event must not be delivered even with a healthy webhook")
		assert.Equal(t, eventDeliveryMaxAttempts, attempts, "a poisoned event must not be retried or incremented past the cap")
	})
}

// Test_EventDeliveryJob_skipsRowsClaimedByAnotherInstance proves the delivery batch is reserved:
// an event another instance is already holding is stepped over rather than delivered a second
// time. Standing in for the concurrent replica is an open transaction holding the row lock.
func Test_EventDeliveryJob_skipsRowsClaimedByAnotherInstance(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	wallet := data.EnsureDefaultDistributionWalletFixture(t, ctx, dbConnectionPool)

	var received atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		received.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	_, err = dbConnectionPool.ExecContext(ctx, `UPDATE organizations SET webhook_url = $1`, server.URL)
	require.NoError(t, err)

	require.NoError(t, events.Write(ctx, dbConnectionPool, events.WalletCreated, wallet.ID, map[string]any{"wallet_id": wallet.ID}))
	var eventID string
	require.NoError(t, dbConnectionPool.GetContext(ctx, &eventID,
		`SELECT id FROM events ORDER BY created_at DESC LIMIT 1`))

	claimer, err := dbConnectionPool.BeginTxx(ctx, nil)
	require.NoError(t, err)
	var lockedID string
	require.NoError(t, claimer.GetContext(ctx, &lockedID, `SELECT id FROM events WHERE id = $1 FOR UPDATE`, eventID))

	job := &eventDeliveryJob{models: models, httpClient: &http.Client{Timeout: eventDeliveryHTTPTimeout}}
	require.NoError(t, job.Execute(ctx))

	assert.Zero(t, received.Load(), "an event already claimed elsewhere must not be delivered again")
	require.NoError(t, claimer.Rollback())

	var attempts int
	require.NoError(t, dbConnectionPool.GetContext(ctx, &attempts, `SELECT delivery_attempts FROM events WHERE id = $1`, eventID))
	assert.Zero(t, attempts, "a skipped event must not burn an attempt against the poison cap")

	// Once the other instance lets go, the event is claimed and delivered normally.
	require.NoError(t, job.Execute(ctx))
	assert.Equal(t, int32(1), received.Load())
}

// Test_EventDeliveryJob_SSRFGuard proves the REAL production constructor (NewEventDeliveryJob,
// not the plain-client bypass the other tests use) actually refuses to dial a loopback address —
// the concrete SSRF vector a malicious or compromised webhook_url could otherwise exploit to
// reach internal services.
func Test_EventDeliveryJob_SSRFGuard(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	wallet := data.EnsureDefaultDistributionWalletFixture(t, ctx, dbConnectionPool)

	var received atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		received.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	_, err = dbConnectionPool.ExecContext(ctx, `UPDATE organizations SET webhook_url = $1`, server.URL)
	require.NoError(t, err)

	require.NoError(t, events.Write(ctx, dbConnectionPool, events.WalletCreated, wallet.ID, map[string]any{"wallet_id": wallet.ID}))

	job := NewEventDeliveryJob(models)
	require.NoError(t, job.Execute(ctx))

	assert.Zero(t, received.Load(), "the SSRF guard must refuse to dial a loopback address")

	var attempts int
	require.NoError(t, dbConnectionPool.GetContext(ctx, &attempts, `SELECT delivery_attempts FROM events ORDER BY created_at DESC LIMIT 1`))
	assert.Equal(t, 1, attempts, "the blocked attempt must still count toward the poison cap")
}

// Test_EventDeliveryJob_retention proves the outbox stays bounded: delivered events past the
// retention window are pruned; undelivered events — poisoned included — are never pruned.
func Test_EventDeliveryJob_retention(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	wallet := data.EnsureDefaultDistributionWalletFixture(t, ctx, dbConnectionPool)

	insertEvent := func(id string, deliveredAgoDays int, attempts int) {
		var deliveredAt interface{}
		if deliveredAgoDays >= 0 {
			deliveredAt = fmt.Sprintf("%d days", deliveredAgoDays)
		}
		payload := fmt.Sprintf(`{"source_wallet_id": %q}`, wallet.ID)
		_, insErr := dbConnectionPool.ExecContext(ctx, `
			INSERT INTO events (id, event_type, schema_version, occurred_at, payload, delivery_attempts, delivered_at)
			VALUES ($1, 'disbursement.created', 1, NOW() - INTERVAL '90 days', $2, $3,
			        CASE WHEN $4::text IS NULL THEN NULL ELSE NOW() - $4::interval END)`,
			id, payload, attempts, deliveredAt)
		require.NoError(t, insErr)
	}

	insertEvent("11111111-1111-4111-8111-111111111111", 45, 1)  // delivered, stale → pruned
	insertEvent("22222222-2222-4222-8222-222222222222", 2, 1)   // delivered, recent → kept
	insertEvent("33333333-3333-4333-8333-333333333333", -1, 20) // poisoned, undelivered → kept forever

	job := NewEventDeliveryJob(models)
	require.NoError(t, job.Execute(ctx))

	var remaining []string
	require.NoError(t, dbConnectionPool.SelectContext(ctx, &remaining, `SELECT id FROM events ORDER BY id`))
	assert.Equal(t, []string{
		"22222222-2222-4222-8222-222222222222",
		"33333333-3333-4333-8333-333333333333",
	}, remaining)
}
