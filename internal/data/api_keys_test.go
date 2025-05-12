package data

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
)

func Test_APIKey_HasPermission(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		perms []APIKeyPermission
		check APIKeyPermission
		want  bool
	}{
		{"has specific", []APIKeyPermission{ReadStatistics, ReadExports}, ReadStatistics, true},
		{"missing specific", []APIKeyPermission{ReadStatistics, ReadExports}, ReadPayments, false},
		{"read:all grants read", []APIKeyPermission{ReadAll}, ReadStatistics, true},
		{"write:all grants write", []APIKeyPermission{WriteAll}, WritePayments, true},
		{"read:all not grant write", []APIKeyPermission{ReadAll}, WritePayments, false},
		{"none", []APIKeyPermission{}, ReadStatistics, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			key := APIKey{Permissions: APIKeyPermissions(tc.perms)}
			assert.Equal(t, tc.want, key.HasPermission(tc.check))
		})
	}
}

func Test_APIKey_IsExpired(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	future := now.Add(24 * time.Hour)
	past := now.Add(-24 * time.Hour)

	cases := []struct {
		name string
		key  APIKey
		want bool
	}{
		{"no expiry", APIKey{ExpiryDate: nil}, false},
		{"future", APIKey{ExpiryDate: &future}, false},
		{"past", APIKey{ExpiryDate: &past}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.key.IsExpired())
		})
	}
}

func Test_APIKey_IsAllowedIP(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		key  APIKey
		ip   string
		want bool
	}{
		{"open", APIKey{AllowedIPs: IPList{}}, "1.2.3.4", true},
		{"direct", APIKey{AllowedIPs: IPList{"1.2.3.4"}}, "1.2.3.4", true},
		{"miss", APIKey{AllowedIPs: IPList{"1.2.3.4"}}, "1.2.3.5", false},
		{"cidr ok", APIKey{AllowedIPs: IPList{"10.0.0.0/8"}}, "10.1.2.3", true},
		{"cidr x", APIKey{AllowedIPs: IPList{"10.0.0.0/8"}}, "11.0.0.1", false},
		{"bad ip", APIKey{AllowedIPs: IPList{"1.2.3.4"}}, "nope", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.key.IsAllowedIP(tc.ip))
		})
	}
}

func Test_ValidatePermissions(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		perms []APIKeyPermission
		err   bool
	}{
		{"all good", []APIKeyPermission{ReadStatistics, ReadExports}, false},
		{"one bad", []APIKeyPermission{ReadStatistics, "bad"}, true},
		{"empty", []APIKeyPermission{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.err {
				require.Error(t, ValidatePermissions(tc.perms))
			} else {
				require.NoError(t, ValidatePermissions(tc.perms))
			}
		})
	}
}

func Test_ValidateAllowedIPs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		ips  []string
		err  bool
	}{
		{"valid IPs", []string{"1.2.3.4", "5.6.7.8"}, false},
		{"valid CIDR", []string{"192.168.0.0/16"}, false},
		{"mixed valid", []string{"1.2.3.4", "10.0.0.0/8"}, false},
		{"bad IP", []string{"1.2.3.4", "nope"}, true},
		{"bad CIDR", []string{"1.2.3.0/33"}, true},
		{"empty", []string{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.err {
				require.Error(t, ValidateAllowedIPs(tc.ips))
			} else {
				require.NoError(t, ValidateAllowedIPs(tc.ips))
			}
		})
	}
}

func Test_generateSecret(t *testing.T) {
	t.Parallel()
	sec, err := generateSecret()
	require.NoError(t, err)
	assert.Len(t, sec, APIKeySecretSize)
	for _, ch := range sec {
		assert.Contains(t, alphabet, string(ch))
	}
}

