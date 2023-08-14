package crashtracker

import (
	"context"
	"time"

	"github.com/stretchr/testify/mock"
)

type MockCrashTrackerClient struct {
	mock.Mock
}

func (m *MockCrashTrackerClient) LogAndReportErrors(ctx context.Context, err error, msg string) {
	m.Called(ctx, err, msg)
}

func (m *MockCrashTrackerClient) LogAndReportMessages(ctx context.Context, msg string) {
	m.Called(ctx, msg)
}

func (m *MockCrashTrackerClient) FlushEvents(waitTime time.Duration) bool {
	return m.Called(waitTime).Get(0).(bool)
}

func (m *MockCrashTrackerClient) Recover() {
	m.Called()
}

func (m *MockCrashTrackerClient) Clone() CrashTrackerClient {
	return m.Called().Get(0).(*MockCrashTrackerClient)
}

// Ensuring that MockCrashTrackerClient is implementing CrashTrackerClient interface
var _ CrashTrackerClient = (*MockCrashTrackerClient)(nil)
