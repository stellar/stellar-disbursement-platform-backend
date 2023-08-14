package scheduler

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stretchr/testify/require"
)

// MockJob is a mock job created for testing purposes
type MockJob struct {
	name       string
	interval   time.Duration
	executions int
	mu         sync.Mutex
}

func (m *MockJob) GetName() string {
	return m.name
}

func (m *MockJob) GetInterval() time.Duration {
	return m.interval
}

func (m *MockJob) Execute(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.executions++
	return nil
}

func (m *MockJob) GetExecutions() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.executions
}

func TestScheduler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	scheduler := newScheduler(cancel)

	mockCrashTrackerClient := &crashtracker.MockCrashTrackerClient{}
	scheduler.crashTrackerClient = mockCrashTrackerClient

	clone := crashtracker.MockCrashTrackerClient{}
	mockCrashTrackerClient.On("Clone").Return(&clone).Times(5)

	mockJob1 := &MockJob{
		name:       "mock_job_1",
		interval:   1 * time.Second,
		executions: 0,
	}

	mockJob2 := &MockJob{
		name:       "mock_job_2",
		interval:   20 * time.Second,
		executions: 0,
	}

	scheduler.addJob(mockJob1)
	scheduler.addJob(mockJob2)

	// Start the scheduler and wait for a short period to let the job run
	scheduler.start(ctx)
	time.Sleep(2 * time.Second)

	job1Executions := mockJob1.GetExecutions()
	require.True(t, job1Executions > 0, "Expected job to be executed at least once, but it was executed %d times", job1Executions)

	job2Executions := mockJob2.GetExecutions()
	require.True(t, job2Executions == 0, "Expected job to be executed 0 times, but it was executed %d times", job2Executions)

	// Test stopping the scheduler
	cancel()
	time.Sleep(1 * time.Second)

	mockCrashTrackerClient.AssertExpectations(t)
}
