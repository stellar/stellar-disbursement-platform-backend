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

type testInterface interface {
	mock.TestingT
	Cleanup(func())
}

// NewMockCrashTrackerClient creates a new instance of MockCrashTrackerClient. It also registers a testing interface on
// the mock and a cleanup function to assert the mocks expectations.
func NewMockCrashTrackerClient(t testInterface) *MockCrashTrackerClient {
	mock := &MockCrashTrackerClient{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
