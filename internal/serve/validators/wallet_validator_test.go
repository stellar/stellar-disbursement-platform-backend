package validators

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

func TestWalletValidator_ValidateCreateWalletRequest(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	testCases := []struct {
		name            string
		reqBody         *WalletRequest
		expectedErrs    map[string]any
		updateRequestFn func(wr *WalletRequest)
		enforceHTTPS    bool
	}{
		{
			name:         "🔴 error when request body is empty",
			reqBody:      nil,
			expectedErrs: map[string]any{"body": "request body is empty"},
		},
		{
			name:    "🔴 error when request body has empty fields",
			reqBody: &WalletRequest{},
			expectedErrs: map[string]any{
				"deep_link_schema":     "deep_link_schema is required",
				"homepage":             "homepage is required",
				"name":                 "name is required",
				"sep_10_client_domain": "sep_10_client_domain is required",
				"assets":               "provide at least one 'assets_ids' or 'assets'",
			},
		},
		{
			name: "🔴 error when homepage,deep-link,client-domain are invalid",
			reqBody: &WalletRequest{
				Name:              "Wallet Provider",
				Homepage:          "no-schema-homepage.com",
				DeepLinkSchema:    "no-schema-deep-link",
				SEP10ClientDomain: "-invaliddomain",
				AssetsIDs:         []string{"asset-id"},
			},
			expectedErrs: map[string]any{
				"homepage":             "invalid URL format",
				"deep_link_schema":     "invalid deep link schema provided",
				"sep_10_client_domain": "invalid SEP-10 client domain provided",
			},
		},
		{
			name: "🟢 successfully validates the homepage,deep-link,client-domain",
			reqBody: &WalletRequest{
				Name:              "Wallet Provider",
				Homepage:          "https://homepage.com",
				DeepLinkSchema:    "wallet://deeplinkschema/sdp",
				SEP10ClientDomain: "sep-10-client-domain.com",
				AssetsIDs:         []string{"asset-id"},
			},
			expectedErrs: map[string]any{},
		},
		{
			name: "🟢 successfully validates the homepage,deep-link,client-domain with query params",
			reqBody: &WalletRequest{
				Name:              "Wallet Provider",
				Homepage:          "http://homepage.com/sdp?redirect=true",
				DeepLinkSchema:    "https://deeplinkschema.com/sdp?redirect=true",
				SEP10ClientDomain: "sep-10-client-domain.com",
				AssetsIDs:         []string{"asset-id"},
			},
			expectedErrs: map[string]any{},
		},
		{
			name: "🔴 fails if enforceHttps=true && homepage=http://...",
			reqBody: &WalletRequest{
				Name:              "Wallet Provider",
				Homepage:          "http://homepage.com/sdp?redirect=true",
				DeepLinkSchema:    "https://deeplinkschema.com/sdp?redirect=true",
				SEP10ClientDomain: "sep-10-client-domain.com",
				AssetsIDs:         []string{"asset-id"},
			},
			expectedErrs: map[string]any{
				"homepage": "invalid URL scheme is not part of [https]",
			},
			enforceHTTPS: true,
		},
		{
			name: "🟢 successfully validates the homepage,deep-link,client-domain and values get sanitized",
			reqBody: &WalletRequest{
				Name:              "Wallet Provider",
				Homepage:          "https://homepage.com",
				DeepLinkSchema:    "wallet://deeplinkschema/sdp",
				SEP10ClientDomain: "https://sep-10-client-domain.com",
				AssetsIDs:         []string{"asset-id"},
			},
			updateRequestFn: func(wr *WalletRequest) {
				wr.SEP10ClientDomain = "sep-10-client-domain.com"
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			wv := NewWalletValidator()
			reqBody := wv.ValidateCreateWalletRequest(ctx, tc.reqBody, tc.enforceHTTPS)

			if len(tc.expectedErrs) == 0 {
				require.Falsef(t, wv.HasErrors(), "expected no errors, got: %v", wv.Errors)
				if tc.updateRequestFn != nil {
					tc.updateRequestFn(tc.reqBody)
				}
				assert.Equal(t, tc.reqBody, reqBody)
			} else {
				assert.True(t, wv.HasErrors())
				assert.Equal(t, tc.expectedErrs, wv.Errors)
			}
		})
	}
}

