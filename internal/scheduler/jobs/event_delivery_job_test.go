package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

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

	job := NewEventDeliveryJob(models)

	t.Run("no webhook configured → skipped, untouched", func(t *testing.T) {
		setWebhook(nil)
		eventID := writeEvent()
		require.NoError(t, job.Execute(ctx))
		delivered, attempts := eventState(eventID)
		assert.False(t, delivered)
		assert.Zero(t, attempts)
	})

	t.Run("successful delivery posts the intact envelope and marks delivered", func(t *testing.T) {
		var received atomic.Int32
		var lastBody []byte
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, readErr := io.ReadAll(r.Body)
			if readErr != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			lastBody = body
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
