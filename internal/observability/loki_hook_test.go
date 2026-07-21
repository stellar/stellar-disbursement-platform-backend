package observability

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test_LokiHook_ShipsLogsToPushEndpoint spins up a fake Loki receiver,
// attaches a LokiHook to a logrus logger, logs a couple of structured
// entries, and asserts that the receiver got a single POST containing the
// expected Loki push JSON shape:
//
//	{"streams": [{"stream": {<labels>}, "values": [["<unix-nano>", "<line>"], ...]}]}
//
// with the log line content preserved verbatim (i.e. this hook ships
// whatever the app's existing formatter emits, it doesn't reformat it) and
// with stream labels limited to service+level (no payment_id/wallet_id).
func Test_LokiHook_ShipsLogsToPushEndpoint(t *testing.T) {
	var mu sync.Mutex
	var gotBody []byte
	var gotContentType string
	var requestCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		mu.Lock()
		gotBody = body
		gotContentType = r.Header.Get("Content-Type")
		mu.Unlock()

		atomic.AddInt32(&requestCount, 1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	hook := NewLokiHook(LokiHookConfig{
		PushURL:     server.URL,
		ServiceName: "sdp-api-test",
		// Long enough that the ticker never fires during the test; Close()
		// below drives the flush deterministically instead.
		BatchInterval: time.Hour,
		BatchSize:     1000,
	})

	logger := logrus.New()
	logger.SetOutput(io.Discard)
	logger.AddHook(hook)

	logger.WithField("payment_id", "pay_123").WithField("wallet_id", "wallet_456").Info("payment submitted")
	logger.WithField("wallet_id", "wallet_456").Warn("horizon retry")

	require.NoError(t, hook.Close())

	require.EqualValues(t, 1, atomic.LoadInt32(&requestCount), "expected exactly one batched POST")

	mu.Lock()
	body := gotBody
	contentType := gotContentType
	mu.Unlock()

	assert.Equal(t, "application/json", contentType)

	var payload lokiPushRequest
	require.NoError(t, json.Unmarshal(body, &payload))

	// Two distinct levels were logged, so we expect two streams (Loki
	// requires one stream per distinct label set).
	require.Len(t, payload.Streams, 2)

	var infoStream, warnStream *lokiStream
	for i := range payload.Streams {
		s := &payload.Streams[i]
		assert.Equal(t, "sdp-api-test", s.Stream["service"])
		assert.Len(t, s.Stream, 2, "stream labels must stay low-cardinality: service+level only, no payment_id/wallet_id")

		switch s.Stream["level"] {
		case "info":
			infoStream = s
		case "warning":
			warnStream = s
		}
	}
	require.NotNil(t, infoStream, "expected an info-level stream")
	require.NotNil(t, warnStream, "expected a warning-level stream")

	require.Len(t, infoStream.Values, 1)
	assert.Contains(t, infoStream.Values[0][1], "payment submitted")
	assert.Contains(t, infoStream.Values[0][1], "payment_id=pay_123")
	assert.Contains(t, infoStream.Values[0][1], "wallet_id=wallet_456")
	assert.NotEmpty(t, infoStream.Values[0][0])

	require.Len(t, warnStream.Values, 1)
	assert.Contains(t, warnStream.Values[0][1], "horizon retry")
	assert.Contains(t, warnStream.Values[0][1], "wallet_id=wallet_456")
}

// Test_LokiHook_QueueFullDropsWithoutBlockingOrPanicking simulates a stuck
// Loki endpoint (a handler that never responds) so the background goroutine
// gets stuck mid-flush and stops draining the queue. It then bursts far more
// log calls than the (tiny, deliberately-configured) queue can hold, and
// asserts that logging never blocks the caller and never panics -- a Loki
// outage must be invisible to the rest of the app.
func Test_LokiHook_QueueFullDropsWithoutBlockingOrPanicking(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release // hang until the test explicitly lets this request finish
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	hook := NewLokiHook(LokiHookConfig{
		PushURL:       server.URL,
		BatchInterval: time.Hour,
		BatchSize:     1, // flush after the very first queued entry, so the background goroutine gets stuck on the hanging request almost immediately
		MaxQueueSize:  1, // tiny queue so a short burst overflows it deterministically
		HTTPClient:    &http.Client{Timeout: time.Minute},
	})
	defer func() {
		close(release)
		_ = hook.Close()
	}()

	logger := logrus.New()
	logger.SetOutput(io.Discard)
	logger.AddHook(hook)

	done := make(chan struct{})
	go func() {
		defer close(done)
		assert.NotPanics(t, func() {
			for i := 0; i < 500; i++ {
				logger.WithField("i", i).Info("burst")
			}
		})
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Fire() blocked the caller instead of dropping once the queue filled up")
	}
}