func TestWalletValidator_ValidatePatchWalletRequest(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("returns error when request body is empty", func(t *testing.T) {
		t.Parallel()
		wv := NewWalletValidator()
		wv.ValidateCreateWalletRequest(ctx, nil, false)
		assert.True(t, wv.HasErrors())
		assert.Equal(t, map[string]any{"body": "request body is empty"}, wv.Errors)
	})

	t.Run("returns error when body has empty fields", func(t *testing.T) {
		t.Parallel()
		wv := NewWalletValidator()
		reqBody := &PatchWalletRequest{}

		wv.ValidatePatchWalletRequest(ctx, reqBody, false)
		assert.True(t, wv.HasErrors())
		assert.Equal(t, map[string]any{
			"body": "at least one field must be provided for update",
		}, wv.Errors)
	})

	t.Run("validates successfully", func(t *testing.T) {
		t.Parallel()
		wv := NewWalletValidator()

		e := new(bool)
		assert.False(t, *e)
		reqBody := &PatchWalletRequest{
			Enabled: e,
		}

		wv.ValidatePatchWalletRequest(ctx, reqBody, false)
		assert.False(t, wv.HasErrors())
		assert.Empty(t, wv.Errors)

		*e = true
		assert.True(t, *e)
		reqBody = &PatchWalletRequest{
			Enabled: e,
		}

		wv.ValidatePatchWalletRequest(ctx, reqBody, false)
		assert.False(t, wv.HasErrors())
		assert.Empty(t, wv.Errors)
	})
}

func TestAssetReference_Validate(t *testing.T) {
	testCases := []struct {
		name          string
		assetRef      AssetReference
		expectedError string
	}{
		{
			name: "valid ID reference",
			assetRef: AssetReference{
				ID: "ef262966-1cbb-4fdb-9f6f-cc335e954dd1",
			},
			expectedError: "",
		},
		{
			name: "ID reference with other fields should fail",
			assetRef: AssetReference{
				ID:   "ef262966-1cbb-4fdb-9f6f-cc335e954dd1",
				Type: "classic",
				Code: "USDC",
			},
			expectedError: "when 'id' is provided, other fields should not be present",
		},

		{
			name: "valid classic asset",
			assetRef: AssetReference{
				Type:   "classic",
				Code:   "USDC",
				Issuer: "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
			},
			expectedError: "",
		},
		{
			name: "classic asset missing code",
			assetRef: AssetReference{
				Type:   "classic",
				Issuer: "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
			},
			expectedError: "'code' is required for classic asset",
		},
		{
			name: "classic asset missing issuer",
			assetRef: AssetReference{
				Type: "classic",
				Code: "USDC",
			},
			expectedError: "'issuer' is required for classic asset",
		},
		{
			name: "classic asset with invalid issuer format",
			assetRef: AssetReference{
				Type:   "classic",
				Code:   "USDC",
				Issuer: "invalid-issuer",
			},
			expectedError: "invalid issuer address format",
		},
		{
			name: "classic asset with issuer starting with wrong character",
			assetRef: AssetReference{
				Type:   "classic",
				Code:   "USDC",
				Issuer: "ABBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
			},
			expectedError: "invalid issuer address format",
		},

		{
			name: "valid native asset",
			assetRef: AssetReference{
				Type: "native",
			},
			expectedError: "",
		},
		{
			name: "native asset with code should fail",
			assetRef: AssetReference{
				Type: "native",
				Code: "XLM",
			},
			expectedError: "native asset should not have code, issuer, or contract_id",
		},
		{
			name: "native asset with issuer should fail",
			assetRef: AssetReference{
				Type:   "native",
				Issuer: "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
			},
			expectedError: "native asset should not have code, issuer, or contract_id",
		},
		{
			name:          "empty reference",
			assetRef:      AssetReference{},
			expectedError: "either 'id' or 'type' must be provided",
		},
		{
			name: "invalid asset type",
			assetRef: AssetReference{
				Type: "invalid-type",
			},
			expectedError: "invalid asset type: invalid-type",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.assetRef.Validate()
			if tc.expectedError == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedError)
			}
		})
	}
}

