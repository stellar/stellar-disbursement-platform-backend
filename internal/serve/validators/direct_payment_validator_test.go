package validators

import (
	"testing"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/testutils"
)

func TestDirectPaymentValidator_ValidateCreateDirectPaymentRequest(t *testing.T) {
	t.Parallel()
	
	validStellarAccount := "GCFXHS4GXL6BVUCXBWXGTITROWLVYXQKQLF4YH5O5JT3YZXCYPAFBJZB"
	
	tests := []struct {
		name        string
		reqBody     *CreateDirectPaymentRequest
		expectValid bool
		expectError bool
	}{
		{
			name: "🟢 valid requests - various asset types",
			reqBody: &CreateDirectPaymentRequest{
				Amount: "100.50",
				Asset:  DirectPaymentAsset{ID: testutils.StringPtr("horus-asset-id")},
				Receiver: DirectPaymentReceiver{ID: testutils.StringPtr("sanguinius-receiver-id")},
				Wallet: DirectPaymentWallet{ID: testutils.StringPtr("fulgrim-wallet-id")},
			},
			expectValid: true,
		},
		{
			name: "🟢 valid native asset with email receiver",
			reqBody: &CreateDirectPaymentRequest{
				Amount: "250.00",
				Asset:  DirectPaymentAsset{Type: testutils.StringPtr("native")},
				Receiver: DirectPaymentReceiver{Email: testutils.StringPtr("guilliman@imperium.galaxy")},
				Wallet: DirectPaymentWallet{Address: testutils.StringPtr(validStellarAccount)},
			},
			expectValid: true,
		},
		{
			name: "🟢 valid classic asset with phone receiver",
			reqBody: &CreateDirectPaymentRequest{
				Amount: "1000.00",
				Asset: DirectPaymentAsset{
					Type:   testutils.StringPtr("classic"),
					Code:   testutils.StringPtr("USDC"),
					Issuer: testutils.StringPtr(validStellarAccount),
				},
				Receiver: DirectPaymentReceiver{PhoneNumber: testutils.StringPtr("+14155552671")},
				Wallet: DirectPaymentWallet{ID: testutils.StringPtr("angron-wallet")},
			},
			expectValid: true,
		},
		{
			name: "🟢 valid contract asset with wallet receiver",
			reqBody: &CreateDirectPaymentRequest{
				Amount: "500.25",
				Asset: DirectPaymentAsset{
					Type:       testutils.StringPtr("contract"),
					ContractID: testutils.StringPtr("contract-perturabo-001"),
				},
				Receiver: DirectPaymentReceiver{WalletAddress: testutils.StringPtr(validStellarAccount)},
				Wallet: DirectPaymentWallet{Address: testutils.StringPtr(validStellarAccount)},
			},
			expectValid: true,
		},
		{
			name: "🟢 valid fiat asset",
			reqBody: &CreateDirectPaymentRequest{
				Amount: "75.99",
				Asset:  DirectPaymentAsset{Type: testutils.StringPtr("fiat"), Code: testutils.StringPtr("USD")},
				Receiver: DirectPaymentReceiver{ID: testutils.StringPtr("mortarion-receiver")},
				Wallet: DirectPaymentWallet{ID: testutils.StringPtr("lorgar-wallet")},
			},
			expectValid: true,
		},
		{
			name: "🟢 amount with whitespace gets trimmed",
			reqBody: &CreateDirectPaymentRequest{
				Amount: "  100.50  ",
				Asset:  DirectPaymentAsset{ID: testutils.StringPtr("asset-id")},
				Receiver: DirectPaymentReceiver{ID: testutils.StringPtr("receiver-id")},
				Wallet: DirectPaymentWallet{ID: testutils.StringPtr("wallet-id")},
			},
			expectValid: true,
		},
		{
			name:        "🔴 nil request body",
			reqBody:     nil,
			expectError: true,
		},
		{
			name: "🔴 empty amount",
			reqBody: &CreateDirectPaymentRequest{
				Amount: "",
				Asset:  DirectPaymentAsset{ID: testutils.StringPtr("asset-id")},
				Receiver: DirectPaymentReceiver{ID: testutils.StringPtr("receiver-id")},
				Wallet: DirectPaymentWallet{ID: testutils.StringPtr("wallet-id")},
			},
			expectError: true,
		},
		{
			name: "🔴 whitespace amount",
			reqBody: &CreateDirectPaymentRequest{
				Amount: "   ",
				Asset:  DirectPaymentAsset{ID: testutils.StringPtr("asset-id")},
				Receiver: DirectPaymentReceiver{ID: testutils.StringPtr("receiver-id")},
				Wallet: DirectPaymentWallet{ID: testutils.StringPtr("wallet-id")},
			},
			expectError: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			
			v := NewDirectPaymentValidator()
			got := v.ValidateCreateDirectPaymentRequest(tt.reqBody)
			
			if tt.expectError {
				if got != nil || !v.HasErrors() {
					t.Errorf("expected validation error, got result: %v, errors: %v", got, v.Errors)
				}
			} else if tt.expectValid {
				if got == nil || v.HasErrors() {
					t.Errorf("expected valid result, got nil or errors: %v", v.Errors)
				}
				if got != nil && tt.reqBody != nil && got.Amount != "100.50" && tt.reqBody.Amount == "  100.50  " {
					t.Errorf("amount not properly trimmed: got %s", got.Amount)
				}
			}
		})
	}
}

