package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

func TestTenantResolutionMiddleware(t *testing.T) {
	tests := []struct {
		name           string
		singleTenant   bool
		setupMock      func(*tenant.TenantManagerMock)
		expectedOutput string
	}{
		{
			name:           "Single tenant mode with one tenant",
			singleTenant:   true,
			setupMock:      func(m *tenant.TenantManagerMock) { setupSingleTenant(m, "single-tenant-id", "test-tenant") },
			expectedOutput: "single-tenant-id",
		},
		{
			name:           "Default tenant already set",
			singleTenant:   true,
			setupMock:      func(m *tenant.TenantManagerMock) { setupSingleTenant(m, "default-tenant-id", "default-tenant") },
			expectedOutput: "default-tenant-id",
		},
		{
			name:           "Multiple tenants",
			singleTenant:   true,
			setupMock:      setupMultipleTenants,
			expectedOutput: "no tenant",
		},
		{
			name:           "Not in single tenant mode",
			singleTenant:   false,
			setupMock:      func(m *tenant.TenantManagerMock) {}, // No-op, no mock setup needed
			expectedOutput: "no tenant",
		},
		{
			name:           "Error getting tenants",
			singleTenant:   true,
			setupMock:      setupError,
			expectedOutput: "no tenant",
		},
	}

	// Run test cases
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockManager := tenant.NewTenantManagerMock(t)
			tc.setupMock(mockManager)
			
			middleware := TenantResolutionMiddleware(mockManager, tc.singleTenant)
			handler := middleware(createTestHandler())
			
			req := httptest.NewRequest("GET", "/", nil)
			rec := httptest.NewRecorder()
			
			handler.ServeHTTP(rec, req)
			
			assert.Equal(t, tc.expectedOutput, rec.Body.String())
		})
	}
}

// Test helper function to create a test handler
func createTestHandler() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tnt, err := tenant.GetTenantFromContext(r.Context())
		if err == nil {
			w.Write([]byte(tnt.ID))
		} else {
			w.Write([]byte("no tenant"))
		}
	})
}

func setupSingleTenant(m *tenant.TenantManagerMock, id, name string) {
	mockTenant := &tenant.Tenant{ID: id, Name: name}
	m.On("EnsureDefaultTenant", mock.Anything).Return(mockTenant, nil)
}

func setupMultipleTenants(m *tenant.TenantManagerMock) {
	m.On("EnsureDefaultTenant", mock.Anything).Return(nil, tenant.ErrTenantDoesNotExist)
}

func setupError(m *tenant.TenantManagerMock) {
	m.On("EnsureDefaultTenant", mock.Anything).Return(nil, assert.AnError)
}