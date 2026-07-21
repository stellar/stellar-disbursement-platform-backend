// Package observability contains optional, pluggable log-shipping
// infrastructure. It mirrors the "optional backend behind a config-gated
// type" style already used by internal/monitor for metrics (MonitorClient +
// GetClient(opts)): nothing in here is wired up unless the operator opts in
// via config, and the rest of the app never needs to know it exists.
package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	// defaultServiceName is the "service" stream label applied when
	// LokiHookConfig.ServiceName is left blank.
	defaultServiceName = "sdp-api"

	// defaultBatchInterval bounds how long a log line can sit buffered before
	// being flushed, even if defaultBatchSize hasn't been reached yet.
	defaultBatchInterval = 2 * time.Second

	// defaultBatchSize triggers an early flush once this many entries have
	// accumulated, instead of waiting for the next interval tick.
	defaultBatchSize = 100

	// defaultMaxQueueSize bounds the number of log lines that can be buffered
	// waiting to be shipped. This is the backpressure valve: once full, Fire()
	// drops new entries (logging a single warning) rather than blocking the
	// caller or growing without bound. Sized generously relative to
	// defaultBatchSize so a slow-but-not-dead Loki endpoint doesn't cause
	// drops under normal bursts.
	defaultMaxQueueSize = 10_000

	// defaultHTTPTimeout bounds each push request. This is also the effective
	// upper bound on how long Close() (graceful shutdown) or a Fatal/Panic
	// synchronous flush can block, since sends never retry.
	defaultHTTPTimeout = 5 * time.Second

	// defaultFatalFlushTimeout bounds the best-effort synchronous flush
	// triggered by a Fatal or Panic log entry (see Fire).
	defaultFatalFlushTimeout = 3 * time.Second
)

// LokiHookConfig configures a LokiHook.
type LokiHookConfig struct {
	// PushURL is the Loki-compatible push endpoint to POST batches to, e.g. a
	// Grafana Alloy `loki.source.api` receiver's
	// "http://alloy.railway.internal:8080/loki/api/v1/push". Required by
	// NewLokiHook's caller -- this package doesn't decide whether shipping is
	// enabled, it just ships once asked to.
	PushURL string

	// ServiceName is promoted as the "service" stream label on every line.
	// Defaults to "sdp-api" if empty.
	ServiceName string

	// BatchInterval is the maximum time a log line waits before being
	// flushed. Defaults to 2s if <= 0.
	BatchInterval time.Duration

	// BatchSize triggers an early flush once reached, ahead of the next
	// interval tick. Defaults to 100 if <= 0.
	BatchSize int

	// MaxQueueSize bounds the number of buffered-but-not-yet-shipped log
	// lines. Defaults to 10_000 if <= 0.
	MaxQueueSize int

	// HTTPClient overrides the client used to push batches. Defaults to a
	// client with a 5s timeout. Exposed mainly for tests.
	HTTPClient *http.Client
}

// logLine is a single entry queued for shipping.
type logLine struct {
	timestampNano string
	level         string
	line          string
}

// LokiHook is a logrus.Hook that ships every log entry the app emits to a
// Loki-compatible HTTP push endpoint, in addition to (never instead of) the
// app's normal stdout logging. It is designed so a Loki outage, a
// misconfigured endpoint, or plain network flakiness can *never* slow down or
// take down the app:
//
//   - Fire() never does network I/O. It renders the entry (reusing whatever
//     formatter/fields the app's logger is already configured with -- this
//     hook does not change or duplicate that logic) and hands it to a
//     background goroutine over a bounded, buffered channel.
//   - If that channel is full (the background goroutine can't keep up, or the
//     endpoint is down), new entries are dropped and a single warning is
//     logged the first time it happens. The app's own logging is completely
//     unaffected either way.
//   - The background goroutine batches entries by time (BatchInterval) or
//     size (BatchSize), whichever comes first, and POSTs them to PushURL in
//     Loki's native push format.
//
// Stream labels are deliberately minimal: only "service" and "level". See the
// comment on buildPayload for why higher-cardinality fields (wallet_id,
// payment_id, tenant_name, ...) are intentionally left out here.
type LokiHook struct {
	cfg    LokiHookConfig
	client *http.Client

	queue chan logLine
	done  chan struct{}
	wg    sync.WaitGroup

	dropWarnOnce sync.Once
	sendWarnOnce sync.Once
	closeOnce    sync.Once
}

