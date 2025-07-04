package data

import (
	"context"
	"fmt"
	"strings"
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
	pool := getConnectionPool(t)

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

func Test_APIKeyModel_GetAll_SortsByCreatedAtDesc(t *testing.T) {
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

func Test_APIKeyModel_GetByID(t *testing.T) {
	t.Parallel()

	pool := getConnectionPool(t)

	models, err := NewModels(pool)
	require.NoError(t, err)
	ctx := context.Background()

	creator := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	wrongCreator := "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"

	expiry := time.Now().Add(1 * time.Hour).UTC().Truncate(time.Second)
	fixture := createAPIKeyFixture(
		t, ctx, pool,
		"Forgefather’s Grimoire",
		[]APIKeyPermission{ReadStatistics},
		[]string{"192.0.2.0/24"},
		&expiry,
		creator,
	)

	cases := []struct {
		name      string
		id        string
		creatorID string
		wantErr   error
	}{
		{"success", fixture.ID, creator, nil},
		{"wrong_creator", fixture.ID, wrongCreator, ErrRecordNotFound},
		{"not_found", "00000000-0000-0000-0000-000000000000", creator, ErrRecordNotFound},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := models.APIKeys.GetByID(ctx, tc.id, tc.creatorID)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, fixture.ID, got.ID)
			assert.Equal(t, "Forgefather’s Grimoire", got.Name)
			assert.ElementsMatch(t, fixture.Permissions, got.Permissions)
			assert.Equal(t, IPList{"192.0.2.0/24"}, got.AllowedIPs)
			require.NotNil(t, got.ExpiryDate)
			assert.WithinDuration(t, expiry, *got.ExpiryDate, time.Second)
		})
	}
}

func Test_APIKeyModel_Delete(t *testing.T) {
	t.Parallel()

	pool := getConnectionPool(t)

	models, err := NewModels(pool)
	require.NoError(t, err)
	ctx := context.Background()

	creator := "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	other := "dddddddd-dddd-4ddd-8ddd-dddddddddddd"

	fixture := createAPIKeyFixture(
		t, ctx, pool,
		"Imperial Cogitator Key",
		[]APIKeyPermission{ReadAll},
		nil,
		nil,
		creator,
	)

	cases := []struct {
		name      string
		id        string
		creatorID string
		wantErr   error
	}{
		{"success", fixture.ID, creator, nil},
		{"not_found", "00000000-0000-0000-0000-000000000000", creator, ErrRecordNotFound},
		{"wrong_creator", fixture.ID, other, ErrRecordNotFound},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := models.APIKeys.Delete(ctx, tc.id, tc.creatorID)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func Test_APIKeyModel_Update(t *testing.T) {
	t.Parallel()

	pool := getConnectionPool(t)

	models, err := NewModels(pool)
	require.NoError(t, err)
	ctx := context.Background()

	creator := "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	otherUser := "dddddddd-dddd-4ddd-8ddd-dddddddddddd"

	initialPermissions := APIKeyPermissions{ReadAll}
	initialIPs := []string{"10.0.0.0/8"}

	fixture := createAPIKeyFixture(
		t, ctx, pool,
		"Magos Dominus Cipher",
		initialPermissions,
		initialIPs,
		nil,
		creator,
	)

	// New values for update
	newPermissions := APIKeyPermissions{ReadStatistics, ReadExports}
	newIPs := []string{"192.168.1.0/24", "203.0.113.42"}

	cases := []struct {
		name       string
		id         string
		creatorID  string
		perms      APIKeyPermissions
		ips        []string
		wantErr    error
		checkPerms APIKeyPermissions
		checkIPs   []string
	}{
		{
			name:       "success",
			id:         fixture.ID,
			creatorID:  creator,
			perms:      newPermissions,
			ips:        newIPs,
			wantErr:    nil,
			checkPerms: newPermissions,
			checkIPs:   newIPs,
		},
		{
			name:      "not_found",
			id:        "00000000-0000-0000-0000-000000000000",
			creatorID: creator,
			perms:     newPermissions,
			ips:       newIPs,
			wantErr:   ErrRecordNotFound,
		},
		{
			name:      "wrong_creator",
			id:        fixture.ID,
			creatorID: otherUser,
			perms:     newPermissions,
			ips:       newIPs,
			wantErr:   ErrRecordNotFound,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			updated, err := models.APIKeys.Update(ctx, tc.id, tc.creatorID, tc.perms, tc.ips)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.id, updated.ID)
			assert.ElementsMatch(t, tc.checkPerms, updated.Permissions)
			assert.ElementsMatch(t, tc.checkIPs, updated.AllowedIPs)

			// Verify other fields preserved
			assert.Equal(t, fixture.Name, updated.Name)
			assert.Equal(t, fixture.ExpiryDate, updated.ExpiryDate)
			assert.Equal(t, fixture.CreatedBy, updated.CreatedBy)

			// Verify timestamps - UpdatedAt should be more recent than CreatedAt
			assert.True(t, updated.UpdatedAt.After(updated.CreatedAt) ||
				updated.UpdatedAt.Equal(updated.CreatedAt))
		})
	}
}

func Test_APIKeyModel_ValidateRawKey(t *testing.T) {
	t.Parallel()

	pool := getConnectionPool(t)
	models, err := NewModels(pool)
	require.NoError(t, err)

	ctx := context.Background()

	creator := "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	otherUser := "dddddddd-dddd-4ddd-8ddd-dddddddddddd"

	expiryFut := time.Now().Add(1 * time.Hour)
	valid := createAPIKeyFixture(
		t, ctx, pool,
		"Ahriman", []APIKeyPermission{ReadStatistics},
		[]string{"127.0.0.1"}, &expiryFut, creator,
	)

	expiryPast := time.Now().Add(-1 * time.Hour)
	expired := createAPIKeyFixture(
		t, ctx, pool,
		"Vulkan", []APIKeyPermission{ReadStatistics},
		nil, &expiryPast, otherUser,
	)

	tests := []struct {
		name   string
		raw    string
		wantID string
		errMsg string
	}{
		{
			name:   "valid future key",
			raw:    valid.Key,
			wantID: valid.ID,
		},
		{
			name:   "invalid prefix",
			raw:    "CHS_" + strings.TrimPrefix(valid.Key, APIKeyPrefix),
			errMsg: "invalid API key prefix",
		},
		{
			name:   "invalid format",
			raw:    APIKeyPrefix + "." + strings.TrimPrefix(valid.Key, APIKeyPrefix),
			errMsg: "invalid API key format",
		},
		{
			name:   "non-existent ID",
			raw:    fmt.Sprintf("%s%s.%s", APIKeyPrefix, "00000000-0000-0000-0000-000000000000", "anykey"),
			errMsg: "no rows in result set",
		},
		{
			name:   "wrong secret",
			raw:    fmt.Sprintf("%s%s.%s", APIKeyPrefix, valid.ID, "WrongSecret"),
			errMsg: "invalid API key",
		},
		{
			name:   "expired key",
			raw:    expired.Key,
			errMsg: "API key expired",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := models.APIKeys.ValidateRawKey(ctx, tc.raw)
			if tc.errMsg != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errMsg)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantID, got.ID)
			assert.ElementsMatch(t, valid.Permissions, got.Permissions)
			assert.ElementsMatch(t, valid.AllowedIPs, got.AllowedIPs)
			assert.WithinDuration(t, *valid.ExpiryDate, *got.ExpiryDate, time.Second)
		})
	}
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