func TestValidateWalletAddressMemo(t *testing.T) {
	contractAddress := "CAMAMZUOULVWFAB3KRROW5ELPUFHSEKPUALORCFBLFX7XBWWUCUJLR53"
	publicKey := "GB3SAK22KSTIFQAV5GCDNPW7RTQCWGFDKALBY5KJ3JRF2DLSED3E7PVH"

	testCases := []struct {
		name        string
		address     string
		memo        string
		wantType    schema.MemoType
		wantErr     error
		errContains string
	}{
		{
			name:    "contract address with memo is rejected",
			address: contractAddress,
			memo:    "1234",
			wantErr: ErrMemoNotSupportedForContract,
		},
		{
			name:     "contract address without memo is allowed",
			address:  contractAddress,
			memo:     "",
			wantType: "",
			wantErr:  nil,
		},
		{
			name:     "public key numeric memo returns ID type",
			address:  publicKey,
			memo:     "123456",
			wantType: schema.MemoTypeID,
			wantErr:  nil,
		},
		{
			name:        "public key invalid memo returns parse error",
			address:     publicKey,
			memo:        strings.Repeat("x", 40),
			errContains: "parsing memo",
		},
		{
			name:    "unknown address leaves memo untouched",
			address: "",
			memo:    "some-memo",
			wantErr: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			memoType, err := ValidateWalletAddressMemo(tc.address, tc.memo)

			if tc.wantErr != nil {
				require.Error(t, err)
				assert.True(t, errors.Is(err, tc.wantErr), "expected error %v, got %v", tc.wantErr, err)
				return
			}

			if tc.errContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errContains)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.wantType, memoType)
		})
	}
}

func TestWalletValidator_ValidateCreateWalletRequest_WithAssets(t *testing.T) {
	ctx := context.Background()

	testCases := []struct {
		name          string
		reqBody       *WalletRequest
		enforceHTTPS  bool
		expectedError map[string]string
		expectedBody  *WalletRequest
	}{
		{
			name: "valid request with legacy assets_ids",
			reqBody: &WalletRequest{
				Name:              "Test Wallet",
				Homepage:          "https://testwallet.com",
				DeepLinkSchema:    "testwallet://sdp",
				SEP10ClientDomain: "testwallet.com",
				AssetsIDs:         []string{"asset-id-1", "asset-id-2"},
			},
			enforceHTTPS:  true,
			expectedError: nil,
			expectedBody: &WalletRequest{
				Name:              "Test Wallet",
				Homepage:          "https://testwallet.com",
				DeepLinkSchema:    "testwallet://sdp",
				SEP10ClientDomain: "testwallet.com",
				AssetsIDs:         []string{"asset-id-1", "asset-id-2"},
				Assets:            nil,
				Enabled:           boolToPtr(true),
			},
		},
		{
			name: "valid request with new assets format",
			reqBody: &WalletRequest{
				Name:              "Test Wallet",
				Homepage:          "https://testwallet.com",
				DeepLinkSchema:    "testwallet://sdp",
				SEP10ClientDomain: "testwallet.com",
				Assets: []AssetReference{
					{ID: "asset-id-1"},
					{Type: "native"},
					{Type: "classic", Code: "USDC", Issuer: "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5"},
				},
			},
			enforceHTTPS:  true,
			expectedError: nil,
			expectedBody: &WalletRequest{
				Name:              "Test Wallet",
				Homepage:          "https://testwallet.com",
				DeepLinkSchema:    "testwallet://sdp",
				SEP10ClientDomain: "testwallet.com",
				AssetsIDs:         nil,
				Assets: []AssetReference{
					{ID: "asset-id-1"},
					{Type: "native"},
					{Type: "classic", Code: "USDC", Issuer: "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5"},
				},
				Enabled: boolToPtr(true),
			},
		},
		{
			name: "request with enabled field",
			reqBody: &WalletRequest{
				Name:              "Test Wallet",
				Homepage:          "https://testwallet.com",
				DeepLinkSchema:    "testwallet://sdp",
				SEP10ClientDomain: "testwallet.com",
				AssetsIDs:         []string{"asset-id-1"},
				Enabled:           &[]bool{true}[0],
			},
			enforceHTTPS:  true,
			expectedError: nil,
			expectedBody: &WalletRequest{
				Name:              "Test Wallet",
				Homepage:          "https://testwallet.com",
				DeepLinkSchema:    "testwallet://sdp",
				SEP10ClientDomain: "testwallet.com",
				AssetsIDs:         []string{"asset-id-1"},
				Assets:            nil,
				Enabled:           &[]bool{true}[0],
			},
		},
		{
			name: "error when both assets_ids and assets are provided",
			reqBody: &WalletRequest{
				Name:              "Test Wallet",
				Homepage:          "https://testwallet.com",
				DeepLinkSchema:    "testwallet://sdp",
				SEP10ClientDomain: "testwallet.com",
				AssetsIDs:         []string{"asset-id-1"},
				Assets: []AssetReference{
					{ID: "asset-id-2"},
				},
			},
			enforceHTTPS: true,
			expectedError: map[string]string{
				"assets": "cannot use both 'assets_ids' and 'assets' fields simultaneously",
			},
			expectedBody: nil,
		},
		{
			name: "error when no assets provided",
			reqBody: &WalletRequest{
				Name:              "Test Wallet",
				Homepage:          "https://testwallet.com",
				DeepLinkSchema:    "testwallet://sdp",
				SEP10ClientDomain: "testwallet.com",
			},
			enforceHTTPS: true,
			expectedError: map[string]string{
				"assets": "provide at least one 'assets_ids' or 'assets'",
			},
			expectedBody: nil,
		},
		{
			name: "error with invalid asset reference",
			reqBody: &WalletRequest{
				Name:              "Test Wallet",
				Homepage:          "https://testwallet.com",
				DeepLinkSchema:    "testwallet://sdp",
				SEP10ClientDomain: "testwallet.com",
				Assets: []AssetReference{
					{Type: "classic", Code: "USDC"},
				},
			},
			enforceHTTPS: true,
			expectedError: map[string]string{
				"assets[0]": "'issuer' is required for classic asset",
			},
			expectedBody: nil,
		},
		{
			name: "error with multiple invalid asset references",
			reqBody: &WalletRequest{
				Name:              "Test Wallet",
				Homepage:          "https://testwallet.com",
				DeepLinkSchema:    "testwallet://sdp",
				SEP10ClientDomain: "testwallet.com",
				Assets: []AssetReference{
					{Type: "contract", Code: "USDC", ContractID: "CA..."},
					{Type: "fiat", Code: "USD"},
				},
			},
			enforceHTTPS: true,
			expectedError: map[string]string{
				"assets[0]": "assets are not implemented yet",
				"assets[1]": "assets are not implemented yet",
			},
			expectedBody: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			validator := NewWalletValidator()
			result := validator.ValidateCreateWalletRequest(ctx, tc.reqBody, tc.enforceHTTPS)

			if tc.expectedError != nil {
				require.True(t, validator.HasErrors())
				require.Nil(t, result)

				for field, expectedMsg := range tc.expectedError {
					actualErrors, ok := validator.Errors[field]
					require.True(t, ok, "Expected error for field %s not found", field)
					require.Contains(t, actualErrors, expectedMsg)
				}
			} else {
				require.False(t, validator.HasErrors())
				require.NotNil(t, result)
				assert.Equal(t, tc.expectedBody, result)
			}
		})
	}
}

