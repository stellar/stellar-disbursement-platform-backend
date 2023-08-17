package httphandler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stellar/go/network"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_StellarTomlHandler_horizonURL(t *testing.T) {
	testCases := []struct {
		name string
		s    StellarTomlHandler
		want string
	}{
		{
			name: "pubnet",
			s:    StellarTomlHandler{NetworkPassphrase: network.PublicNetworkPassphrase},
			want: horizonPubnetURL,
		},
		{
			name: "testnet",
			s:    StellarTomlHandler{NetworkPassphrase: network.TestNetworkPassphrase},
			want: horizonTestnetURL,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.s.horizonURL(); got != tc.want {
				t.Errorf("StellarTomlHandler.horizonURL() = %v, want %v", got, tc.want)
			}
		})
	}
}

func Test_StellarTomlHandler_buildGeneralInformation(t *testing.T) {
	testCases := []struct {
		name string
		s    StellarTomlHandler
		want string
	}{
		{
			name: "pubnet",
			s: StellarTomlHandler{
				DistributionPublicKey:    "GBC2HVWFIFN7WJHFORVBCDKJORG6LWTW3O2QBHOURL3KHZPM4KMWTUSA",
				NetworkPassphrase:        network.PublicNetworkPassphrase,
				Sep10SigningPublicKey:    "GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S",
				AnchorPlatformBaseSepURL: "https://anchor-platform-domain",
			},
			want: fmt.Sprintf(`
		ACCOUNTS=["GBC2HVWFIFN7WJHFORVBCDKJORG6LWTW3O2QBHOURL3KHZPM4KMWTUSA", "GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S"]
		SIGNING_KEY="GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S"
		NETWORK_PASSPHRASE=%q
		HORIZON_URL=%q
		WEB_AUTH_ENDPOINT="https://anchor-platform-domain/auth"
		TRANSFER_SERVER_SEP0024="https://anchor-platform-domain/sep24"
	`, network.PublicNetworkPassphrase, horizonPubnetURL),
		},
		{
			name: "testnet",
			s: StellarTomlHandler{
				DistributionPublicKey:    "GBC2HVWFIFN7WJHFORVBCDKJORG6LWTW3O2QBHOURL3KHZPM4KMWTUSA",
				NetworkPassphrase:        network.TestNetworkPassphrase,
				Sep10SigningPublicKey:    "GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S",
				AnchorPlatformBaseSepURL: "https://anchor-platform-domain",
			},
			want: fmt.Sprintf(`
		ACCOUNTS=["GBC2HVWFIFN7WJHFORVBCDKJORG6LWTW3O2QBHOURL3KHZPM4KMWTUSA", "GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S"]
		SIGNING_KEY="GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S"
		NETWORK_PASSPHRASE=%q
		HORIZON_URL=%q
		WEB_AUTH_ENDPOINT="https://anchor-platform-domain/auth"
		TRANSFER_SERVER_SEP0024="https://anchor-platform-domain/sep24"
	`, network.TestNetworkPassphrase, horizonTestnetURL),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			genaralInformation := tc.s.buildGeneralInformation()
			assert.Equal(t, tc.want, genaralInformation)
		})
	}
}

func Test_StellarTomlHandler_buildOrganizationDocumentation(t *testing.T) {
	stellarTomlHandler := StellarTomlHandler{}
	testCases := []struct {
		name         string
		organization data.Organization
		want         string
	}{
		{
			name: "FOO Org",
			organization: data.Organization{
				Name: "FOO Org",
			},
			want: `
		[DOCUMENTATION]
		ORG_NAME="FOO Org"
	`,
		},
		{
			name: "BAR Org",
			organization: data.Organization{
				Name: "BAR Org",
			},
			want: `
		[DOCUMENTATION]
		ORG_NAME="BAR Org"
	`,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			genaralInformation := stellarTomlHandler.buildOrganizationDocumentation(tc.organization)
			assert.Equal(t, tc.want, genaralInformation)
		})
	}
}