func TestDirectPaymentValidator_validateAssetReference(t *testing.T) {
	t.Parallel()
	
	validStellarAccount := "GCFXHS4GXL6BVUCXBWXGTITROWLVYXQKQLF4YH5O5JT3YZXCYPAFBJZB"
	
	tests := []struct {
		name        string
		asset       *DirectPaymentAsset
		expectError bool
		errorFields []string
	}{
		{
			name:  "🟢 valid asset with ID only",
			asset: &DirectPaymentAsset{ID: testutils.StringPtr("horus-heresy-asset")},
		},
		{
			name:  "🟢 valid native asset",
			asset: &DirectPaymentAsset{Type: testutils.StringPtr("native")},
		},
		{
			name: "🟢 valid classic asset",
			asset: &DirectPaymentAsset{
				Type:   testutils.StringPtr("classic"),
				Code:   testutils.StringPtr("USDC"),
				Issuer: testutils.StringPtr(validStellarAccount),
			},
		},
		{
			name: "🟢 valid contract asset",
			asset: &DirectPaymentAsset{
				Type:       testutils.StringPtr("contract"),
				ContractID: testutils.StringPtr("mechanicus-contract-001"),
			},
		},
		{
			name:  "🟢 valid fiat asset",
			asset: &DirectPaymentAsset{Type: testutils.StringPtr("fiat"), Code: testutils.StringPtr("USD")},
		},
		{
			name: "🟢 fields with whitespace get trimmed",
			asset: &DirectPaymentAsset{
				Type:       testutils.StringPtr("  contract  "),
				ContractID: testutils.StringPtr("  mechanicus-001  "),
			},
		},
		{
			name:        "🔴 asset with both ID and type",
			asset:       &DirectPaymentAsset{ID: testutils.StringPtr("asset-id"), Type: testutils.StringPtr("native")},
			expectError: true,
			errorFields: []string{"asset"},
		},
		{
			name:        "🔴 asset with neither ID nor type",
			asset:       &DirectPaymentAsset{},
			expectError: true,
			errorFields: []string{"asset"},
		},
		{
			name:        "🔴 invalid asset type",
			asset:       &DirectPaymentAsset{Type: testutils.StringPtr("chaos")},
			expectError: true,
			errorFields: []string{"asset.type"},
		},
		{
			name: "🔴 classic asset missing code",
			asset: &DirectPaymentAsset{
				Type:   testutils.StringPtr("classic"),
				Issuer: testutils.StringPtr(validStellarAccount),
			},
			expectError: true,
			errorFields: []string{"asset.code"},
		},
		{
			name: "🔴 classic asset missing issuer",
			asset: &DirectPaymentAsset{
				Type: testutils.StringPtr("classic"),
				Code: testutils.StringPtr("USDC"),
			},
			expectError: true,
			errorFields: []string{"asset.issuer"},
		},
		{
			name: "🔴 classic asset with invalid issuer",
			asset: &DirectPaymentAsset{
				Type:   testutils.StringPtr("classic"),
				Code:   testutils.StringPtr("USDC"),
				Issuer: testutils.StringPtr("invalid-account"),
			},
			expectError: true,
			errorFields: []string{"asset.issuer"},
		},
		{
			name:        "🔴 contract asset missing contract_id",
			asset:       &DirectPaymentAsset{Type: testutils.StringPtr("contract")},
			expectError: true,
			errorFields: []string{"asset.contract_id"},
		},
		{
			name:        "🔴 fiat asset missing code",
			asset:       &DirectPaymentAsset{Type: testutils.StringPtr("fiat")},
			expectError: true,
			errorFields: []string{"asset.code"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			
			v := NewDirectPaymentValidator()
			v.validateAssetReference(tt.asset)

			if tt.expectError {
				if !v.HasErrors() {
					t.Error("expected validation errors, but validator has no errors")
				}
				for _, expectedField := range tt.errorFields {
					if _, found := v.Errors[expectedField]; !found {
						t.Errorf("expected error for field %s, but not found in errors: %v", expectedField, v.Errors)
					}
				}
			} else {
				if v.HasErrors() {
					t.Errorf("unexpected validation errors: %v", v.Errors)
				}
			}
		})
	}
}

