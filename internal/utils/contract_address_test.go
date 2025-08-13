package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCalculateContractAddress(t *testing.T) {
	tests := []struct {
		name        string
		account     string
		salt        string
		network     string
		wantErr     bool
		wantAddress string
	}{
		{
			name:        "valid",
			account:     "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
			salt:        "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824",
			network:     "Test SDF Network ; September 2015",
			wantAddress: "CAD3HTTNXS7OWHVKBAST6UZFMLZOSGY3SLTDHMNXMWW4CVX72NXST6QU",
		},
		{
			name:    "invalid salt length",
			account: "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
			salt:    "2cf24dba",
			network: "Test SDF Network ; September 2015",
			wantErr: true,
		},
		{
			name:    "invalid account",
			account: "INVALID",
			salt:    "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824",
			network: "Test SDF Network ; September 2015",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CalculateContractAddress(tt.account, tt.salt, tt.network)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Empty(t, got)
				return
			}
			require.NoError(t, err)
			if tt.wantAddress != "" {
				assert.Equal(t, tt.wantAddress, got)
			}
			assert.Len(t, got, 56)
			assert.Equal(t, byte('C'), got[0])
		})
	}
}

func TestCalculateContractAddressFromReceiver(t *testing.T) {
	tests := []struct {
		name    string
		email   string
		phone   string
		account string
		network string
		wantErr bool
	}{
		{
			name:    "with email",
			email:   "test@example.com",
			account: "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
			network: "Test SDF Network ; September 2015",
		},
		{
			name:    "with phone",
			phone:   "+1-555-123-4567",
			account: "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
			network: "Test SDF Network ; September 2015",
		},
		{
			name:    "email takes precedence",
			email:   "test@example.com",
			phone:   "+1-555-123-4567",
			account: "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
			network: "Test SDF Network ; September 2015",
		},
		{
			name:    "no contact info",
			account: "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
			network: "Test SDF Network ; September 2015",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CalculateContractAddressFromReceiver(tt.email, tt.phone, tt.account, tt.network)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Empty(t, got)
				return
			}
			require.NoError(t, err)
			assert.Len(t, got, 56)
			assert.Equal(t, byte('C'), got[0])
		})
	}
}
