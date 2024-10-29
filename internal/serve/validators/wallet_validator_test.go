package validators

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWalletValidator_ValidateCreateWalletRequest(t *testing.T) {
	ctx := context.Background()

	testCases := []struct {
		name            string
		reqBody         *WalletRequest
		expectedErrs    map[string]interface{}
		updateRequestFn func(wr *WalletRequest)
		enforceHTTPS    bool
	}{
		{
			name:         "🔴 error when request body is empty",
			reqBody:      nil,
			expectedErrs: map[string]interface{}{"body": "request body is empty"},
		},
		{
			name:    "🔴 error when request body has empty fields",
			reqBody: &WalletRequest{},
			expectedErrs: map[string]interface{}{
				"deep_link_schema":     "deep_link_schema is required",
				"homepage":             "homepage is required",
				"name":                 "name is required",
				"sep_10_client_domain": "sep_10_client_domain is required",
				"assets_ids":           "provide at least one asset ID",
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
			expectedErrs: map[string]interface{}{
				"homepage":             "invalid homepage URL provided",
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
			expectedErrs: map[string]interface{}{},
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
			expectedErrs: map[string]interface{}{},
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
			expectedErrs: map[string]interface{}{
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
		t.Run(tc.name, func(t *testing.T) {
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
	ctx := context.Background()

	t.Run("returns error when request body is empty", func(t *testing.T) {
		wv := NewWalletValidator()
		wv.ValidateCreateWalletRequest(ctx, nil, false)
		assert.True(t, wv.HasErrors())
		assert.Equal(t, map[string]interface{}{"body": "request body is empty"}, wv.Errors)
	})

	t.Run("returns error when body has empty fields", func(t *testing.T) {
		wv := NewWalletValidator()
		reqBody := &PatchWalletRequest{}

		wv.ValidatePatchWalletRequest(reqBody)
		assert.True(t, wv.HasErrors())
		assert.Equal(t, map[string]interface{}{
			"enabled": "enabled is required",
		}, wv.Errors)
	})

	t.Run("validates successfully", func(t *testing.T) {
		wv := NewWalletValidator()

		e := new(bool)
		assert.False(t, *e)
		reqBody := &PatchWalletRequest{
			Enabled: e,
		}

		wv.ValidatePatchWalletRequest(reqBody)
		assert.False(t, wv.HasErrors())
		assert.Empty(t, wv.Errors)

		*e = true
		assert.True(t, *e)
		reqBody = &PatchWalletRequest{
			Enabled: e,
		}

		wv.ValidatePatchWalletRequest(reqBody)
		assert.False(t, wv.HasErrors())
		assert.Empty(t, wv.Errors)
	})
}