func Test_StellarTomlHandler_buildCurrencyInformation(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	s := StellarTomlHandler{}

	t.Run("build currency information without assets", func(t *testing.T) {
		currencyInformation := s.buildCurrencyInformation([]data.Asset{})
		assert.Empty(t, currencyInformation)
	})

	t.Run("build currency information with asset", func(t *testing.T) {
		asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZCC")

		currencyInformation := s.buildCurrencyInformation([]data.Asset{*asset})
		wantStr := `
		[[CURRENCIES]]
		code = "USDC"
		issuer = "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZCC"
		is_asset_anchored = true
		anchor_asset_type = "fiat"
		status = "live"
		desc = "USDC"
	`

		assert.Equal(t, wantStr, currencyInformation)
	})

	t.Run("build currency information with native asset", func(t *testing.T) {
		data.DeleteAllAssetFixtures(t, ctx, dbConnectionPool)
		asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "XLM", "")

		currencyInformation := s.buildCurrencyInformation([]data.Asset{*asset})
		wantStr := `
		[[CURRENCIES]]
		code = "native"
		status = "live"
		is_asset_anchored = true
		anchor_asset_type = "crypto"
		desc = "XLM, the native token of the Stellar Network."
	`

		assert.Equal(t, wantStr, currencyInformation)
	})

	t.Run("build currency information with multiple assets", func(t *testing.T) {
		assets := data.ClearAndCreateAssetFixtures(t, ctx, dbConnectionPool)
		xlm := data.CreateAssetFixture(t, ctx, dbConnectionPool, "XLM", "")

		currencyInformation := s.buildCurrencyInformation(append(assets, *xlm))
		wantStr := `
		[[CURRENCIES]]
		code = "EURT"
		issuer = "GA62MH5RDXFWAIWHQEFNMO2SVDDCQLWOO3GO36VQB5LHUXL22DQ6IQAU"
		is_asset_anchored = true
		anchor_asset_type = "fiat"
		status = "live"
		desc = "EURT"
	
		[[CURRENCIES]]
		code = "USDC"
		issuer = "GABC65XJDMXTGPNZRCI6V3KOKKWVK55UEKGQLONRIVYPMEJNNQ45YOEE"
		is_asset_anchored = true
		anchor_asset_type = "fiat"
		status = "live"
		desc = "USDC"
	
		[[CURRENCIES]]
		code = "native"
		status = "live"
		is_asset_anchored = true
		anchor_asset_type = "crypto"
		desc = "XLM, the native token of the Stellar Network."
	`

		assert.Equal(t, wantStr, currencyInformation)
	})
}

