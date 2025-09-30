package httphandler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
)

const adminUserID = "b9c29a1a-4d30-4b99-8c5f-0546054be91b"

func getDBConnectionPool(t *testing.T) db.DBConnectionPool {
	dbt := dbtest.Open(t)
	t.Cleanup(func() {
		dbt.Close()
	})

	pool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)

	t.Cleanup(func() { pool.Close() })

	return pool
}

func setupHandler(t *testing.T) (APIKeyHandler, context.Context) {
	pool := getDBConnectionPool(t)
	models, err := data.NewModels(pool)
	require.NoError(t, err)

	handler := APIKeyHandler{Models: models}
	ctx := sdpcontext.SetUserIDInContext(context.Background(), adminUserID)

	return handler, ctx
}

func Test_CreateAPIKey_WithAllFields(t *testing.T) {
	handler, ctx := setupHandler(t)

	expiry := time.Now().Add(24 * time.Hour)
	reqBody := map[string]any{
		"name":        "Relic of the Omnissiah",
		"permissions": []string{"read:statistics", "read:exports"},
		"allowed_ips": data.IPList{"198.51.100.0/24"},
		"expiry_date": expiry,
	}
	b, err := json.Marshal(reqBody)
	require.NoError(t, err)
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api-keys", bytes.NewReader(b))
	rr := httptest.NewRecorder()

	handler.CreateAPIKey(rr, req)
	resp := rr.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	var out data.APIKey
	dataBytes, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(dataBytes, &out))

	assert.NotEmpty(t, out.ID)
	assert.Equal(t, "Relic of the Omnissiah", out.Name)
	assert.NotEmpty(t, out.Key)
	assert.Contains(t, out.Key, "SDP_")
	assert.ElementsMatch(t, []data.APIKeyPermission{data.ReadStatistics, data.ReadExports}, out.Permissions)
	assert.Equal(t, data.IPList{"198.51.100.0/24"}, out.AllowedIPs)
	require.NotNil(t, out.ExpiryDate)
	assert.WithinDuration(t, expiry, *out.ExpiryDate, time.Second)
	assert.Equal(t, adminUserID, out.CreatedBy)
}

func Test_CreateAPIKey_WithMinimumFields(t *testing.T) {
	handler, ctx := setupHandler(t)

	reqBody := map[string]any{
		"name":        "Magos Dominus Access Key",
		"permissions": []string{"read:all"},
	}
	b, err := json.Marshal(reqBody)
	require.NoError(t, err)
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api-keys", bytes.NewReader(b))
	rr := httptest.NewRecorder()

	handler.CreateAPIKey(rr, req)
	resp := rr.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	var out data.APIKey
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))

	assert.NotEmpty(t, out.ID)
	assert.Equal(t, "Magos Dominus Access Key", out.Name)
	assert.NotEmpty(t, out.Key)
	assert.ElementsMatch(t, []data.APIKeyPermission{data.ReadAll}, out.Permissions)
	assert.Empty(t, out.AllowedIPs)
	assert.Nil(t, out.ExpiryDate)
}

