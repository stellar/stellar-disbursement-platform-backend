package store

import (
	"fmt"
	"testing"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_TransactionStatus_All(t *testing.T) {
	allStatuses := TransactionStatus("").All()
	require.Len(t, allStatuses, 4)
	require.Contains(t, allStatuses, TransactionStatusPending)
	require.Contains(t, allStatuses, TransactionStatusProcessing)
	require.Contains(t, allStatuses, TransactionStatusSuccess)
	require.Contains(t, allStatuses, TransactionStatusError)
}

func Test_TransactionStatus_Validate(t *testing.T) {
	testCases := []struct {
		name      string
		status    TransactionStatus
		wantError error
	}{
		{
			name:   "valid status (PENDING)",
			status: TransactionStatusPending,
		},
		{
			name:   "valid status (PROCESSING)",
			status: TransactionStatusProcessing,
		},
		{
			name:   "valid status (SUCCESS)",
			status: TransactionStatusSuccess,
		},
		{
			name:   "valid status (ERROR)",
			status: TransactionStatusError,
		},
		{
			name:      "invalid status (UNKNOWN)",
			status:    TransactionStatus("UNKNOWN"),
			wantError: fmt.Errorf("invalid disbursement status: UNKNOWN"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.status.Validate()
			if tc.wantError == nil {
				require.NoError(t, err)
			} else {
				require.Equal(t, tc.wantError, err)
			}
		})
	}
}

func Test_TransactionStatus_State(t *testing.T) {
	for _, status := range TransactionStatus("").All() {
		t.Run(string(status), func(t *testing.T) {
			require.Equal(t, data.State(status), status.State())
		})
	}
}

func Test_TransactionStatus_CanTransitionTo(t *testing.T) {
	type canTransitionTestCase struct {
		name          string
		from          TransactionStatus
		to            TransactionStatus
		canTransition bool
	}
	newCanTransitionTestCase := func(from TransactionStatus, to TransactionStatus, canTransition bool) canTransitionTestCase {
		namePrefix := "🛣️"
		if !canTransition {
			namePrefix = "🚧"
		}
		return canTransitionTestCase{
			name:          fmt.Sprintf("[%s]%s->%s", namePrefix, from, to),
			from:          from,
			to:            to,
			canTransition: canTransition,
		}
	}

	testCases := []struct {
		name          string
		from          TransactionStatus
		to            TransactionStatus
		canTransition bool
	}{
		// TransactionStatusPending -> ANY
		newCanTransitionTestCase(TransactionStatusPending, TransactionStatusPending, false),
		newCanTransitionTestCase(TransactionStatusPending, TransactionStatusProcessing, true),
		newCanTransitionTestCase(TransactionStatusPending, TransactionStatusSuccess, false),
		newCanTransitionTestCase(TransactionStatusPending, TransactionStatusError, false),
		// TransactionStatusProcessing -> ANY
		newCanTransitionTestCase(TransactionStatusProcessing, TransactionStatusPending, false),
		newCanTransitionTestCase(TransactionStatusProcessing, TransactionStatusProcessing, false),
		newCanTransitionTestCase(TransactionStatusProcessing, TransactionStatusSuccess, true),
		newCanTransitionTestCase(TransactionStatusProcessing, TransactionStatusError, true),
		// TransactionStatusSuccess -> ANY
		newCanTransitionTestCase(TransactionStatusSuccess, TransactionStatusPending, false),
		newCanTransitionTestCase(TransactionStatusSuccess, TransactionStatusProcessing, false),
		newCanTransitionTestCase(TransactionStatusSuccess, TransactionStatusSuccess, false),
		newCanTransitionTestCase(TransactionStatusSuccess, TransactionStatusError, false),
		// TransactionStatusError -> ANY
		newCanTransitionTestCase(TransactionStatusError, TransactionStatusPending, false),
		newCanTransitionTestCase(TransactionStatusError, TransactionStatusProcessing, false),
		newCanTransitionTestCase(TransactionStatusError, TransactionStatusSuccess, false),
		newCanTransitionTestCase(TransactionStatusError, TransactionStatusError, false),
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.from.CanTransitionTo(tc.to)
			if tc.canTransition {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.EqualError(t, err, fmt.Sprintf("cannot transition from %s to %s", tc.from, tc.to))
			}
		})
	}
}