func Test_APIKeyModel_Insert(t *testing.T) {
	dbt := dbtest.Open(t)
	t.Cleanup(func() { dbt.Close() })

	pool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })

	ctx := context.Background()
	models, err := NewModels(pool)
	require.NoError(t, err)

	t.Run("insert key", func(t *testing.T) {
		perms := APIKeyPermissions{ReadStatistics, ReadExports}
		ips := IPList{"1.2.3.4", "10.0.0.0/8"}

		key := createAPIKeyFixture(
			t, ctx, pool,
			"Relic of the Omnissiah",
			perms,
			ips,
			nil,
			"00000000-0000-0000-0000-000000000000",
		)

		assert.NotEmpty(t, key.ID)
		assert.Equal(t, "Relic of the Omnissiah", key.Name)
		assert.NotEmpty(t, key.KeyHash)
		assert.NotEmpty(t, key.Salt)
		assert.Nil(t, key.ExpiryDate)
		assert.Equal(t, perms, key.Permissions)
		assert.Equal(t, ips, key.AllowedIPs)
		assert.Equal(t, "00000000-0000-0000-0000-000000000000", key.CreatedBy)
		assert.Equal(t, "00000000-0000-0000-0000-000000000000", key.UpdatedBy)
		assert.NotEmpty(t, key.Key)
		assert.WithinDuration(t, time.Now().UTC(), key.CreatedAt, time.Second*5)
		assert.WithinDuration(t, time.Now().UTC(), key.UpdatedAt, time.Second*5)
		assert.Nil(t, key.LastUsedAt)
	})

	t.Run("insert new key with minimum fields", func(t *testing.T) {
		expiry := time.Now().Add(48 * time.Hour)
		perms := []APIKeyPermission{ReadAll}
		ips := []string{}
		name := "Stygies VIII Archive Key"
		createdBy := "00000000-0000-0000-0000-000000000000"

		key, err := models.APIKeys.Insert(ctx, name, perms, ips, &expiry, createdBy)

		require.NoError(t, err)

		assert.Equal(t, name, key.Name)
		require.NotNil(t, key.ExpiryDate)
		assert.WithinDuration(t, expiry, *key.ExpiryDate, time.Second)
	})
}

func TestAPIKeyModel_GetAll_SortsByCreatedAtDesc(t *testing.T) {
	t.Parallel()

	pool := getConnectionPool(t)

	models, err := NewModels(pool)
	require.NoError(t, err)
	ctx := context.Background()
	creator := "fe302e77-ec3f-4a3b-9f8a-1234567890ab"

	k1 := createAPIKeyFixture(
		t, ctx, pool,
		"Black Crusade Vault Key",
		[]APIKeyPermission{ReadExports},
		nil, // no IP restrictions
		nil,
		creator,
	)

	k2 := createAPIKeyFixture(
		t, ctx, pool,
		"Cadian Token",
		[]APIKeyPermission{ReadStatistics},
		[]string{"10.0.0.0/8"},
		nil,
		creator,
	)

	k3 := createAPIKeyFixture(
		t, ctx, pool,
		"Sigil",
		[]APIKeyPermission{WriteAll},
		nil,
		nil,
		creator,
	)

	keys, err := models.APIKeys.GetAll(ctx, creator)
	require.NoError(t, err)

	require.Len(t, keys, 3)
	assert.Equal(t, "Sigil", k3.Name)
	assert.Equal(t, "Cadian Token", k2.Name)
	assert.Equal(t, "Black Crusade Vault Key", k1.Name)
}

func getConnectionPool(t *testing.T) db.DBConnectionPool {
	dbt := dbtest.Open(t)
	t.Cleanup(func() { dbt.Close() })

	pool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })
	return pool
}

func createAPIKeyFixture(t *testing.T, ctx context.Context, pool db.DBConnectionPool, name string, perms []APIKeyPermission, ips []string, expiry *time.Time, createdBy string) *APIKey {
	t.Helper()
	models, err := NewModels(pool)
	require.NoError(t, err)

	key, err := models.APIKeys.Insert(ctx,
		name,
		perms,
		ips,
		expiry,
		createdBy,
	)
	require.NoError(t, err)
	return key
}
