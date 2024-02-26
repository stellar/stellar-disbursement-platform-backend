package signing

import (
	"testing"

	"github.com/stellar/go/keypair"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_DistributionAccountResolverConfig_Validate(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	testCases := []struct {
		name            string
		config          DistributionAccountResolverConfig
		wantErrContains string
	}{
		{
			name: "return an error if AdminDBConnectionPool is nil",
			config: DistributionAccountResolverConfig{
				AdminDBConnectionPool: nil,
			},
			wantErrContains: "AdminDBConnectionPool cannot be nil",
		},
		{
			name: "return an error if HostDistributionAccountPublicKey is empty",
			config: DistributionAccountResolverConfig{
				AdminDBConnectionPool:            dbConnectionPool,
				HostDistributionAccountPublicKey: "",
			},
			wantErrContains: "HostDistributionAccountPublicKey cannot be empty",
		},
		{
			name: "return an error if HostDistributionAccountPublicKey is not a valid ed25519 public key",
			config: DistributionAccountResolverConfig{
				AdminDBConnectionPool:            dbConnectionPool,
				HostDistributionAccountPublicKey: "not-a-valid-ed25519-public-key",
			},
			wantErrContains: "HostDistributionAccountPublicKey is not a valid ed25519 public key",
		},
		{
			name: "🎉 successfully validate the config",
			config: DistributionAccountResolverConfig{
				AdminDBConnectionPool:            dbConnectionPool,
				HostDistributionAccountPublicKey: keypair.MustRandom().Address(),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.config.Validate()
			if tc.wantErrContains == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.wantErrContains)
			}
		})
	}
}

func Test_NewDistributionAccountResolver(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	hostDistPublicKey := keypair.MustRandom().Address()

	testCases := []struct {
		name            string
		config          DistributionAccountResolverConfig
		wantErrContains string
		wantResult      DistributionAccountResolver
	}{
		{
			name: "return an error if config is invalid",
			config: DistributionAccountResolverConfig{
				AdminDBConnectionPool:            nil,
				HostDistributionAccountPublicKey: "",
			},
			wantErrContains: "validating config in NewDistributionAccountResolver: AdminDBConnectionPool cannot be nil",
		},
		{
			name: "🎉 successfully create a new DistributionAccountResolver",
			config: DistributionAccountResolverConfig{
				AdminDBConnectionPool:            dbConnectionPool,
				HostDistributionAccountPublicKey: hostDistPublicKey,
			},
			wantResult: &DistributionAccountResolverImpl{
				dbConnectionPool:              dbConnectionPool,
				hostDistributionAccountPubKey: hostDistPublicKey,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotResult, err := NewDistributionAccountResolver(tc.config)
			if tc.wantErrContains == "" {
				assert.NoError(t, err)
				assert.Equal(t, tc.wantResult, gotResult)
			} else {
				assert.ErrorContains(t, err, tc.wantErrContains)
				assert.Empty(t, tc.wantResult)
			}
		})
	}
}

func Test_DistributionAccountResolverImpl_DistributionAccount(t *testing.T) {
	t.Skip("TODO")
}

func Test_DistributionAccountResolverImpl_HostDistributionAccount(t *testing.T) {
	publicKeys := []string{
		keypair.MustRandom().Address(),
		keypair.MustRandom().Address(),
		keypair.MustRandom().Address(),
	}

	for i, publicKey := range publicKeys {
		distAccResolver := &DistributionAccountResolverImpl{hostDistributionAccountPubKey: publicKey}
		assert.Equalf(t, publicKey, distAccResolver.HostDistributionAccount(), "assertion failed at index %d", i)
	}
}
