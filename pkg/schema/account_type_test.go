package schema

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_AccountType_IsStellar(t *testing.T) {
	testCases := []struct {
		accountType AccountType
		isStellar   bool
	}{
		{accountType: HostStellarEnv, isStellar: true},
		{accountType: ChannelAccountStellarDB, isStellar: true},
		{accountType: DistributionAccountStellarEnv, isStellar: true},
		{accountType: DistributionAccountStellarDBVault, isStellar: true},
		{accountType: DistributionAccountCircleDBVault, isStellar: false},
	}
	for _, tc := range testCases {
		t.Run(string(tc.accountType), func(t *testing.T) {
			if tc.isStellar {
				assert.True(t, tc.accountType.IsStellar())
			} else {
				assert.False(t, tc.accountType.IsStellar())
			}
		})
	}
}

func Test_AccountType_IsCircle(t *testing.T) {
	testCases := []struct {
		accountType AccountType
		isCircle    bool
	}{
		{accountType: HostStellarEnv, isCircle: false},
		{accountType: ChannelAccountStellarDB, isCircle: false},
		{accountType: DistributionAccountStellarEnv, isCircle: false},
		{accountType: DistributionAccountStellarDBVault, isCircle: false},
		{accountType: DistributionAccountCircleDBVault, isCircle: true},
	}
	for _, tc := range testCases {
		t.Run(string(tc.accountType), func(t *testing.T) {
			if tc.isCircle {
				assert.True(t, tc.accountType.IsCircle())
			} else {
				assert.False(t, tc.accountType.IsCircle())
			}
		})
	}
}

func Test_AccountType_Role(t *testing.T) {
	testCases := []struct {
		accountType AccountType
		wantRole    Role
	}{
		{accountType: HostStellarEnv, wantRole: HostRole},
		{accountType: ChannelAccountStellarDB, wantRole: ChannelAccountRole},
		{accountType: DistributionAccountStellarEnv, wantRole: DistributionAccountRole},
		{accountType: DistributionAccountStellarDBVault, wantRole: DistributionAccountRole},
		{accountType: DistributionAccountCircleDBVault, wantRole: DistributionAccountRole},
	}
	for _, tc := range testCases {
		t.Run(string(tc.accountType), func(t *testing.T) {
			assert.Equal(t, tc.wantRole, tc.accountType.Role())

			// Ensure the order of the qualifiers in the string is correct:
			qualifiers := strings.Split(string(tc.accountType), ".")
			assert.Len(t, qualifiers, 3)
			firstQualifier := qualifiers[0]
			assert.Equal(t, string(tc.wantRole), firstQualifier)
		})
	}
}

func Test_AccountType_Platform(t *testing.T) {
	testCases := []struct {
		accountType  AccountType
		wantPlatform Platform
	}{
		{accountType: HostStellarEnv, wantPlatform: StellarPlatform},
		{accountType: ChannelAccountStellarDB, wantPlatform: StellarPlatform},
		{accountType: DistributionAccountStellarEnv, wantPlatform: StellarPlatform},
		{accountType: DistributionAccountStellarDBVault, wantPlatform: StellarPlatform},
		{accountType: DistributionAccountCircleDBVault, wantPlatform: CirclePlatform},
	}
	for _, tc := range testCases {
		t.Run(string(tc.accountType), func(t *testing.T) {
			assert.Equal(t, tc.wantPlatform, tc.accountType.Platform())

			// Ensure the order of the qualifiers in the string is correct:
			qualifiers := strings.Split(string(tc.accountType), ".")
			assert.Len(t, qualifiers, 3)
			secondQualifier := qualifiers[1]
			assert.Equal(t, string(tc.wantPlatform), secondQualifier)
		})
	}
}

func Test_AccountType_StorageMethod(t *testing.T) {
	testCases := []struct {
		accountType       AccountType
		wantStorageMethod StorageMethod
	}{
		{accountType: HostStellarEnv, wantStorageMethod: EnvStorageMethod},
		{accountType: ChannelAccountStellarDB, wantStorageMethod: DBStorageMethod},
		{accountType: DistributionAccountStellarEnv, wantStorageMethod: EnvStorageMethod},
		{accountType: DistributionAccountStellarDBVault, wantStorageMethod: DBVaultStorageMethod},
		{accountType: DistributionAccountCircleDBVault, wantStorageMethod: DBVaultStorageMethod},
	}
	for _, tc := range testCases {
		t.Run(string(tc.accountType), func(t *testing.T) {
			assert.Equal(t, tc.wantStorageMethod, tc.accountType.StorageMethod())

			// Ensure the order of the qualifiers in the string is correct:
			qualifiers := strings.Split(string(tc.accountType), ".")
			assert.Len(t, qualifiers, 3)
			thirdQualifier := qualifiers[2]
			assert.Equal(t, string(tc.wantStorageMethod), thirdQualifier)
		})
	}
}
