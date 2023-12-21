package data

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_PaymentStatus_SourceStatuses(t *testing.T) {
	tests := []struct {
		name                   string
		targetStatus           PaymentStatus
		expectedSourceStatuses []PaymentStatus
	}{
		{
			name:                   "Draft",
			targetStatus:           DraftPaymentStatus,
			expectedSourceStatuses: []PaymentStatus{},
		},
		{
			name:                   "Ready",
			targetStatus:           ReadyPaymentStatus,
			expectedSourceStatuses: []PaymentStatus{DraftPaymentStatus, PausedPaymentStatus},
		},
		{
			name:                   "Pending",
			targetStatus:           PendingPaymentStatus,
			expectedSourceStatuses: []PaymentStatus{ReadyPaymentStatus, FailedPaymentStatus},
		},
		{
			name:                   "Paused",
			targetStatus:           PausedPaymentStatus,
			expectedSourceStatuses: []PaymentStatus{ReadyPaymentStatus},
		},
		{
			name:                   "Success",
			targetStatus:           SuccessPaymentStatus,
			expectedSourceStatuses: []PaymentStatus{PendingPaymentStatus},
		},
		{
			name:                   "Failure",
			targetStatus:           FailedPaymentStatus,
			expectedSourceStatuses: []PaymentStatus{PendingPaymentStatus},
		},
		{
			name:                   "Canceled",
			targetStatus:           CanceledPaymentStatus,
			expectedSourceStatuses: []PaymentStatus{ReadyPaymentStatus},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expectedSourceStatuses, tt.targetStatus.SourceStatuses())
		})
	}
}

func Test_PaymentStatus_PaymentStatuses(t *testing.T) {
	expectedStatuses := []PaymentStatus{DraftPaymentStatus, ReadyPaymentStatus, PendingPaymentStatus, PausedPaymentStatus, SuccessPaymentStatus, FailedPaymentStatus, CanceledPaymentStatus}
	require.Equal(t, expectedStatuses, PaymentStatuses())
}

func Test_PaymentStatus_ToPaymentStatus(t *testing.T) {
	tests := []struct {
		name   string
		actual string
		want   PaymentStatus
		err    error
	}{
		{
			name:   "valid entry",
			actual: "CANCELED",
			want:   CanceledPaymentStatus,
			err:    nil,
		},
		{
			name:   "valid lower case",
			actual: "canceled",
			want:   CanceledPaymentStatus,
			err:    nil,
		},
		{
			name:   "valid weird case",
			actual: "CancEled",
			want:   CanceledPaymentStatus,
			err:    nil,
		},
		{
			name:   "invalid entry",
			actual: "NOT_VALID",
			want:   CanceledPaymentStatus,
			err:    fmt.Errorf("invalid payment status: NOT_VALID"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ToPaymentStatus(tt.actual)

			if tt.err != nil {
				require.EqualError(t, err, tt.err.Error())
				return
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_PaymentStatus_TransitionTo(t *testing.T) {
	tests := []struct {
		name   string
		actual PaymentStatus
		target PaymentStatus
		err    error
	}{
		{
			name:   "disbursement started transition - success",
			actual: DraftPaymentStatus,
			target: ReadyPaymentStatus,
			err:    nil,
		},
		{
			name:   "disbursement started transition - success",
			actual: DraftPaymentStatus,
			target: ReadyPaymentStatus,
			err:    nil,
		},
		{
			name:   "payment gets submitted if user is ready",
			actual: ReadyPaymentStatus,
			target: PendingPaymentStatus,
			err:    nil,
		},
		{
			name:   "user pauses payment transition",
			actual: ReadyPaymentStatus,
			target: PausedPaymentStatus,
			err:    nil,
		},
		{
			name:   "user cancels payment transition",
			actual: ReadyPaymentStatus,
			target: CanceledPaymentStatus,
			err:    nil,
		},
		{
			name:   "user resumes payment transition",
			actual: PausedPaymentStatus,
			target: ReadyPaymentStatus,
			err:    nil,
		},
		{
			name:   "payment fails transition",
			actual: PendingPaymentStatus,
			target: FailedPaymentStatus,
			err:    nil,
		},
		{
			name:   "payment is retried transition",
			actual: FailedPaymentStatus,
			target: PendingPaymentStatus,
			err:    nil,
		},
		{
			name:   "payment succeeds transition",
			actual: PendingPaymentStatus,
			target: SuccessPaymentStatus,
			err:    nil,
		},
		{
			name:   "invalid cancellation 1",
			actual: DraftPaymentStatus,
			target: CanceledPaymentStatus,
			err:    fmt.Errorf("cannot transition from DRAFT to CANCELED"),
		},
		{
			name:   "invalid cancellation 2",
			actual: PendingPaymentStatus,
			target: CanceledPaymentStatus,
			err:    fmt.Errorf("cannot transition from PENDING to CANCELED"),
		},
		{
			name:   "invalid cancellation 3",
			actual: PausedPaymentStatus,
			target: CanceledPaymentStatus,
			err:    fmt.Errorf("cannot transition from PAUSED to CANCELED"),
		},
		{
			name:   "invalid cancellation 4",
			actual: FailedPaymentStatus,
			target: CanceledPaymentStatus,
			err:    fmt.Errorf("cannot transition from FAILED to CANCELED"),
		},
		{
			name:   "invalid cancellation 5",
			actual: SuccessPaymentStatus,
			target: CanceledPaymentStatus,
			err:    fmt.Errorf("cannot transition from SUCCESS to CANCELED"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.actual.TransitionTo(tt.target)
			if tt.err != nil {
				require.EqualError(t, err, tt.err.Error())
				return
			} else {
				require.NoError(t, err)
			}
		})
	}
}
