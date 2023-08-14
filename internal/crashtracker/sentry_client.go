package crashtracker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/stellar/go/support/log"
)

type hubSentryInterface interface {
	CaptureException(exception error) *sentry.EventID
	CaptureMessage(message string) *sentry.EventID
	Clone() *sentry.Hub
	Flush(timeout time.Duration) bool
	Recover(err interface{}) *sentry.EventID
}

// Ensuring that *sentry.Hub is implementing hubSentryInterface interface.
var _ hubSentryInterface = (*sentry.Hub)(nil)

type sentryInterface interface {
	Init(options sentry.ClientOptions) error
	GetHubFromContext(ctx context.Context) hubSentryInterface
	CurrentHub() hubSentryInterface
}

// sentryImplementation implements the sentry interface methods using the sentry module.
type sentryImplementation struct{}

func (s *sentryImplementation) Init(options sentry.ClientOptions) error {
	return sentry.Init(options)
}

func (s *sentryImplementation) GetHubFromContext(ctx context.Context) hubSentryInterface {
	return sentry.GetHubFromContext(ctx)
}

func (s *sentryImplementation) CurrentHub() hubSentryInterface {
	return sentry.CurrentHub()
}

// Ensuring that *sentryImplementation is implementing sentryInterface interface.
var _ sentryInterface = (*sentryImplementation)(nil)

type sentryClient struct {
	hub                  hubSentryInterface
	sentryImplementation sentryInterface
}

// LogAndReportErrors is a method responsible to receive a err and a message and log this info before capture the exception with sentry.
func (s *sentryClient) LogAndReportErrors(ctx context.Context, err error, msg string) {
	// check if error is context canceled:
	if errors.Is(err, context.Canceled) {
		log.Warn("context canceled, not reporting error to sentry")
		return
	}

	if msg != "" {
		err = fmt.Errorf("%s: %w", msg, err)
	}
	log.Ctx(ctx).WithStack(err).Errorf("%+v", err)
	s.hub.CaptureException(err)
}

// LogAndReportMessages is a method responsible to receive a message and log this info before capture a message with sentry.
func (s *sentryClient) LogAndReportMessages(ctx context.Context, msg string) {
	log.Ctx(ctx).Info(msg)
	s.hub.CaptureMessage(msg)
}

// FlushEvents is a method that implements a timeout for events to be dispatched after an application terminates.
func (s *sentryClient) FlushEvents(waitTime time.Duration) bool {
	return s.hub.Flush(waitTime)
}

// Recover is a method that capture unhandled panics.
func (s *sentryClient) Recover() {
	if err := recover(); err != nil {
		s.hub.Recover(err)
	}
}

// Clone is a method that clones a new CrashTrackerClient to be used in concurrent routines.
func (s *sentryClient) Clone() CrashTrackerClient {
	cloneHub := s.hub.Clone()
	return &sentryClient{hub: cloneHub}
}

// NewSentryClient is a func that creates a new sentryClient using the sentryImplementation.
func NewSentryClient(sentryDSN string, environment string, gitCommit string) (*sentryClient, error) {
	si := &sentryImplementation{}
	err := si.Init(sentry.ClientOptions{
		Dsn:         sentryDSN,
		Release:     gitCommit,
		Environment: environment,
	})
	if err != nil {
		return nil, fmt.Errorf("error setting up Sentry: %w", err)
	}

	hub := si.CurrentHub()
	return &sentryClient{hub: hub, sentryImplementation: si}, nil
}

// Ensuring that sentryClient is implementing CrashTrackerClient interface
var _ CrashTrackerClient = (*sentryClient)(nil)
