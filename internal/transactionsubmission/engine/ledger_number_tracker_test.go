package engine

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/protocols/horizon"
	"github.com/stellar/go/support/render/problem"
	"github.com/stellar/go/txnbuild"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewLedgerNumberTracker(t *testing.T) {
	mockHorizonClient := &horizonclient.MockClient{}

	testCases := []struct {
		name            string
		hClient         horizonclient.ClientInterface
		wantErrContains string
		wantResult      LedgerNumberTracker
	}{
		{
			name:            "returns an error if the horizon client is nil",
			hClient:         nil,
			wantErrContains: "horizon client cannot be nil",
		},
		{
			name:    "🎉 successfully provides new LedgerNumberTracker",
			hClient: mockHorizonClient,
			wantResult: &DefaultLedgerNumberTracker{
				hClient:      mockHorizonClient,
				maxLedgerAge: MaxLedgerAge,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ledgerNumberTracker, err := NewLedgerNumberTracker(tc.hClient)
			if tc.wantErrContains != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErrContains)
				assert.Nil(t, ledgerNumberTracker)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, ledgerNumberTracker)
				assert.Equal(t, tc.wantResult, ledgerNumberTracker)
			}
		})
	}

	mockHorizonClient.AssertExpectations(t)
}

func Test_LedgerNumberTracker_getLedgerNumberFromHorizon(t *testing.T) {
	testCases := []struct {
		name                        string
		horizonResponseError        error
		wantErrContains             string
		horizonResponseRoot         horizon.Root
		horizonResponseLedgerNumber int
		wantResult                  int
	}{
		{
			name: "returns an error if horizon returns a horizon error",
			horizonResponseError: horizonclient.Error{
				Problem: problem.P{
					Title:  "Foo",
					Type:   "bar",
					Status: http.StatusTooManyRequests,
				},
			},
			wantErrContains: "horizon response error: StatusCode=429, Type=bar, Title=Foo",
		},
		{
			name:                 "returns an error if horizon returns an unexpected error",
			horizonResponseError: fmt.Errorf("some random error"),
			wantErrContains:      "horizon response error: some random error",
		},
		{
			name:                "🎉 successfully gets the latest ledger number",
			horizonResponseRoot: horizon.Root{HorizonSequence: 1234},
			wantResult:          1234,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockHorizonClient := &horizonclient.MockClient{}
			mockHorizonClient.On("Root").Return(tc.horizonResponseRoot, tc.horizonResponseError).Once()

			ledgerNumberTracker, err := NewLedgerNumberTracker(mockHorizonClient)
			require.NoError(t, err)

			ledgerNumber, err := ledgerNumberTracker.getLedgerNumberFromHorizon()
			if tc.horizonResponseError != nil {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErrContains)
				assert.Equal(t, 0, ledgerNumber)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.wantResult, ledgerNumber)
			}

			mockHorizonClient.AssertExpectations(t)
		})
	}
}

