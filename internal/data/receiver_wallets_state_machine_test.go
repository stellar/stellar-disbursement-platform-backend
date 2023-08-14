package data

import "testing"

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
