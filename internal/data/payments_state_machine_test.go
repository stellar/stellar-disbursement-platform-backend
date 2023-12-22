package data

import (
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