func Test_LedgerNumberTracker_GetLedgerNumber(t *testing.T) {
	const startingLedgerNumber = 1230

	testCases := []struct {
		name                        string
		startingLedgerNumber        int
		isNumberExpired             bool
		horizonResponseRoot         horizon.Root
		horizonResponseError        error
		wantErrContains             string
		horizonResponseLedgerNumber int
		wantResult                  int
	}{
		{
			name:                 "returns an error if horizon returns a horizon error (EXPIRED)",
			startingLedgerNumber: startingLedgerNumber,
			isNumberExpired:      true,
			horizonResponseError: horizonclient.Error{
				Problem: problem.P{
					Title:  "Foo",
					Type:   "bar",
					Status: http.StatusTooManyRequests,
				},
			},
			wantErrContains: "getting ledger number from horizon: horizon response error: StatusCode=429, Type=bar, Title=Foo",
		},
		{
			name:                 "returns an error if horizon returns an unexpected error (EXPIRED)",
			startingLedgerNumber: startingLedgerNumber,
			isNumberExpired:      true,
			horizonResponseError: fmt.Errorf("some random error"),
			wantErrContains:      "getting ledger number from horizon: horizon response error: some random error",
		},
		{
			name:                 "🎉 successfully gets the latest ledger number (EXPIRED)",
			startingLedgerNumber: startingLedgerNumber,
			isNumberExpired:      true,
			horizonResponseRoot:  horizon.Root{HorizonSequence: 1234},
			wantResult:           1234,
		},
		{
			name:                 "🎉 successfully gets the latest ledger number (NOT EXPIRED)",
			startingLedgerNumber: startingLedgerNumber,
			isNumberExpired:      false,
			wantResult:           startingLedgerNumber,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockHorizonClient := &horizonclient.MockClient{}
			if tc.isNumberExpired {
				mockHorizonClient.On("Root").Return(tc.horizonResponseRoot, tc.horizonResponseError).Once()
			}

			ledgerNumberTracker, err := NewLedgerNumberTracker(mockHorizonClient)
			require.NoError(t, err)
			ledgerNumberTracker.ledgerNumber = tc.startingLedgerNumber
			if tc.isNumberExpired {
				ledgerNumberTracker.lastUpdatedAt = time.Now().Add(-ledgerNumberTracker.maxLedgerAge - time.Second)
			} else {
				ledgerNumberTracker.lastUpdatedAt = time.Now().Add(-ledgerNumberTracker.maxLedgerAge + time.Second)
			}
			initialLedgerLastUpdatedAt := ledgerNumberTracker.lastUpdatedAt

			ledgerNumber, err := ledgerNumberTracker.GetLedgerNumber()
			if tc.horizonResponseError != nil {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErrContains)
				assert.Equal(t, 0, ledgerNumber)
				assert.Equal(t, initialLedgerLastUpdatedAt, ledgerNumberTracker.lastUpdatedAt)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.wantResult, ledgerNumber)
				if tc.isNumberExpired {
					assert.NotEqual(t, initialLedgerLastUpdatedAt, ledgerNumberTracker.lastUpdatedAt)
				} else {
					assert.Equal(t, initialLedgerLastUpdatedAt, ledgerNumberTracker.lastUpdatedAt)
				}
			}

			mockHorizonClient.AssertExpectations(t)
		})
	}
}

func Test_LedgerNumberTracker_GetLedgerBounds(t *testing.T) {
	const startingLedgerNumber = 1230

	testCases := []struct {
		name                 string
		startingLedgerNumber int
		isNumberExpired      bool
		horizonResponseRoot  horizon.Root
		horizonResponseError error
		wantErrContains      string
		wantResult           *txnbuild.LedgerBounds
	}{
		{
			name:                 "returns an error if horizon returns a horizon error (EXPIRED)",
			startingLedgerNumber: startingLedgerNumber,
			isNumberExpired:      true,
			horizonResponseError: horizonclient.Error{
				Problem: problem.P{
					Title:  "Foo",
					Type:   "bar",
					Status: http.StatusTooManyRequests,
				},
			},
			wantErrContains: "getting ledger number: getting ledger number from horizon: horizon response error: StatusCode=429, Type=bar, Title=Foo",
		},
		{
			name:                 "🎉 successfully gets the latest ledger number (EXPIRED)",
			startingLedgerNumber: startingLedgerNumber,
			isNumberExpired:      true,
			horizonResponseRoot:  horizon.Root{HorizonSequence: 1234},
			wantResult:           &txnbuild.LedgerBounds{MaxLedger: 1234 + IncrementForMaxLedgerBounds},
		},
		{
			name:                 "🎉 successfully gets the latest ledger number (NOT EXPIRED)",
			startingLedgerNumber: startingLedgerNumber,
			isNumberExpired:      false,
			wantResult:           &txnbuild.LedgerBounds{MaxLedger: startingLedgerNumber + IncrementForMaxLedgerBounds},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockHorizonClient := &horizonclient.MockClient{}
			if tc.isNumberExpired {
				mockHorizonClient.On("Root").Return(tc.horizonResponseRoot, tc.horizonResponseError).Once()
			}

			ledgerNumberTracker, err := NewLedgerNumberTracker(mockHorizonClient)
			require.NoError(t, err)
			ledgerNumberTracker.ledgerNumber = tc.startingLedgerNumber
			if tc.isNumberExpired {
				ledgerNumberTracker.lastUpdatedAt = time.Now().Add(-ledgerNumberTracker.maxLedgerAge - time.Second)
			} else {
				ledgerNumberTracker.lastUpdatedAt = time.Now().Add(-ledgerNumberTracker.maxLedgerAge + time.Second)
			}

			ledgerBounds, err := ledgerNumberTracker.GetLedgerBounds()
			if tc.horizonResponseError != nil {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErrContains)
				assert.Nil(t, ledgerBounds)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.wantResult, ledgerBounds)
			}

			mockHorizonClient.AssertExpectations(t)
		})
	}
}
