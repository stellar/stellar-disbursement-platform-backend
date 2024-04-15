package jobs

import (
	"context"
	"sync"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

// MockJob is a mock job created for testing purposes
type MockJob struct {
	Name       string
	Interval   time.Duration
	Executions int
	mu         sync.Mutex
}

func (m *MockJob) GetName() string {
	return m.Name
}

func (m *MockJob) GetInterval() time.Duration {
	return m.Interval
}

func (m *MockJob) Execute(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Executions++
	return nil
}

func (m *MockJob) GetExecutions() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Executions
}

func (m *MockJob) IsJobMultiTenant() bool {
	return false
}

// verify that MockJob implements the Job interface
var _ Job = (*MockJob)(nil)

// MockMultiTenantJob is a mock multi-tenant job created for testing purposes
type MockMultiTenantJob struct {
	Name       string
	Interval   time.Duration
	Executions sync.Map
}

func (m *MockMultiTenantJob) GetName() string {
	return m.Name
}

func (m *MockMultiTenantJob) GetInterval() time.Duration {
	return m.Interval
}

func (m *MockMultiTenantJob) Execute(ctx context.Context) error {
	tnt, err := tenant.GetTenantFromContext(ctx)
	if err != nil {
		return err
	}
	execs, ok := m.Executions.Load(tnt.ID)
	if !ok {
		execs = 0
	}
	m.Executions.Store(tnt.ID, execs.(int)+1)
	return nil
}

func (m *MockMultiTenantJob) GetExecutions(id string) int {
	value, ok := m.Executions.Load(id)
	if !ok {
		return 0
	}
	return value.(int)
}

func (m *MockMultiTenantJob) IsJobMultiTenant() bool {
	return true
}

// verify that MockMultiTenantJob implements the Job interface
var _ Job = (*MockMultiTenantJob)(nil)