func TestUpdateKey_AllowedIPsHandling(t *testing.T) {
	t.Parallel()
	handler, ctx := setupHandler(t)

	r := chi.NewRouter()
	r.Patch("/api-keys/{id}", handler.UpdateKey)

	originalKey, err := handler.Models.APIKeys.Insert(
		ctx,
		"Techpriest Archive Key",
		[]data.APIKeyPermission{data.ReadAll},
		[]string{"10.0.0.0/8"},
		nil,
		adminUserID,
	)
	require.NoError(t, err)

	successCases := []struct {
		name       string
		allowedIPs any
		expected   data.IPList
	}{
		{
			name:       "single string IP",
			allowedIPs: "203.0.113.5",
			expected:   data.IPList{"203.0.113.5"},
		},
		{
			name:       "array of IP strings",
			allowedIPs: data.IPList{"192.168.1.0/24", "10.0.0.0/8"},
			expected:   data.IPList{"192.168.1.0/24", "10.0.0.0/8"},
		},
		{
			name:       "empty array",
			allowedIPs: data.IPList{},
			expected:   data.IPList{},
		},
	}

	for _, tc := range successCases {
		t.Run(tc.name, func(t *testing.T) {
			reqBody := map[string]any{
				"permissions": []string{"read:statistics", "read:exports"},
				"allowed_ips": tc.allowedIPs,
			}
			b, err := json.Marshal(reqBody)
			require.NoError(t, err)

			req := httptest.NewRequestWithContext(
				ctx,
				http.MethodPatch,
				"/api-keys/"+originalKey.ID,
				bytes.NewReader(b),
			)
			rr := httptest.NewRecorder()

			r.ServeHTTP(rr, req)

			resp := rr.Result()
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)
			var out data.APIKey
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
			assert.ElementsMatch(t, tc.expected, out.AllowedIPs)
		})
	}
}

func Test_CreateAPIKey_ValidationErrors(t *testing.T) {
	errorCases := []struct {
		name          string
		requestBody   map[string]any
		expectedField string
	}{
		{
			name:          "missing name",
			requestBody:   map[string]any{"permissions": []string{"read:statistics"}},
			expectedField: "name",
		},
		{
			name:          "missing permissions",
			requestBody:   map[string]any{"name": "Null Permissions Key"},
			expectedField: "permissions",
		},
		{
			name:          "empty permissions array",
			requestBody:   map[string]any{"name": "Empty Permissions Key", "permissions": []string{}},
			expectedField: "permissions",
		},
		{
			name:          "invalid permissions",
			requestBody:   map[string]any{"name": "Heretical Key", "permissions": []string{"read:statistics", "nope:invalid"}},
			expectedField: "permissions",
		},
		{
			name: "past expiry date",
			requestBody: map[string]any{
				"name":        "Chronometron Key",
				"permissions": []string{"read:statistics"},
				"expiry_date": time.Now().Add(-time.Hour),
			},
			expectedField: "expiry_date",
		},
	}

	for _, tc := range errorCases {
		t.Run(tc.name, func(t *testing.T) {
			handler, ctx := setupHandler(t)

			b, err := json.Marshal(tc.requestBody)
			require.NoError(t, err)
			req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api-keys", bytes.NewReader(b))
			rr := httptest.NewRecorder()

			handler.CreateAPIKey(rr, req)
			resp := rr.Result()
			defer resp.Body.Close()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
			var errResp map[string]any
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&errResp))
			fields := errResp["extras"].(map[string]any)
			assert.Contains(t, fields, tc.expectedField)
		})
	}
}

func TestCreateAPIKey_IPValidationErrors(t *testing.T) {
	ipErrorCases := []struct {
		name       string
		allowedIPs any
	}{
		{
			name:       "invalid IP address",
			allowedIPs: []string{"198.51.100.0/24", "bad-ip"},
		},
		{
			name:       "invalid CIDR notation",
			allowedIPs: []string{"192.168.1.0/40"}, // Invalid CIDR (max is /32)
		},
		{
			name:       "invalid type (number)",
			allowedIPs: 12345,
		},
		{
			name:       "mixed types in array",
			allowedIPs: []any{"192.168.1.1", 12345},
		},
	}

	for _, tc := range ipErrorCases {
		t.Run(tc.name, func(t *testing.T) {
			handler, ctx := setupHandler(t)

			reqBody := map[string]any{
				"name":        "Magos Biologis Key - " + tc.name,
				"permissions": []string{"read:statistics"},
				"allowed_ips": tc.allowedIPs,
			}
			b, err := json.Marshal(reqBody)
			require.NoError(t, err)
			req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api-keys", bytes.NewReader(b))
			rr := httptest.NewRecorder()

			handler.CreateAPIKey(rr, req)
			resp := rr.Result()
			defer resp.Body.Close()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
			var errResp map[string]any
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&errResp))
			fields := errResp["extras"].(map[string]any)
			assert.Contains(t, fields, "allowed_ips")
		})
	}
}

