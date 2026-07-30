package jobs

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
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

// eventDeliveryJob delivers undelivered outbox events to the tenant's configured webhook URL:
// at-least-once, oldest first; consumers deduplicate on event_id. Tenants without a webhook URL
// are skipped.
type eventDeliveryJob struct {
	models     *data.Models
	httpClient *http.Client
}

func NewEventDeliveryJob(models *data.Models) Job {
	return &eventDeliveryJob{
		models:     models,
		httpClient: newWebhookHTTPClient(eventDeliveryHTTPTimeout),
	}
}

// newWebhookHTTPClient builds a client hardened against SSRF: it refuses to dial private,
// loopback, link-local, or otherwise non-public addresses, and never follows redirects (a
// webhook target could otherwise redirect delivery to an internal address after passing the
// initial check). The address validation happens in DialContext, at actual connection time,
// rather than as a one-off pre-check against the parsed URL — a pre-check alone is vulnerable to
// DNS rebinding, where the hostname resolves to a public IP during validation and to a private
// one by the time the client actually dials.
func newWebhookHTTPClient(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	safeDialContext := func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, fmt.Errorf("splitting host/port %q: %w", addr, err)
		}
		ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("resolving host %q: %w", host, err)
		}
		var target net.IP
		for _, ip := range ips {
			if !isPublicWebhookAddress(ip.IP) {
				return nil, fmt.Errorf("refusing to dial non-public address %s for host %q", ip.IP, host)
			}
			if target == nil {
				target = ip.IP
			}
		}
		// Dial the specific address just validated, not the original host, so nothing can
		// re-resolve to a different (unvalidated) address between the check above and the dial.
		return dialer.DialContext(ctx, network, net.JoinHostPort(target.String(), port))
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: &http.Transport{DialContext: safeDialContext},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return fmt.Errorf("webhook deliveries do not follow redirects (target: %s)", req.URL)
		},
	}
}

// isPublicWebhookAddress reports whether ip is safe to deliver a webhook to: reachable only if
// it isn't a private, loopback, link-local, unspecified, or multicast address.
func isPublicWebhookAddress(ip net.IP) bool {
	return !(ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast())
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
		if deliverErr := j.deliver(ctx, webhookURL, organization.WebhookSecret, event.Payload); deliverErr != nil {
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

func (j eventDeliveryJob) deliver(ctx context.Context, webhookURL, webhookSecret string, payload []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("building webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Signs the payload so consumers can verify it actually came from SDP: the webhook URL is
	// the only thing an attacker would need to send fake events otherwise, and URLs leak into
	// logs and proxies far more easily than a dedicated secret does.
	mac := hmac.New(sha256.New, []byte(webhookSecret))
	mac.Write(payload)
	req.Header.Set("X-SDP-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))

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
