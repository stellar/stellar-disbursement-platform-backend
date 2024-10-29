package data

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