func Test_CreateAPIKey_InvalidJSON(t *testing.T) {
	handler, ctx := setupHandler(t)

	invalid := []byte(`{invalid-json}`)
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api-keys", bytes.NewReader(invalid))
	rr := httptest.NewRecorder()

	handler.CreateAPIKey(rr, req)
	resp := rr.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestCreateAPIKey_MissingUserID(t *testing.T) {
	pool := getDBConnectionPool(t)
	models, err := data.NewModels(pool)
	require.NoError(t, err)
	handler := APIKeyHandler{Models: models}

	emptyCtx := context.Background()

	reqBody := map[string]any{
		"name":        "Adeptus Mechanicus Key",
		"permissions": []string{"read:statistics"},
	}
	b, err := json.Marshal(reqBody)
	require.NoError(t, err)
	req := httptest.NewRequestWithContext(emptyCtx, http.MethodPost, "/api-keys", bytes.NewReader(b))
	rr := httptest.NewRecorder()

	handler.CreateAPIKey(rr, req)
	resp := rr.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

func Test_GetAllAPIKeys(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		handler, ctx := setupHandler(t)
		userID := adminUserID

		k1, err := handler.Models.APIKeys.Insert(ctx,
			"Eisenhorn Archive Key",
			[]data.APIKeyPermission{data.ReadAll},
			nil,
			nil,
			userID,
		)
		require.NoError(t, err)

		k2, err := handler.Models.APIKeys.Insert(ctx,
			"Cicatrix Maledictum Cipher",
			[]data.APIKeyPermission{data.ReadStatistics},
			[]string{"203.0.113.0/24"},
			nil,
			userID,
		)
		require.NoError(t, err)

		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api-keys", nil)
		rr := httptest.NewRecorder()
		handler.GetAllAPIKeys(rr, req)
		res := rr.Result()
		defer res.Body.Close()

		assert.Equal(t, http.StatusOK, res.StatusCode)

		var list []data.APIKey
		require.NoError(t, json.NewDecoder(res.Body).Decode(&list))

		require.Len(t, list, 2)
		// newest first
		assert.Equal(t, k2.ID, list[0].ID)
		assert.Equal(t, "Cicatrix Maledictum Cipher", list[0].Name)
		assert.ElementsMatch(t, []data.APIKeyPermission{data.ReadStatistics}, list[0].Permissions)
		assert.Equal(t, data.IPList{"203.0.113.0/24"}, list[0].AllowedIPs)

		assert.Equal(t, k1.ID, list[1].ID)
		assert.Equal(t, "Eisenhorn Archive Key", list[1].Name)
		assert.ElementsMatch(t, []data.APIKeyPermission{data.ReadAll}, list[1].Permissions)
		assert.Empty(t, list[1].AllowedIPs)
	})

	t.Run("missing user ID in context", func(t *testing.T) {
		handler, _ := setupHandler(t)
		// Create request without user ID in context
		req := httptest.NewRequest(http.MethodGet, "/api-keys", nil)
		rr := httptest.NewRecorder()
		handler.GetAllAPIKeys(rr, req)
		assert.Equal(t, http.StatusInternalServerError, rr.Code)
		assert.Contains(t, rr.Body.String(), "User identification error")
	})
}

