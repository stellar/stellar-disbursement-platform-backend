package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"

	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
	"github.com/stretchr/testify/assert"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/scheduler/jobs"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stretchr/testify/require"
)

func TestScheduler(t *testing.T) {
	t.Parallel()

	_, cancel := context.WithCancel(context.Background())
	scheduler := newScheduler(cancel)

	mockCrashTrackerClient := &crashtracker.MockCrashTrackerClient{}
	scheduler.crashTrackerClient = mockCrashTrackerClient

	clone := crashtracker.MockCrashTrackerClient{}
	mockCrashTrackerClient.On("Clone").Return(&clone).Times(5)

	mockJob1 := &jobs.MockJob{
		Name:       "mock_job_1",
		Interval:   1 * time.Second,
		Executions: 0,
	}

	mockJob2 := &jobs.MockJob{
		Name:       "mock_job_2",
		Interval:   20 * time.Second,
		Executions: 0,
	}

	scheduler.addJob(mockJob1)
	scheduler.addJob(mockJob2)

	// Start the scheduler and wait for a short period to let the job run
	scheduler.start(context.Background())
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

func TestMultiTenantScheduler(t *testing.T) {
	t.Parallel()

	_, cancel := context.WithCancel(context.Background())
	scheduler := newScheduler(cancel)

	mockCrashTrackerClient := &crashtracker.MockCrashTrackerClient{}
	scheduler.crashTrackerClient = mockCrashTrackerClient

	clone := crashtracker.MockCrashTrackerClient{}
	mockCrashTrackerClient.On("Clone").Return(&clone).Times(5)

	mockTenantManager := &tenant.TenantManagerMock{}
	scheduler.tenantManager = mockTenantManager

	tenant1 := tenant.Tenant{ID: "tenant1", Name: "Tenant 1"}
	tenant2 := tenant.Tenant{ID: "tenant2", Name: "Tenant 2"}

	mockTenantManager.On("GetAllTenants", mock.Anything, mock.Anything).
		Return([]tenant.Tenant{tenant1, tenant2}, nil).
		Once()

	mockJob := &jobs.MockMultiTenantJob{
		Name:     "mock_job_1",
		Interval: 1 * time.Second,
	}

	scheduler.addJob(mockJob)

	// Start the scheduler and wait for a short period to let the job run
	scheduler.start(context.Background())
	time.Sleep(2 * time.Second)

	tenant1Executions := mockJob.GetExecutions(tenant1.ID)
	assert.True(t, tenant1Executions > 0, "Expected job to be executed at least once, but it was executed %d times", tenant1Executions)

	tenant2Executions := mockJob.GetExecutions(tenant2.ID)
	assert.Equal(t, tenant1Executions, tenant2Executions, "Expected both tenants to have the same number of executions, but tenant1 had %d and tenant2 had %d", tenant1Executions, tenant2Executions)

	// Test stopping the scheduler
	cancel()
	time.Sleep(1 * time.Second)

	mockCrashTrackerClient.AssertExpectations(t)
	mockTenantManager.AssertExpectations(t)
}
