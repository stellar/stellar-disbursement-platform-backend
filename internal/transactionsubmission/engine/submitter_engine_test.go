package engine

import (
	"testing"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_SubmitterEngine_Validate(t *testing.T) {
	hMock := &horizonclient.MockClient{}
	mLedgerNumberTracker := mocks.NewMockLedgerNumberTracker(t)
	mSigService := mocks.NewMockSignatureService(t)

	testCases := []struct {
		name            string
		engine          SubmitterEngine
		wantErrContains string
	}{
		{
			name:            "returns an error if the horizon client is nil",
			wantErrContains: "horizon client cannot be nil",
		},
		{
			name: "returns an error if the ledger number tracker is nil",
			engine: SubmitterEngine{
				HorizonClient: hMock,
			},
			wantErrContains: "ledger number tracker cannot be nil",
		},
		{
			name: "returns an error if the signature service is nil",
			engine: SubmitterEngine{
				HorizonClient:       hMock,
				LedgerNumberTracker: mLedgerNumberTracker,
			},
			wantErrContains: "signature service cannot be nil",
		},
		{
			name: "returns an error if the max base fee is less than the minimum",
			engine: SubmitterEngine{
				HorizonClient:       hMock,
				LedgerNumberTracker: mLedgerNumberTracker,
				SignatureService:    mSigService,
				MaxBaseFee:          99,
			},
			wantErrContains: "maxBaseFee must be greater than or equal to 100",
		},
		{
			name: "🎉 successfully validates the SubmitterEngine",
			engine: SubmitterEngine{
				HorizonClient:       hMock,
				LedgerNumberTracker: mLedgerNumberTracker,
				SignatureService:    mSigService,
				MaxBaseFee:          100,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.engine.Validate()
			if tc.wantErrContains != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErrContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
