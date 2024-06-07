package signing

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

func Test_SignatureClientType_AccountType(t *testing.T) {
	testCases := []struct {
		distSigClientType           DistributionSignatureClientType
		wantErrContains             string
		wantDistributionAccountType schema.AccountType
	}{
		{
			distSigClientType: DistributionSignatureClientType("INVALID"),
			wantErrContains:   `invalid distribution account type "INVALID"`,
		},
		{
			distSigClientType:           DistributionAccountEnvSignatureClientType,
			wantDistributionAccountType: schema.DistributionAccountStellarEnv,
		},
		{
			distSigClientType:           DistributionAccountDBSignatureClientType,
			wantDistributionAccountType: schema.DistributionAccountStellarDBVault,
		},
	}

	for _, tc := range testCases {
		t.Run(string(tc.distSigClientType), func(t *testing.T) {
			distAccType, err := tc.distSigClientType.AccountType()
			if tc.wantErrContains != "" {
				require.ErrorContains(t, err, tc.wantErrContains)
				assert.Empty(t, distAccType)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.wantDistributionAccountType, distAccType)
			}
		})
	}
}

func Test_ParseDistributionSignatureClientType(t *testing.T) {
	testCases := []struct {
		sigServiceTypeStr         string
		expectedDistSigClientType DistributionSignatureClientType
		wantErr                   error
	}{
		{wantErr: fmt.Errorf(`invalid distribution signature client type ""`)},
		{sigServiceTypeStr: "INVALID", wantErr: fmt.Errorf(`invalid distribution signature client type "INVALID"`)},
		{sigServiceTypeStr: "DISTRIBUTION_ACCOUNT_DB", expectedDistSigClientType: DistributionAccountDBSignatureClientType},
		{sigServiceTypeStr: "DISTRIBUTION_ACCOUNT_ENV", expectedDistSigClientType: DistributionAccountEnvSignatureClientType},
	}

	for _, tc := range testCases {
		t.Run("signatureServiceTypeType: "+tc.sigServiceTypeStr, func(t *testing.T) {
			distSigClientType, err := ParseDistributionSignatureClientType(tc.sigServiceTypeStr)
			assert.Equal(t, tc.expectedDistSigClientType, distSigClientType)
			assert.Equal(t, tc.wantErr, err)
		})
	}
}
