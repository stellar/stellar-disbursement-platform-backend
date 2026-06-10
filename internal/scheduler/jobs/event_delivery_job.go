package jobs

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/stellar/go-stellar-sdk/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

const (
	eventDeliveryJobName     = "event_delivery_job"
	eventDeliveryJobInterval = time.Second * 30
	// eventDeliveryBatchLimit bounds one run's work; older events go first.
	eventDeliveryBatchLimit = 100
	// eventDeliveryMaxAttempts stops retrying poisoned events; they stay undelivered and
	// visible in the events table for operations.
	eventDeliveryMaxAttempts = 20
	eventDeliveryHTTPTimeout = 10 * time.Second
	// eventDeliveryRetentionDays bounds outbox growth: DELIVERED events older than this are
	// pruned each run. Undelivered (including poisoned) events are NEVER pruned — they stay
	// visible for operations until handled.
	eventDeliveryRetentionDays = 30
)

// eventDeliveryJob delivers undelivered outbox events to the tenant's configured webhook URL
// (W3, accepted spec flags E1/E3): at-least-once, oldest first; consumers deduplicate on
// event_id. Tenants without a webhook URL are skipped.
type eventDeliveryJob struct {
	models     *data.Models
	httpClient *http.Client
}

func NewEventDeliveryJob(models *data.Models) Job {
	return &eventDeliveryJob{
		models:     models,
		httpClient: &http.Client{Timeout: eventDeliveryHTTPTimeout},
	}
}

func (j eventDeliveryJob) Execute(ctx context.Context) error {
	// Retention: prune delivered events past the window (runs regardless of webhook config so
	// the outbox table stays bounded even if delivery is later disabled).
	if _, pruneErr := j.models.DBConnectionPool.ExecContext(ctx, `
		DELETE FROM events
		WHERE delivered_at IS NOT NULL AND delivered_at < NOW() - make_interval(days => $1)`,
		eventDeliveryRetentionDays); pruneErr != nil {
		return fmt.Errorf("pruning delivered events: %w", pruneErr)
	}

	organization, err := j.models.Organizations.Get(ctx)
	if err != nil {
		return fmt.Errorf("getting organization for event delivery: %w", err)
	}
	if organization.WebhookURL == nil || *organization.WebhookURL == "" {
		return nil // webhook delivery not configured for this tenant
	}
	webhookURL := *organization.WebhookURL

	type eventRow struct {
		ID      string `db:"id"`
		Payload []byte `db:"payload"`
	}
	var pending []eventRow
	err = j.models.DBConnectionPool.SelectContext(ctx, &pending, `
		SELECT id, payload FROM events
		WHERE delivered_at IS NULL AND delivery_attempts < $1
		ORDER BY occurred_at ASC
		LIMIT $2`, eventDeliveryMaxAttempts, eventDeliveryBatchLimit)
	if err != nil {
		return fmt.Errorf("listing undelivered events: %w", err)
	}

	for _, event := range pending {
		if deliverErr := j.deliver(ctx, webhookURL, event.Payload); deliverErr != nil {
			log.Ctx(ctx).Warnf("delivering event %s: %v", event.ID, deliverErr)
			if _, uErr := j.models.DBConnectionPool.ExecContext(ctx, `
				UPDATE events SET delivery_attempts = delivery_attempts + 1 WHERE id = $1`, event.ID); uErr != nil {
				return fmt.Errorf("recording delivery attempt for event %s: %w", event.ID, uErr)
			}
			continue
		}
		if _, uErr := j.models.DBConnectionPool.ExecContext(ctx, `
			UPDATE events SET delivered_at = NOW(), delivery_attempts = delivery_attempts + 1 WHERE id = $1`, event.ID); uErr != nil {
			return fmt.Errorf("marking event %s delivered: %w", event.ID, uErr)
		}
	}

	return nil
}

func (j eventDeliveryJob) deliver(ctx context.Context, webhookURL string, payload []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("building webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := j.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("posting webhook: %w", err)
	}
	defer utils.DeferredClose(ctx, resp.Body, "closing webhook response body")

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook responded %d", resp.StatusCode)
	}
	return nil
}

func (j eventDeliveryJob) GetInterval() time.Duration {
	return eventDeliveryJobInterval
}

func (j eventDeliveryJob) GetName() string {
	return eventDeliveryJobName
}

func (j eventDeliveryJob) IsJobMultiTenant() bool {
	return true
}

var _ Job = (*eventDeliveryJob)(nil)
