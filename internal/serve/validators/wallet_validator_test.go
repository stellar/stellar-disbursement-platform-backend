package validators

import (
	"testing"

	"github.com/stellar/go/support/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWalletValidator_ValidateCreateWalletRequest(t *testing.T) {
	t.Run("returns error when request body is empty", func(t *testing.T) {
		wv := NewWalletValidator()
		wv.ValidateCreateWalletRequest(nil)
		assert.True(t, wv.HasErrors())
		assert.Equal(t, map[string]interface{}{"body": "request body is empty"}, wv.Errors)
	})

	t.Run("returns error when request body has empty fields", func(t *testing.T) {
		wv := NewWalletValidator()
		reqBody := &WalletRequest{}

		wv.ValidateCreateWalletRequest(reqBody)
		assert.True(t, wv.HasErrors())
		assert.Equal(t, map[string]interface{}{
			"deep_link_schema":     "deep_link_schema is required",
			"homepage":             "homepage is required",
			"name":                 "name is required",
			"sep_10_client_domain": "sep_10_client_domain is required",
			"assets_ids":           "provide at least one asset ID",
		}, wv.Errors)

		reqBody.Name = "Wallet Provider"
		wv.Errors = map[string]interface{}{}
		wv.ValidateCreateWalletRequest(reqBody)
		assert.True(t, wv.HasErrors())
		assert.Equal(t, map[string]interface{}{
			"deep_link_schema":     "deep_link_schema is required",
			"homepage":             "homepage is required",
			"sep_10_client_domain": "sep_10_client_domain is required",
			"assets_ids":           "provide at least one asset ID",
		}, wv.Errors)
	})

	t.Run("returns error when homepage/deep link schema has a invalid URL", func(t *testing.T) {
		getEntries := log.DefaultLogger.StartTest(log.ErrorLevel)

		wv := NewWalletValidator()
		reqBody := &WalletRequest{
			Name:              "Wallet Provider",
			Homepage:          "no-schema-homepage.com",
			DeepLinkSchema:    "no-schema-deep-link",
			SEP10ClientDomain: "sep-10-client-domain.com",
			AssetsIDs:         []string{"asset-id"},
		}

		wv.ValidateCreateWalletRequest(reqBody)

		assert.True(t, wv.HasErrors())

		assert.Contains(t, wv.Errors, "homepage")
		assert.Equal(t, "invalid homepage URL provided", wv.Errors["homepage"])

		assert.Contains(t, wv.Errors, "deep_link_schema")
		assert.Equal(t, "invalid deep link schema provided", wv.Errors["deep_link_schema"])

		entries := getEntries()
		require.Len(t, entries, 2)
		assert.Equal(t, `parsing homepage URL: parse "no-schema-homepage.com": invalid URI for request`, entries[0].Message)
		assert.Equal(t, `parsing deep link schema: parse "no-schema-deep-link": invalid URI for request`, entries[1].Message)
	})

	t.Run("validates the homepage successfully", func(t *testing.T) {
		wv := NewWalletValidator()
		reqBody := &WalletRequest{
			Name:              "Wallet Provider",
			Homepage:          "https://homepage.com",
			DeepLinkSchema:    "wallet://deeplinkschema/sdp",
			SEP10ClientDomain: "sep-10-client-domain.com",
			AssetsIDs:         []string{"asset-id"},
		}

		wv.ValidateCreateWalletRequest(reqBody)
		assert.False(t, wv.HasErrors())

		reqBody.Homepage = "http://homepage.com/sdp?redirect=true"
		wv.ValidateCreateWalletRequest(reqBody)
		assert.False(t, wv.HasErrors())
		assert.Equal(t, map[string]interface{}{}, wv.Errors)
	})

	t.Run("validates the deep link schema successfully", func(t *testing.T) {
		wv := NewWalletValidator()
		reqBody := &WalletRequest{
			Name:              "Wallet Provider",
			Homepage:          "https://homepage.com",
			DeepLinkSchema:    "wallet://deeplinkschema/sdp",
			SEP10ClientDomain: "sep-10-client-domain.com",
			AssetsIDs:         []string{"asset-id"},
		}

		wv.ValidateCreateWalletRequest(reqBody)
		assert.False(t, wv.HasErrors())

		reqBody.DeepLinkSchema = "https://deeplinkschema.com/sdp?redirect=true"
		wv.ValidateCreateWalletRequest(reqBody)
		assert.False(t, wv.HasErrors())
	})

	t.Run("validates the SEP-10 Client Domain successfully", func(t *testing.T) {
		wv := NewWalletValidator()
		reqBody := &WalletRequest{
			Name:              "Wallet Provider",
			Homepage:          "https://homepage.com",
			DeepLinkSchema:    "wallet://deeplinkschema/sdp",
			SEP10ClientDomain: "https://sep-10-client-domain.com",
			AssetsIDs:         []string{"asset-id"},
		}

		reqBody = wv.ValidateCreateWalletRequest(reqBody)
		assert.False(t, wv.HasErrors())
		assert.Equal(t, "sep-10-client-domain.com", reqBody.SEP10ClientDomain)

		reqBody.SEP10ClientDomain = "https://sep-10-client-domain.com/sdp?redirect=true"
		reqBody = wv.ValidateCreateWalletRequest(reqBody)
		assert.False(t, wv.HasErrors())
		assert.Equal(t, "sep-10-client-domain.com", reqBody.SEP10ClientDomain)

		reqBody.SEP10ClientDomain = "http://localhost:8000"
		reqBody = wv.ValidateCreateWalletRequest(reqBody)
		assert.False(t, wv.HasErrors())
		assert.Equal(t, "localhost:8000", reqBody.SEP10ClientDomain)
	})
}
