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
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
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

	// The creator is an Owner, so a key that names no wallets inherits every active wallet.
	authManagerMock := &auth.AuthManagerMock{}
	authManagerMock.On("GetUserByID", mock.Anything, adminUserID).
		Return(&auth.User{ID: adminUserID, IsOwner: true}, nil).Maybe()

	handler := APIKeyHandler{Models: models, AuthManager: authManagerMock}
	ctx := sdpcontext.SetUserIDInContext(context.Background(), adminUserID)

	return handler, ctx
}

// Test_CreateAPIKey_WalletScope covers the creation-time ceiling: what a creator may put on a key
// is their own access, and nothing about the key changes afterwards when that access does.
func Test_CreateAPIKey_WalletScope(t *testing.T) {
	const developerUserID = "d24d2a52-9a4d-4b2f-b1b1-4d9f2a7f0c31"

	setup := func(t *testing.T, user *auth.User) (APIKeyHandler, context.Context, *data.DistributionWallet, string) {
		t.Helper()
		pool := getDBConnectionPool(t)
		models, err := data.NewModels(pool)
		require.NoError(t, err)

		ctx := sdpcontext.SetUserIDInContext(context.Background(), user.ID)
		walletA := data.EnsureDefaultDistributionWalletFixture(t, ctx, pool)
		var walletBID string
		require.NoError(t, pool.GetContext(ctx, &walletBID, `
			INSERT INTO distribution_wallets (name, distribution_account_type)
			VALUES ('scope-wallet-b', 'DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT') RETURNING id`))

		if !user.IsOwner {
			_, err = models.WalletMemberships.Insert(ctx, pool, user.ID, walletA.ID, data.DeveloperUserRole, nil)
			require.NoError(t, err)
		}

		authManagerMock := &auth.AuthManagerMock{}
		authManagerMock.On("GetUserByID", mock.Anything, user.ID).Return(user, nil).Maybe()

		return APIKeyHandler{Models: models, AuthManager: authManagerMock}, ctx, walletA, walletBID
	}

	create := func(t *testing.T, handler APIKeyHandler, ctx context.Context, body map[string]any) *httptest.ResponseRecorder {
		t.Helper()
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api-keys", bytes.NewReader(b))
		rr := httptest.NewRecorder()
		handler.CreateAPIKey(rr, req)
		return rr
	}

	developer := &auth.User{ID: developerUserID, Roles: []string{string(data.DeveloperUserRole)}}
	owner := &auth.User{ID: adminUserID, IsOwner: true}

	t.Run("a non-owner may scope a key to a wallet they hold", func(t *testing.T) {
		handler, ctx, walletA, _ := setup(t, developer)
		rr := create(t, handler, ctx, map[string]any{
			"name":                    "developer key",
			"permissions":             []string{"read:payments"},
			"distribution_wallet_ids": []string{walletA.ID},
		})
		require.Equal(t, http.StatusCreated, rr.Code)

		var got data.APIKey
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
		assert.Equal(t, []string{walletA.ID}, []string(got.DistributionWalletIDs))
	})

	t.Run("a non-owner may not scope a key beyond their own memberships", func(t *testing.T) {
		handler, ctx, _, walletBID := setup(t, developer)
		rr := create(t, handler, ctx, map[string]any{
			"name":                    "overreaching key",
			"permissions":             []string{"read:payments"},
			"distribution_wallet_ids": []string{walletBID},
		})
		assert.Equal(t, http.StatusForbidden, rr.Code)
		assert.NotContains(t, rr.Body.String(), "scope-wallet-b", "the refusal discloses no wallet detail")
	})

	t.Run("omitting the field inherits a non-owner's memberships, not the tenant", func(t *testing.T) {
		handler, ctx, walletA, walletBID := setup(t, developer)
		rr := create(t, handler, ctx, map[string]any{
			"name": "inheriting key", "permissions": []string{"read:payments"},
		})
		require.Equal(t, http.StatusCreated, rr.Code)

		var got data.APIKey
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
		assert.Equal(t, []string{walletA.ID}, []string(got.DistributionWalletIDs))
		assert.NotContains(t, got.DistributionWalletIDs, walletBID)
	})

	t.Run("omitting the field inherits every active wallet for an owner", func(t *testing.T) {
		handler, ctx, walletA, walletBID := setup(t, owner)
		rr := create(t, handler, ctx, map[string]any{
			"name": "owner key", "permissions": []string{"read:payments"},
		})
		require.Equal(t, http.StatusCreated, rr.Code)

		var got data.APIKey
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
		assert.ElementsMatch(t, []string{walletA.ID, walletBID}, []string(got.DistributionWalletIDs))
	})

	t.Run("an explicit empty list is honored as no wallet access", func(t *testing.T) {
		handler, ctx, _, _ := setup(t, owner)
		rr := create(t, handler, ctx, map[string]any{
			"name": "org-only key", "permissions": []string{"read:organization"},
			"distribution_wallet_ids": []string{},
		})
		require.Equal(t, http.StatusCreated, rr.Code)

		var got data.APIKey
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
		assert.Empty(t, got.DistributionWalletIDs)
	})

	t.Run("an archived account cannot be named on a new key", func(t *testing.T) {
		handler, ctx, _, walletBID := setup(t, owner)
		_, err := handler.Models.DBConnectionPool.ExecContext(ctx,
			`UPDATE distribution_wallets SET status = 'ARCHIVED', archived_at = NOW() WHERE id = $1`, walletBID)
		require.NoError(t, err)

		rr := create(t, handler, ctx, map[string]any{
			"name": "archived key", "permissions": []string{"read:payments"},
			"distribution_wallet_ids": []string{walletBID},
		})
		assert.Equal(t, http.StatusForbidden, rr.Code)
	})

	t.Run("inheriting on creation skips archived accounts", func(t *testing.T) {
		handler, ctx, walletA, walletBID := setup(t, owner)
		_, err := handler.Models.DBConnectionPool.ExecContext(ctx,
			`UPDATE distribution_wallets SET status = 'ARCHIVED', archived_at = NOW() WHERE id = $1`, walletBID)
		require.NoError(t, err)

		rr := create(t, handler, ctx, map[string]any{
			"name": "inheriting key", "permissions": []string{"read:payments"},
		})
		require.Equal(t, http.StatusCreated, rr.Code)

		var got data.APIKey
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
		assert.Equal(t, []string{walletA.ID}, []string(got.DistributionWalletIDs))
	})

	// An account archived after the key was scoped to it stays put: keeping it grants nothing new,
	// and refusing it would make an unrelated permissions edit impossible to save.
	t.Run("an edit keeps an account archived since the key was created", func(t *testing.T) {
		handler, ctx, walletA, walletBID := setup(t, owner)

		rr := create(t, handler, ctx, map[string]any{
			"name": "long-lived key", "permissions": []string{"read:payments"},
			"distribution_wallet_ids": []string{walletA.ID, walletBID},
		})
		require.Equal(t, http.StatusCreated, rr.Code)
		var created data.APIKey
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &created))

		_, err := handler.Models.DBConnectionPool.ExecContext(ctx,
			`UPDATE distribution_wallets SET status = 'ARCHIVED', archived_at = NOW() WHERE id = $1`, walletBID)
		require.NoError(t, err)

		patch := func(body map[string]any) *httptest.ResponseRecorder {
			b, marshalErr := json.Marshal(body)
			require.NoError(t, marshalErr)
			req := httptest.NewRequestWithContext(ctx, http.MethodPatch, "/api-keys/"+created.ID, bytes.NewReader(b))
			chiCtx := chi.NewRouteContext()
			chiCtx.URLParams.Add("id", created.ID)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, chiCtx))
			rr := httptest.NewRecorder()
			handler.UpdateKey(rr, req)
			return rr
		}

		resent := patch(map[string]any{
			"permissions":             []string{"read:payments", "read:exports"},
			"distribution_wallet_ids": []string{walletA.ID, walletBID},
		})
		require.Equal(t, http.StatusOK, resent.Code)
		var updated data.APIKey
		require.NoError(t, json.Unmarshal(resent.Body.Bytes(), &updated))
		assert.ElementsMatch(t, []string{walletA.ID, walletBID}, []string(updated.DistributionWalletIDs))

		omitted := patch(map[string]any{"permissions": []string{"read:payments"}})
		require.Equal(t, http.StatusOK, omitted.Code)
		require.NoError(t, json.Unmarshal(omitted.Body.Bytes(), &updated))
		assert.ElementsMatch(t, []string{walletA.ID, walletBID}, []string(updated.DistributionWalletIDs),
			"an edit that does not mention wallets leaves the scope alone")

		dropped := patch(map[string]any{
			"permissions":             []string{"read:payments"},
			"distribution_wallet_ids": []string{walletA.ID},
		})
		require.Equal(t, http.StatusOK, dropped.Code)
		require.NoError(t, json.Unmarshal(dropped.Body.Bytes(), &updated))
		assert.Equal(t, []string{walletA.ID}, []string(updated.DistributionWalletIDs),
			"removing an archived account is always allowed")

		readded := patch(map[string]any{
			"permissions":             []string{"read:payments"},
			"distribution_wallet_ids": []string{walletA.ID, walletBID},
		})
		assert.Equal(t, http.StatusForbidden, readded.Code,
			"once dropped, an archived account cannot be put back")
	})

	// An explicit [] on edit has to clear the scope. Omitting the field is what means "leave it
	// alone", and the two must not collapse into each other on the way to the DB.
	t.Run("an explicit empty list on edit clears the scope, omitting it does not", func(t *testing.T) {
		handler, ctx, walletA, _ := setup(t, owner)

		rr := create(t, handler, ctx, map[string]any{
			"name": "scope-clearing key", "permissions": []string{"read:payments"},
			"distribution_wallet_ids": []string{walletA.ID},
		})
		require.Equal(t, http.StatusCreated, rr.Code)
		var created data.APIKey
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &created))

		patch := func(body map[string]any) data.APIKey {
			b, marshalErr := json.Marshal(body)
			require.NoError(t, marshalErr)
			req := httptest.NewRequestWithContext(ctx, http.MethodPatch, "/api-keys/"+created.ID, bytes.NewReader(b))
			chiCtx := chi.NewRouteContext()
			chiCtx.URLParams.Add("id", created.ID)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, chiCtx))
			rr := httptest.NewRecorder()
			handler.UpdateKey(rr, req)
			require.Equal(t, http.StatusOK, rr.Code)

			var updated data.APIKey
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &updated))
			return updated
		}

		kept := patch(map[string]any{"permissions": []string{"read:payments", "read:exports"}})
		assert.Equal(t, []string{walletA.ID}, []string(kept.DistributionWalletIDs))

		cleared := patch(map[string]any{
			"permissions": []string{"read:payments"}, "distribution_wallet_ids": []string{},
		})
		assert.Empty(t, cleared.DistributionWalletIDs, "an explicit [] must reach the DB as empty, not nil")
	})

	t.Run("a key minting a key cannot widen the scope it holds", func(t *testing.T) {
		handler, ctx, walletA, walletBID := setup(t, developer)
		// The acting key names only walletA, so walletB is out of reach even though the request
		// arrives with write:all and the creator is resolvable.
		keyCtx := sdpcontext.SetAPIKeyInContext(ctx, &data.APIKey{
			ID:                    "acting-key",
			Permissions:           data.APIKeyPermissions{data.WriteAll},
			CreatedBy:             developerUserID,
			DistributionWalletIDs: pq.StringArray{walletA.ID},
		})

		rr := create(t, handler, keyCtx, map[string]any{
			"name": "child key", "permissions": []string{"read:payments"},
			"distribution_wallet_ids": []string{walletBID},
		})
		assert.Equal(t, http.StatusForbidden, rr.Code)

		rr = create(t, handler, keyCtx, map[string]any{
			"name": "sibling key", "permissions": []string{"read:payments"},
			"distribution_wallet_ids": []string{walletA.ID},
		})
		assert.Equal(t, http.StatusCreated, rr.Code)
	})
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
			nil,
			userID,
		)
		require.NoError(t, err)

		k2, err := handler.Models.APIKeys.Insert(ctx,
			"Cicatrix Maledictum Cipher",
			[]data.APIKeyPermission{data.ReadStatistics},
			[]string{"203.0.113.0/24"},
			nil,
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
			nil, nil, nil,
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
			nil, nil, nil,
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
			nil,
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
			nil, nil, nil,
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
