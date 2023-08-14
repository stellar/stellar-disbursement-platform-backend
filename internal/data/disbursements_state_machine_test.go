package data

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_DisbursementStatus_ToDisbursementStatus(t *testing.T) {
	tests := []struct {
		name   string
		actual string
		want   DisbursementStatus
		err    error
	}{
		{
			name:   "valid entry",
			actual: "STARTED",
			want:   StartedDisbursementStatus,
			err:    nil,
		},
		{
			name:   "valid lower case",
			actual: "draft",
			want:   DraftDisbursementStatus,
			err:    nil,
		},
		{
			name:   "valid weird case",
			actual: "ReAdY",
			want:   ReadyDisbursementStatus,
			err:    nil,
		},
		{
			name:   "invalid entry",
			actual: "NOT_VALID",
			want:   StartedDisbursementStatus,
			err:    fmt.Errorf("invalid disbursement status: NOT_VALID"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ToDisbursementStatus(tt.actual)

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

func Test_DisbursementStatus_TransitionTo(t *testing.T) {
	tests := []struct {
		name   string
		actual DisbursementStatus
		target DisbursementStatus
		err    error
	}{
		{
			name:   "instructions uploaded successfully transition",
			actual: DraftDisbursementStatus,
			target: ReadyDisbursementStatus,
			err:    nil,
		},
		{
			name:   "user re-uploads instructions transition",
			actual: ReadyDisbursementStatus,
			target: ReadyDisbursementStatus,
			err:    nil,
		},
		{
			name:   "instructions uploaded successfully transition",
			actual: DraftDisbursementStatus,
			target: ReadyDisbursementStatus,
			err:    nil,
		},
		{
			name:   "user starts disbursement transition",
			actual: ReadyDisbursementStatus,
			target: StartedDisbursementStatus,
			err:    nil,
		},
		{
			name:   "user pauses disbursement transition",
			actual: StartedDisbursementStatus,
			target: PausedDisbursementStatus,
			err:    nil,
		},
		{
			name:   "user resumes disbursement transition",
			actual: PausedDisbursementStatus,
			target: StartedDisbursementStatus,
			err:    nil,
		},
		{
			name:   "all payments went through transition",
			actual: StartedDisbursementStatus,
			target: CompletedDisbursementStatus,
			err:    nil,
		},
		{
			name:   "invalid transition 1",
			actual: DraftDisbursementStatus,
			target: StartedDisbursementStatus,
			err:    fmt.Errorf("cannot transition from DRAFT to STARTED"),
		},
		{
			name:   "invalid transition 2",
			actual: StartedDisbursementStatus,
			target: DraftDisbursementStatus,
			err:    fmt.Errorf("cannot transition from STARTED to DRAFT"),
		},
		{
			name:   "invalid transition 3",
			actual: DraftDisbursementStatus,
			target: PausedDisbursementStatus,
			err:    fmt.Errorf("cannot transition from DRAFT to PAUSED"),
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

func Test_DisbursementStatus_SourceStatuses(t *testing.T) {
	tests := []struct {
		name                   string
		targetStatus           DisbursementStatus
		expectedSourceStatuses []DisbursementStatus
	}{
		{
			name:                   "Draft",
			targetStatus:           DraftDisbursementStatus,
			expectedSourceStatuses: []DisbursementStatus{},
		},
		{
			name:                   "Ready",
			targetStatus:           ReadyDisbursementStatus,
			expectedSourceStatuses: []DisbursementStatus{DraftDisbursementStatus, ReadyDisbursementStatus},
		},
		{
			name:                   "Started",
			targetStatus:           StartedDisbursementStatus,
			expectedSourceStatuses: []DisbursementStatus{ReadyDisbursementStatus, PausedDisbursementStatus},
		},
		{
			name:                   "Paused",
			targetStatus:           PausedDisbursementStatus,
			expectedSourceStatuses: []DisbursementStatus{StartedDisbursementStatus},
		},
		{
			name:                   "Completed",
			targetStatus:           CompletedDisbursementStatus,
			expectedSourceStatuses: []DisbursementStatus{StartedDisbursementStatus},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expectedSourceStatuses, tt.targetStatus.SourceStatuses())
		})
	}
}

func Test_DisbursementStatus_DisbursementStatuses(t *testing.T) {
	expectedStatuses := []DisbursementStatus{DraftDisbursementStatus, ReadyDisbursementStatus, StartedDisbursementStatus, PausedDisbursementStatus, CompletedDisbursementStatus}
	require.Equal(t, expectedStatuses, DisbursementStatuses())
}
