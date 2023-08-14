package crashtracker

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/stellar/go/support/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockHubSentry struct {
	mock.Mock
}

func (m *mockHubSentry) CaptureException(exception error) *sentry.EventID {
	return m.Called(exception).Get(0).(*sentry.EventID)
}

func (m *mockHubSentry) CaptureMessage(message string) *sentry.EventID {
	return m.Called(message).Get(0).(*sentry.EventID)
}

func (m *mockHubSentry) Clone() *sentry.Hub {
	return m.Called().Get(0).(*sentry.Hub)
}

func (m *mockHubSentry) Flush(timeout time.Duration) bool {
	return m.Called(timeout).Get(0).(bool)
}

func (m *mockHubSentry) Recover(err interface{}) *sentry.EventID {
	return m.Called(err).Get(0).(*sentry.EventID)
}

// Ensuring that mockSentry is implementing sentryInterface interface
var _ hubSentryInterface = (*mockHubSentry)(nil)

type mockSentry struct {
	mock.Mock
}

func (m *mockSentry) Init(options sentry.ClientOptions) error {
	return m.Called(options).Error(0)
}

func (m *mockSentry) GetHubFromContext(ctx context.Context) hubSentryInterface {
	return m.Called(ctx).Get(0).(*mockHubSentry)
}

func (m *mockSentry) CurrentHub() hubSentryInterface {
	return m.Called().Get(0).(*mockHubSentry)
}

// Ensuring that *mockSentry is implementing sentryInterface interface.
var _ sentryInterface = (*mockSentry)(nil)

func Test_SentryClient_LogAndReportErrors(t *testing.T) {
	mHubSentry := &mockHubSentry{}

	mSentryClient := &sentryClient{
		hub: mHubSentry,
	}
	mMsgError := "error"
	mError := fmt.Errorf("mock error")
	ctx := context.Background()

	t.Run("LogAndReportErrors without message", func(t *testing.T) {
		e := fmt.Errorf("%s: %w", mMsgError, mError)
		sentryId := sentry.EventID("id-1")

		mHubSentry.On("CaptureException", e).Return(&sentryId).Once()
		mSentryClient.LogAndReportErrors(ctx, mError, mMsgError)

		mHubSentry.AssertExpectations(t)
	})

	t.Run("LogAndReportErrors with message", func(t *testing.T) {
		mMsgError = ""
		sentryId := sentry.EventID("id-1")

		mHubSentry.On("CaptureException", mError).Return(&sentryId).Once()
		mSentryClient.LogAndReportErrors(ctx, mError, mMsgError)

		mHubSentry.AssertExpectations(t)
	})

	t.Run("LogAndReportErrors ignores context.Canceled", func(t *testing.T) {
		mHubSentry = &mockHubSentry{}
		mSentryClient = &sentryClient{hub: mHubSentry}

		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)

		err := fmt.Errorf("external error that wraps: %w", context.Canceled)
		mSentryClient.LogAndReportErrors(ctx, err, mMsgError)
		mHubSentry.AssertNotCalled(t, "CaptureException", mock.Anything)

		require.Contains(t, buf.String(), "context canceled, not reporting error to sentry")
	})
}

func Test_SentryClient_LogAndReportMessages(t *testing.T) {
	mHubSentry := &mockHubSentry{}

	mSentryClient := &sentryClient{
		hub: mHubSentry,
	}
	mMsgError := "crash error"

	sentryId := sentry.EventID("id-1")

	mHubSentry.On("CaptureMessage", mMsgError).Return(&sentryId).Once()
	mSentryClient.LogAndReportMessages(context.Background(), mMsgError)

	mHubSentry.AssertExpectations(t)
}

func Test_SentryClient_FlushEvents(t *testing.T) {
	mHubSentry := &mockHubSentry{}

	mSentryClient := &sentryClient{
		hub: mHubSentry,
	}
	waitTimeout := time.Second

	mHubSentry.On("Flush", waitTimeout).Return(true).Once()
	mSentryClient.FlushEvents(waitTimeout)

	mHubSentry.AssertExpectations(t)
}

func Test_SentryClient_Recover(t *testing.T) {
	mHubSentry := &mockHubSentry{}

	mSentryClient := &sentryClient{
		hub: mHubSentry,
	}

	mockErr := fmt.Errorf("error test")
	sentryId := sentry.EventID("id-1")

	mHubSentry.On("Recover", mockErr).Return(&sentryId).Once()

	defer mHubSentry.AssertExpectations(t)
	defer mSentryClient.Recover()

	panic(mockErr)
}

func Test_SentryClient_Clone(t *testing.T) {
	mHubSentry := &mockHubSentry{}

	mSentryClient := &sentryClient{
		hub: mHubSentry,
	}

	hub := sentry.Hub{}
	mHubSentry.On("Clone").Return(&hub).Once()

	cloneClient := mSentryClient.Clone()

	sc := cloneClient.(*sentryClient)
	assert.Equal(t, &hub, sc.hub)

	mHubSentry.AssertExpectations(t)
}