// NewLokiHook constructs a LokiHook and starts its background batching
// goroutine. Callers are responsible for calling Close() during a graceful
// shutdown so any buffered-but-unsent lines get a final flush attempt.
func NewLokiHook(cfg LokiHookConfig) *LokiHook {
	if cfg.ServiceName == "" {
		cfg.ServiceName = defaultServiceName
	}
	if cfg.BatchInterval <= 0 {
		cfg.BatchInterval = defaultBatchInterval
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = defaultBatchSize
	}
	if cfg.MaxQueueSize <= 0 {
		cfg.MaxQueueSize = defaultMaxQueueSize
	}

	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: defaultHTTPTimeout}
	}

	h := &LokiHook{
		cfg:    cfg,
		client: client,
		queue:  make(chan logLine, cfg.MaxQueueSize),
		done:   make(chan struct{}),
	}

	h.wg.Add(1)
	go h.run()

	return h
}

// Levels implements logrus.Hook. All levels are shipped; logrus already
// filters by the logger's configured level before hooks ever fire, so this
// hook simply mirrors whatever gets logged.
func (h *LokiHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

// Fire implements logrus.Hook. It must never block or fail the caller's log
// call: rendering is cheap/local, and the only "sends" are non-blocking
// channel operations (with one deliberate exception, documented below).
func (h *LokiHook) Fire(entry *logrus.Entry) error {
	// entry.String() renders the entry through the app's *currently
	// configured* logrus formatter (the stellar-sdk log wrapper's
	// TextFormatter, key=value style) -- the exact same bytes that are about
	// to be written to stdout. We deliberately don't reformat/re-derive the
	// line ourselves so this hook can never drift from, duplicate, or change
	// what the existing logger emits.
	rendered, err := entry.String()
	if err != nil {
		// Never let a rendering failure affect the caller's logging call.
		return nil //nolint:nilerr
	}

	ll := logLine{
		timestampNano: strconv.FormatInt(entry.Time.UnixNano(), 10),
		level:         entry.Level.String(),
		line:          strings.TrimRight(rendered, "\n"),
	}

	select {
	case h.queue <- ll:
	default:
		h.dropWarnOnce.Do(func() {
			warnf("log shipping queue is full (max %d buffered); "+
				"dropping log entries destined for Loki. Normal application logging is unaffected -- "+
				"this only affects the Loki shipping side-channel. This warning is only printed once.",
				h.cfg.MaxQueueSize)
		})
	}

	// Fatal/Panic entries precede an imminent os.Exit (Fatal) or a panic
	// unwind (Panic); the background goroutine's next scheduled tick may
	// never come. Make one best-effort, bounded, synchronous flush attempt so
	// the line that explains *why* the process is going down has a chance of
	// reaching Loki too. This intentionally blocks the caller -- but only on
	// the Fatal/Panic path, which is already about to end/unwind this
	// goroutine, so it doesn't violate the "never block normal logging"
	// guarantee above.
	if entry.Level == logrus.FatalLevel || entry.Level == logrus.PanicLevel {
		ctx, cancel := context.WithTimeout(context.Background(), defaultFatalFlushTimeout)
		defer cancel()
		h.flushAvailable(ctx)
	}

	return nil
}

// Close stops the background goroutine and makes a final, bounded-by-
// defaultHTTPTimeout attempt to flush any buffered entries. It is safe to
// call multiple times. Intended to be called once, during graceful shutdown.
func (h *LokiHook) Close() error {
	h.closeOnce.Do(func() {
		close(h.done)
		h.wg.Wait()
	})
	return nil
}

// run is the background batching loop: one flush per BatchInterval tick, or
// as soon as BatchSize is reached, whichever comes first.
func (h *LokiHook) run() {
	defer h.wg.Done()

	ticker := time.NewTicker(h.cfg.BatchInterval)
	defer ticker.Stop()

	batch := make([]logLine, 0, h.cfg.BatchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), defaultHTTPTimeout)
		h.send(ctx, batch)
		cancel()
		batch = batch[:0]
	}

	for {
		select {
		case ll := <-h.queue:
			batch = append(batch, ll)
			if len(batch) >= h.cfg.BatchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-h.done:
			// Graceful shutdown: drain whatever is already buffered (without
			// waiting for more to arrive) and do one final flush.
			for {
				select {
				case ll := <-h.queue:
					batch = append(batch, ll)
				default:
					flush()
					return
				}
			}
		}
	}
}

