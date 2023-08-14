package engine

import (
	"testing"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewSubmitterEngine(t *testing.T) {
	mockHorizonClient := &horizonclient.MockClient{}

	testCases := []struct {
		name            string
		hClient         horizonclient.ClientInterface
		wantErrContains string
		wantResult      *SubmitterEngine
	}{
		{
			name:            "returns an error if the horizon client is nil",
			hClient:         nil,
			wantErrContains: "creating ledger keeper: horizon client cannot be nil",
		},
		{
			name:    "🎉 successfully provides new SubmitterEngine",
			hClient: mockHorizonClient,
			wantResult: &SubmitterEngine{
				HorizonClient:       mockHorizonClient,
				LedgerNumberTracker: &DefaultLedgerNumberTracker{hClient: mockHorizonClient, maxLedgerAge: MaxLedgerAge},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			submitterEngine, err := NewSubmitterEngine(tc.hClient)
			if tc.wantErrContains != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErrContains)
				assert.Nil(t, submitterEngine)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, submitterEngine)
				assert.Equal(t, tc.wantResult, submitterEngine)
			}
		})
	}

	mockHorizonClient.AssertExpectations(t)
}