func TestDirectPaymentValidator_validateReceiverReference(t *testing.T) {
	t.Parallel()
	
	validStellarAccount := "GCFXHS4GXL6BVUCXBWXGTITROWLVYXQKQLF4YH5O5JT3YZXCYPAFBJZB"
	
	tests := []struct {
		name        string
		receiver    *DirectPaymentReceiver
		expectError bool
		errorFields []string
	}{
		{
			name:     "🟢 valid ID",
			receiver: &DirectPaymentReceiver{ID: testutils.StringPtr("vulkan-receiver-001")},
		},
		{
			name:     "🟢 valid email",
			receiver: &DirectPaymentReceiver{Email: testutils.StringPtr("rogal.dorn@imperial.fists")},
		},
		{
			name:     "🟢 valid phone",
			receiver: &DirectPaymentReceiver{PhoneNumber: testutils.StringPtr("+14155552671")},
		},
		{
			name:     "🟢 valid wallet address",
			receiver: &DirectPaymentReceiver{WalletAddress: testutils.StringPtr(validStellarAccount)},
		},
		{
			name:     "🟢 fields with whitespace get trimmed",
			receiver: &DirectPaymentReceiver{ID: testutils.StringPtr("  corvus-corax  ")},
		},
		{
			name:        "🔴 no identifiers",
			receiver:    &DirectPaymentReceiver{},
			expectError: true,
			errorFields: []string{"receiver"},
		},
		{
			name: "🔴 multiple identifiers",
			receiver: &DirectPaymentReceiver{
				ID:    testutils.StringPtr("ferrus-manus"),
				Email: testutils.StringPtr("iron.hands@imperium.galaxy"),
			},
			expectError: true,
			errorFields: []string{"receiver"},
		},
		{
			name:        "🔴 invalid email format",
			receiver:    &DirectPaymentReceiver{Email: testutils.StringPtr("invalid-email")},
			expectError: true,
			errorFields: []string{"receiver.email"},
		},
		{
			name:        "🔴 invalid phone format",
			receiver:    &DirectPaymentReceiver{PhoneNumber: testutils.StringPtr("1234567890")},
			expectError: true,
			errorFields: []string{"receiver.phone_number"},
		},
		{
			name:        "🔴 invalid wallet address format",
			receiver:    &DirectPaymentReceiver{WalletAddress: testutils.StringPtr("invalid-account")},
			expectError: true,
			errorFields: []string{"receiver.wallet_address"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			
			v := NewDirectPaymentValidator()
			v.validateReceiverReference(tt.receiver)

			if tt.expectError {
				if !v.HasErrors() {
					t.Error("expected validation errors, but validator has no errors")
				}
				for _, expectedField := range tt.errorFields {
					if _, found := v.Errors[expectedField]; !found {
						t.Errorf("expected error for field %s, but not found in errors: %v", expectedField, v.Errors)
					}
				}
			} else {
				if v.HasErrors() {
					t.Errorf("unexpected validation errors: %v", v.Errors)
				}
			}
		})
	}
}

func TestDirectPaymentValidator_validateWalletReference(t *testing.T) {
	t.Parallel()
	
	validStellarAccount := "GCFXHS4GXL6BVUCXBWXGTITROWLVYXQKQLF4YH5O5JT3YZXCYPAFBJZB"
	
	tests := []struct {
		name        string
		wallet      *DirectPaymentWallet
		expectError bool
		errorFields []string
	}{
		{
			name:   "🟢 valid wallet with ID",
			wallet: &DirectPaymentWallet{ID: testutils.StringPtr("alpharius-wallet-001")},
		},
		{
			name:   "🟢 valid wallet with address",
			wallet: &DirectPaymentWallet{Address: testutils.StringPtr(validStellarAccount)},
		},
		{
			name:   "🟢 fields with whitespace get trimmed",
			wallet: &DirectPaymentWallet{ID: testutils.StringPtr("  jaghatai-khan-wallet  ")},
		},
		{
			name:        "🔴 neither ID nor address",
			wallet:      &DirectPaymentWallet{},
			expectError: true,
			errorFields: []string{"wallet"},
		},
		{
			name: "🔴 both ID and address",
			wallet: &DirectPaymentWallet{
				ID:      testutils.StringPtr("omegon-wallet"),
				Address: testutils.StringPtr(validStellarAccount),
			},
			expectError: true,
			errorFields: []string{"wallet"},
		},
		{
			name:        "🔴 invalid address format",
			wallet:      &DirectPaymentWallet{Address: testutils.StringPtr("invalid-account")},
			expectError: true,
			errorFields: []string{"wallet.address"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			
			v := NewDirectPaymentValidator()
			v.validateWalletReference(tt.wallet)

			if tt.expectError {
				if !v.HasErrors() {
					t.Error("expected validation errors, but validator has no errors")
				}
				for _, expectedField := range tt.errorFields {
					if _, found := v.Errors[expectedField]; !found {
						t.Errorf("expected error for field %s, but not found in errors: %v", expectedField, v.Errors)
					}
				}
			} else {
				if v.HasErrors() {
					t.Errorf("unexpected validation errors: %v", v.Errors)
				}
			}
		})
	}
}