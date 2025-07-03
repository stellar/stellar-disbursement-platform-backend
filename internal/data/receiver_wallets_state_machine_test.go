package data

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_ReceiversWalletStatus_TransitionTo(t *testing.T) {
	tests := []struct {
		name    string
		initial ReceiversWalletStatus
		target  ReceiversWalletStatus
		wantErr bool
	}{
		{
			"DRAFT to READY",
			DraftReceiversWalletStatus,
			ReadyReceiversWalletStatus,
			false,
		},
		{
			"READY to REGISTERED",
			ReadyReceiversWalletStatus,
			RegisteredReceiversWalletStatus,
			false,
		},
		{
			"READY to FLAGGED",
			ReadyReceiversWalletStatus,
			FlaggedReceiversWalletStatus,
			false,
		},
		{
			"FLAGGED to READY",
			FlaggedReceiversWalletStatus,
			ReadyReceiversWalletStatus,
			false,
		},
		{
			"REGISTERED to FLAGGED",
			RegisteredReceiversWalletStatus,
			FlaggedReceiversWalletStatus,
			false,
		},
		{
			"FLAGGED to REGISTERED",
			FlaggedReceiversWalletStatus,
			RegisteredReceiversWalletStatus,
			false,
		},
		{
			"DRAFT to REGISTERED",
			DraftReceiversWalletStatus,
			RegisteredReceiversWalletStatus,
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.initial.TransitionTo(tt.target); (err != nil) != tt.wantErr {
				t.Errorf("ReceiversWalletStatus.TransitionTo() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_ReceiversWalletStatus_Validate(t *testing.T) {
	tests := []struct {
		name   string
		status ReceiversWalletStatus
		err    string
	}{
		{
			"validate Draft receiver wallet status",
			DraftReceiversWalletStatus,
			"",
		},
		{
			"validate Ready receiver wallet status",
			ReadyReceiversWalletStatus,
			"",
		},
		{
			"validate Registered receiver wallet status",
			RegisteredReceiversWalletStatus,
			"",
		},
		{
			"validate Flagged receiver wallet status",
			FlaggedReceiversWalletStatus,
			"",
		},
		{
			"invalid receiver wallet status",
			ReceiversWalletStatus("INVALID"),
			"invalid receiver wallet status \"INVALID\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.status.Validate()
			if tt.err == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.err)
			}
		})
	}
}

func Test_ReceiversWalletStatus_ReceiversWalletStatuses(t *testing.T) {
	expectedStatuses := []ReceiversWalletStatus{DraftReceiversWalletStatus, ReadyReceiversWalletStatus, RegisteredReceiversWalletStatus, FlaggedReceiversWalletStatus}
	require.Equal(t, expectedStatuses, ReceiversWalletStatuses())
}

func Test_ReceiversWalletStatus_ToReceiversWalletStatus(t *testing.T) {
	tests := []struct {
		name   string
		actual string
		want   ReceiversWalletStatus
		err    error
	}{
		{
			name:   "valid status",
			actual: "DRAFT",
			want:   DraftReceiversWalletStatus,
			err:    nil,
		},
		{
			name:   "valid status with lower case",
			actual: "draft",
			want:   DraftReceiversWalletStatus,
			err:    nil,
		},
		{
			name:   "valid status with mixed case",
			actual: "DrAfT",
			want:   DraftReceiversWalletStatus,
			err:    nil,
		},
		{
			name:   "invalid status",
			actual: "INVALID",
			want:   "",
			err:    fmt.Errorf("invalid receiver wallet status \"INVALID\""),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ToReceiversWalletStatus(tt.actual)

			if tt.err != nil {
				assert.ErrorContains(t, err, tt.err.Error())
				return
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
