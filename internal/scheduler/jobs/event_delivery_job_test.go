package jobs

import (
	"context"
	"encoding/json"
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
			body := make([]byte, r.ContentLength)
			_, _ = r.Body.Read(body)
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
}
