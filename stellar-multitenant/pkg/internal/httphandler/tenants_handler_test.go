package httphandler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Get(t *testing.T) {
	dbt := dbtest.OpenWithTenantMigrationsOnly(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	handler := &TenantsHandler{
		Manager: &tenant.Manager{},
	}

	t.Run("GetAll successfully returns a list of all tenants", func(t *testing.T) {
		tenant.DeleteAllTenantFixtures(t, ctx, dbConnectionPool)

		tnt1 := tenant.CreateTenantFixture(t, ctx, dbConnectionPool, "myorg1")
		tnt2 := tenant.CreateTenantFixture(t, ctx, dbConnectionPool, "myorg2")

		tnt1JSON, err := json.Marshal(tnt1)
		require.NoError(t, err)
		tnt2JSON, err := json.Marshal(tnt2)
		require.NoError(t, err)

		expectedJSON := fmt.Sprintf("[%s, %s]", tnt1JSON, tnt2JSON)

		rr := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/tenants/", nil)
		http.HandlerFunc(handler.GetAll).ServeHTTP(rr, req)

		resp := rr.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, expectedJSON, string(respBody))
	})

	t.Run("GetByID successfully returns a tenant by ID", func(t *testing.T) {
		tenant.DeleteAllTenantFixtures(t, ctx, dbConnectionPool)

		tnt1 := tenant.CreateTenantFixture(t, ctx, dbConnectionPool, "myorg1")
		_ = tenant.CreateTenantFixture(t, ctx, dbConnectionPool, "myorg2")
		tnt1JSON, err := json.Marshal(tnt1)
		require.NoError(t, err)

		url := fmt.Sprintf("/tenants/%s", tnt1.ID)
		rr := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", url, nil)
		http.HandlerFunc(handler.GetByID).ServeHTTP(rr, req)

		resp := rr.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, string(tnt1JSON), string(respBody))
	})

	t.Run("GetByName successfully returns a tenant by name", func(t *testing.T) {
		tenant.DeleteAllTenantFixtures(t, ctx, dbConnectionPool)

		_ = tenant.CreateTenantFixture(t, ctx, dbConnectionPool, "myorg1")
		tnt2 := tenant.CreateTenantFixture(t, ctx, dbConnectionPool, "myorg2")
		tnt2JSON, err := json.Marshal(tnt2)
		require.NoError(t, err)

		url := fmt.Sprintf("/tenants/%s", tnt2.Name)
		rr := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", url, nil)
		http.HandlerFunc(handler.GetByName).ServeHTTP(rr, req)

		resp := rr.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, string(tnt2JSON), string(respBody))
	})
}