func TestWalletValidator_BackwardCompatibility(t *testing.T) {
	ctx := context.Background()

	legacyRequest := &WalletRequest{
		Name:              "Legacy Wallet",
		Homepage:          "https://legacy.com",
		DeepLinkSchema:    "legacy://sdp",
		SEP10ClientDomain: "legacy.com",
		AssetsIDs:         []string{"id-1", "id-2", "id-3"},
	}

	validator := NewWalletValidator()
	result := validator.ValidateCreateWalletRequest(ctx, legacyRequest, true)

	require.False(t, validator.HasErrors())
	require.NotNil(t, result)
	assert.Equal(t, []string{"id-1", "id-2", "id-3"}, result.AssetsIDs)
	assert.Nil(t, result.Assets)
}

func TestWalletValidator_AssetReferenceValidation(t *testing.T) {
	ctx := context.Background()

	request := &WalletRequest{
		Name:              "Multi Asset Wallet",
		Homepage:          "https://multi.com",
		DeepLinkSchema:    "multi://sdp",
		SEP10ClientDomain: "multi.com",
		Assets: []AssetReference{
			{ID: "existing-id"},
			{Type: "native"},
			{Type: "classic", Code: "USDC", Issuer: "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5"},

			{Type: "classic", Code: "BAD"},
		},
	}

	validator := NewWalletValidator()
	result := validator.ValidateCreateWalletRequest(ctx, request, true)

	require.True(t, validator.HasErrors())
	require.Nil(t, result)

	errors, ok := validator.Errors["assets[3]"]
	require.True(t, ok)
	assert.Contains(t, errors, "'issuer' is required for classic asset")
}

func boolToPtr(b bool) *bool {
	return &b
}