// flushAvailable drains whatever is currently sitting in the queue (without
// touching h.done/h.wg -- the background goroutine keeps running
// independently) and ships it immediately. Used for the Fatal/Panic
// synchronous-flush path in Fire, so a recovered panic doesn't permanently
// disable shipping the way closing the hook would.
func (h *LokiHook) flushAvailable(ctx context.Context) {
	batch := make([]logLine, 0, h.cfg.BatchSize)
drainLoop:
	for len(batch) < cap(batch) {
		select {
		case ll := <-h.queue:
			batch = append(batch, ll)
		default:
			break drainLoop
		}
	}
	h.send(ctx, batch)
}

// send POSTs a batch to PushURL in Loki's native push format:
//
//	{"streams": [{"stream": {<labels>}, "values": [["<unix-nano>", "<line>"], ...]}]}
//
// Failures are swallowed (with a single one-time warning) -- by design, a
// Loki outage must never surface as an application error.
func (h *LokiHook) send(ctx context.Context, batch []logLine) {
	if len(batch) == 0 {
		return
	}

	payload := h.buildPayload(batch)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.cfg.PushURL, bytes.NewReader(payload))
	if err != nil {
		h.warnSendFailure(err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.client.Do(req)
	if err != nil {
		h.warnSendFailure(err)
		return
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body) // drain for connection reuse

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		h.warnSendFailure(fmt.Errorf("loki push endpoint returned status %d", resp.StatusCode))
	}
}

// lokiPushRequest/lokiStream mirror Loki's push API JSON shape.
type lokiPushRequest struct {
	Streams []lokiStream `json:"streams"`
}

type lokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][2]string       `json:"values"`
}

// buildPayload groups the batch into one Loki stream per distinct label set.
//
// Stream labels are kept to "service" and "level" ONLY, deliberately:
//
//   - This codebase already made -- and had to fix -- exactly this mistake
//     with unbounded Prometheus label cardinality (see the TSS metrics
//     cardinality fix earlier on this branch). Loki has the identical risk on
//     stream labels: every distinct label combination is its own index
//     series.
//   - High-cardinality fields the app already logs in a structured way
//     (payment_id, wallet_id, tx_id, ...) stay in the rendered log line body
//     -- fully searchable there -- and are NOT promoted to labels here.
//   - deploy/observability/alloy/config.alloy's `loki.process "sdp"` stage is
//     the one place downstream that already promotes `wallet_id` out of the
//     JSON body into a label, with an explicit comment justifying that it's
//     safe because the wallet count is small/bounded per tenant. That is a
//     deliberate, curated cardinality call made once, at the collector. This
//     hook intentionally does NOT duplicate that judgment call at the app
//     level too -- promoting a field as a label in two different places (with
//     two different people/PRs deciding "is this safe?") is how cardinality
//     mistakes creep back in. If wallet_id (or anything else) should also be
//     a label here, that's a conversation to have explicitly, not something
//     to default into.
//   - tenant_name is available on this codebase's per-request context loggers
//     but is NOT included here either: tenant count is expected to grow over
//     the life of this product (new NGOs onboard over time), so it does not
//     have the "small and bounded" property that makes wallet_id safe to
//     promote. It stays in the log line body only.
func (h *LokiHook) buildPayload(batch []logLine) []byte {
	streamsByLevel := make(map[string]*lokiStream, 8)
	order := make([]string, 0, 8)

	for _, ll := range batch {
		s, ok := streamsByLevel[ll.level]
		if !ok {
			s = &lokiStream{
				Stream: map[string]string{
					"service": h.cfg.ServiceName,
					"level":   ll.level,
				},
			}
			streamsByLevel[ll.level] = s
			order = append(order, ll.level)
		}
		s.Values = append(s.Values, [2]string{ll.timestampNano, ll.line})
	}

	req := lokiPushRequest{Streams: make([]lokiStream, 0, len(order))}
	for _, level := range order {
		req.Streams = append(req.Streams, *streamsByLevel[level])
	}

	// Marshaling a well-formed static struct; an error here would be a bug in
	// this file, not a runtime/network condition, so there's nothing useful
	// to do with it besides ship an (empty-streams) request.
	b, _ := json.Marshal(req)
	return b
}

func (h *LokiHook) warnSendFailure(err error) {
	h.sendWarnOnce.Do(func() {
		warnf("failed to push logs to Loki endpoint %s: %v. "+
			"Application logging to stdout is unaffected. This warning is only printed once; "+
			"further failures are silently retried on the next batch.", h.cfg.PushURL, err)
	})
}

// warnf prints a one-off operational warning about the log-shipping
// side-channel directly to stderr, deliberately bypassing the app's own
// logger (which this hook is itself attached to) to avoid any risk of
// feedback between "log a warning about logging" and this hook's Fire().
func warnf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[loki-hook] "+format+"\n", args...)
}
