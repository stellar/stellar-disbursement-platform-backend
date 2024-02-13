package engine

import (
	"testing"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	preconditionsMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/preconditions/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
)

func Test_SubmitterEngine_Validate(t *testing.T) {
	hMock := &horizonclient.MockClient{}
	mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)
	sigService, _, mDistAccSigClient, _, _ := signing.NewMockSignatureService(t)

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
			wantErrContains: "signature service cannot be empty",
		},
		{
			name: "returns an error if the max base fee is less than the minimum",
			engine: SubmitterEngine{
				HorizonClient:       hMock,
				LedgerNumberTracker: mLedgerNumberTracker,
				SignatureService: signing.SignatureService{
					DistAccountSigner: mDistAccSigClient,
				},
				MaxBaseFee: 99,
			},
			wantErrContains: "validating signature service: channel account signer cannot be nil",
		},
		{
			name: "returns an error if the max base fee is less than the minimum",
			engine: SubmitterEngine{
				HorizonClient:       hMock,
				LedgerNumberTracker: mLedgerNumberTracker,
				SignatureService:    sigService,
				MaxBaseFee:          99,
			},
			wantErrContains: "maxBaseFee must be greater than or equal to 100",
		},
		{
			name: "🎉 successfully validates the SubmitterEngine",
			engine: SubmitterEngine{
				HorizonClient:       hMock,
				LedgerNumberTracker: mLedgerNumberTracker,
				SignatureService:    sigService,
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