func Test_StellarTomlHandler_ServeHTTP(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	ctx := context.Background()

	data.ClearAndCreateAssetFixtures(t, ctx, dbConnectionPool)

	t.Run("build testnet toml", func(t *testing.T) {
		tomlHandler := StellarTomlHandler{
			DistributionPublicKey:    "GBC2HVWFIFN7WJHFORVBCDKJORG6LWTW3O2QBHOURL3KHZPM4KMWTUSA",
			NetworkPassphrase:        network.TestNetworkPassphrase,
			Sep10SigningPublicKey:    "GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S",
			AnchorPlatformBaseSepURL: "https://anchor-platform-domain",
			Models:                   models,
		}

		r := chi.NewRouter()
		r.Get("/.well-known/stellar.toml", tomlHandler.ServeHTTP)

		req, err := http.NewRequest("GET", "/.well-known/stellar.toml", nil)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		wantToml := fmt.Sprintf(`
			ACCOUNTS=["GBC2HVWFIFN7WJHFORVBCDKJORG6LWTW3O2QBHOURL3KHZPM4KMWTUSA", "GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S"]
			SIGNING_KEY="GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S"
			NETWORK_PASSPHRASE=%q
			HORIZON_URL=%q
			WEB_AUTH_ENDPOINT="https://anchor-platform-domain/auth"
			TRANSFER_SERVER_SEP0024="https://anchor-platform-domain/sep24"

			[DOCUMENTATION]
			ORG_NAME="MyCustomAid"

			[[CURRENCIES]]
			code = "EURT"
			issuer = "GA62MH5RDXFWAIWHQEFNMO2SVDDCQLWOO3GO36VQB5LHUXL22DQ6IQAU"
			is_asset_anchored = true
			anchor_asset_type = "fiat"
			status = "live"
			desc = "EURT"

			[[CURRENCIES]]
			code = "USDC"
			issuer = "GABC65XJDMXTGPNZRCI6V3KOKKWVK55UEKGQLONRIVYPMEJNNQ45YOEE"
			is_asset_anchored = true
			anchor_asset_type = "fiat"
			status = "live"
			desc = "USDC"
		`, network.TestNetworkPassphrase, horizonTestnetURL)
		wantToml = strings.TrimSpace(wantToml)
		wantToml = strings.ReplaceAll(wantToml, "\t", "")
		assert.Equal(t, wantToml, rr.Body.String())
	})

	t.Run("build pubnet toml", func(t *testing.T) {
		tomlHandler := StellarTomlHandler{
			DistributionPublicKey:    "GBC2HVWFIFN7WJHFORVBCDKJORG6LWTW3O2QBHOURL3KHZPM4KMWTUSA",
			NetworkPassphrase:        network.PublicNetworkPassphrase,
			Sep10SigningPublicKey:    "GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S",
			AnchorPlatformBaseSepURL: "https://anchor-platform-domain",
			Models:                   models,
		}

		r := chi.NewRouter()
		r.Get("/.well-known/stellar.toml", tomlHandler.ServeHTTP)

		req, err := http.NewRequest("GET", "/.well-known/stellar.toml", nil)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		wantToml := fmt.Sprintf(`
			ACCOUNTS=["GBC2HVWFIFN7WJHFORVBCDKJORG6LWTW3O2QBHOURL3KHZPM4KMWTUSA", "GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S"]
			SIGNING_KEY="GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S"
			NETWORK_PASSPHRASE=%q
			HORIZON_URL=%q
			WEB_AUTH_ENDPOINT="https://anchor-platform-domain/auth"
			TRANSFER_SERVER_SEP0024="https://anchor-platform-domain/sep24"

			[DOCUMENTATION]
			ORG_NAME="MyCustomAid"

			[[CURRENCIES]]
			code = "EURT"
			issuer = "GA62MH5RDXFWAIWHQEFNMO2SVDDCQLWOO3GO36VQB5LHUXL22DQ6IQAU"
			is_asset_anchored = true
			anchor_asset_type = "fiat"
			status = "live"
			desc = "EURT"

			[[CURRENCIES]]
			code = "USDC"
			issuer = "GABC65XJDMXTGPNZRCI6V3KOKKWVK55UEKGQLONRIVYPMEJNNQ45YOEE"
			is_asset_anchored = true
			anchor_asset_type = "fiat"
			status = "live"
			desc = "USDC"
		`, network.PublicNetworkPassphrase, horizonPubnetURL)
		wantToml = strings.TrimSpace(wantToml)
		wantToml = strings.ReplaceAll(wantToml, "\t", "")
		assert.Equal(t, wantToml, rr.Body.String())
	})

	t.Run("build toml without assets in database", func(t *testing.T) {
		data.DeleteAllAssetFixtures(t, ctx, dbConnectionPool)

		tomlHandler := StellarTomlHandler{
			DistributionPublicKey:    "GBC2HVWFIFN7WJHFORVBCDKJORG6LWTW3O2QBHOURL3KHZPM4KMWTUSA",
			NetworkPassphrase:        network.PublicNetworkPassphrase,
			Sep10SigningPublicKey:    "GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S",
			AnchorPlatformBaseSepURL: "https://anchor-platform-domain",
			Models:                   models,
		}

		r := chi.NewRouter()
		r.Get("/.well-known/stellar.toml", tomlHandler.ServeHTTP)

		req, err := http.NewRequest("GET", "/.well-known/stellar.toml", nil)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		wantToml := fmt.Sprintf(`
			ACCOUNTS=["GBC2HVWFIFN7WJHFORVBCDKJORG6LWTW3O2QBHOURL3KHZPM4KMWTUSA", "GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S"]
			SIGNING_KEY="GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S"
			NETWORK_PASSPHRASE=%q
			HORIZON_URL=%q
			WEB_AUTH_ENDPOINT="https://anchor-platform-domain/auth"
			TRANSFER_SERVER_SEP0024="https://anchor-platform-domain/sep24"

			[DOCUMENTATION]
			ORG_NAME="MyCustomAid"
		`, network.PublicNetworkPassphrase, horizonPubnetURL)
		wantToml = strings.TrimSpace(wantToml)
		wantToml = strings.ReplaceAll(wantToml, "\t", "")
		assert.Equal(t, wantToml, rr.Body.String())
	})
}