func Test_DeleteAPIKeyEndpoints(t *testing.T) {
	t.Parallel()
	handler, ctx := setupHandler(t)

	r := chi.NewRouter()
	r.Delete("/api-keys/{id}", handler.DeleteAPIKey)

	t.Run("success", func(t *testing.T) {
		key, err := handler.Models.APIKeys.Insert(
			ctx,
			"Tempestus Scion Key",
			[]data.APIKeyPermission{data.ReadAll},
			nil, nil,
			adminUserID,
		)
		require.NoError(t, err)

		req := httptest.NewRequestWithContext(ctx, http.MethodDelete, "/api-keys/"+key.ID, nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		require.Equal(t, http.StatusNoContent, rr.Code)
	})

	t.Run("not found", func(t *testing.T) {
		fake := "00000000-0000-0000-0000-000000000000"
		req := httptest.NewRequestWithContext(ctx, http.MethodDelete, "/api-keys/"+fake, nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("other user cannot delete", func(t *testing.T) {
		key, err := handler.Models.APIKeys.Insert(
			ctx,
			"Stormcaller Relic Key",
			[]data.APIKeyPermission{data.ReadAll},
			nil, nil,
			adminUserID,
		)
		require.NoError(t, err)

		otherCtx := sdpcontext.SetUserIDInContext(context.Background(), "11111111-2222-3333-4444-555555555555")
		req := httptest.NewRequestWithContext(otherCtx, http.MethodDelete, "/api-keys/"+key.ID, nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("missing user id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api-keys/irrelevant", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusInternalServerError, rr.Code)
	})
}

func Test_GetAPIKeyByIDEndpoints(t *testing.T) {
	t.Parallel()
	handler, ctx := setupHandler(t)

	r := chi.NewRouter()
	r.Get("/api-keys/{id}", handler.GetAPIKeyByID)

	t.Run("success", func(t *testing.T) {
		expiry := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second)
		key, err := handler.Models.APIKeys.Insert(
			ctx,
			"Vox Imperator Index Key",
			[]data.APIKeyPermission{data.ReadStatistics, data.ReadExports},
			[]string{"198.51.100.0/24"},
			&expiry,
			adminUserID,
		)
		require.NoError(t, err)

		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api-keys/"+key.ID, nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		var out data.APIKey
		require.NoError(t, json.NewDecoder(rr.Body).Decode(&out))

		assert.Equal(t, key.ID, out.ID)
		assert.Equal(t, "Vox Imperator Index Key", out.Name)
		assert.ElementsMatch(t, key.Permissions, out.Permissions)
		assert.Equal(t, data.IPList{"198.51.100.0/24"}, out.AllowedIPs)

		require.NotNil(t, out.ExpiryDate)
		assert.WithinDuration(t, expiry, *out.ExpiryDate, time.Second)
		assert.WithinDuration(t, key.CreatedAt.UTC(), out.CreatedAt.UTC(), time.Second)
		assert.WithinDuration(t, key.UpdatedAt.UTC(), out.UpdatedAt.UTC(), time.Second)
		assert.Nil(t, out.LastUsedAt)
	})

	t.Run("not found", func(t *testing.T) {
		fake := "00000000-0000-0000-0000-000000000000"
		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api-keys/"+fake, nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("other user cannot access", func(t *testing.T) {
		key, err := handler.Models.APIKeys.Insert(
			ctx,
			"Iridium Tomb Key",
			[]data.APIKeyPermission{data.ReadAll},
			nil, nil,
			adminUserID,
		)
		require.NoError(t, err)

		otherCtx := sdpcontext.SetUserIDInContext(context.Background(), "11111111-2222-3333-4444-555555555555")
		req := httptest.NewRequestWithContext(otherCtx, http.MethodGet, "/api-keys/"+key.ID, nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("missing user ID in context", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api-keys/some-id", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusInternalServerError, rr.Code)
		assert.Contains(t, rr.Body.String(), "User identification error")
	})
}

func Test_UpdateKeyEndpoints(t *testing.T) {
	t.Parallel()
	handler, ctx := setupHandler(t)

	r := chi.NewRouter()
	r.Put("/api-keys/{id}", handler.UpdateKey)

	originalKey, err := handler.Models.APIKeys.Insert(
		ctx,
		"Adeptus Mechanicus Secret Key",
		[]data.APIKeyPermission{data.ReadAll, data.ReadStatistics},
		[]string{"10.0.0.0/8"},
		nil,
		adminUserID,
	)
	require.NoError(t, err)

	t.Run("success", func(t *testing.T) {
		reqBody := map[string]any{
			"permissions": []string{"read:statistics", "read:exports"},
			"allowed_ips": []string{"192.168.1.0/24", "203.0.113.42"},
		}
		b, err := json.Marshal(reqBody)
		require.NoError(t, err)

		req := httptest.NewRequestWithContext(ctx, http.MethodPut, "/api-keys/"+originalKey.ID, bytes.NewReader(b))
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		var updated data.APIKey
		require.NoError(t, json.NewDecoder(rr.Body).Decode(&updated))

		assert.Equal(t, originalKey.ID, updated.ID)
		assert.Equal(t, "Adeptus Mechanicus Secret Key", updated.Name) // Name shouldn't change
		assert.ElementsMatch(t, []data.APIKeyPermission{data.ReadStatistics, data.ReadExports}, updated.Permissions)
		assert.ElementsMatch(t, []string{"192.168.1.0/24", "203.0.113.42"}, updated.AllowedIPs)
	})

	t.Run("empty permissions", func(t *testing.T) {
		reqBody := map[string]any{
			"permissions": []string{},
			"allowed_ips": []string{"192.168.1.0/24"},
		}
		b, err := json.Marshal(reqBody)
		require.NoError(t, err)

		req := httptest.NewRequestWithContext(ctx, http.MethodPut, "/api-keys/"+originalKey.ID, bytes.NewReader(b))
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("invalid permissions", func(t *testing.T) {
		reqBody := map[string]any{
			"permissions": []string{"read:statistics", "heresy:purge"},
			"allowed_ips": []string{"192.168.1.0/24"},
		}
		b, err := json.Marshal(reqBody)
		require.NoError(t, err)

		req := httptest.NewRequestWithContext(ctx, http.MethodPut, "/api-keys/"+originalKey.ID, bytes.NewReader(b))
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("invalid IP format", func(t *testing.T) {
		reqBody := map[string]any{
			"permissions": []string{"read:statistics"},
			"allowed_ips": []string{"192.168.1.0/24", "not-an-ip"},
		}
		b, err := json.Marshal(reqBody)
		require.NoError(t, err)

		req := httptest.NewRequestWithContext(ctx, http.MethodPut, "/api-keys/"+originalKey.ID, bytes.NewReader(b))
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("not found", func(t *testing.T) {
		reqBody := map[string]any{
			"permissions": []string{"read:statistics"},
			"allowed_ips": []string{"192.168.1.0/24"},
		}
		b, err := json.Marshal(reqBody)
		require.NoError(t, err)

		req := httptest.NewRequestWithContext(ctx, http.MethodPut, "/api-keys/00000000-0000-0000-0000-000000000000", bytes.NewReader(b))
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("other user cannot update", func(t *testing.T) {
		reqBody := map[string]any{
			"permissions": []string{"read:statistics"},
			"allowed_ips": []string{"192.168.1.0/24"},
		}
		b, err := json.Marshal(reqBody)
		require.NoError(t, err)

		otherUserID := "11111111-2222-3333-4444-555555555555"
		otherCtx := sdpcontext.SetUserIDInContext(context.Background(), otherUserID)
		req := httptest.NewRequestWithContext(otherCtx, http.MethodPut, "/api-keys/"+originalKey.ID, bytes.NewReader(b))
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("invalid json", func(t *testing.T) {
		invalid := []byte(`{invalid-json}`)
		req := httptest.NewRequestWithContext(ctx, http.MethodPut, "/api-keys/"+originalKey.ID, bytes.NewReader(invalid))
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("missing user id", func(t *testing.T) {
		reqBody := map[string]any{
			"permissions": []string{"read:statistics"},
			"allowed_ips": []string{"192.168.1.0/24"},
		}
		b, err := json.Marshal(reqBody)
		require.NoError(t, err)

		emptyCtx := context.Background()
		req := httptest.NewRequestWithContext(emptyCtx, http.MethodPut, "/api-keys/"+originalKey.ID, bytes.NewReader(b))
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusInternalServerError, rr.Code)
	})
}
