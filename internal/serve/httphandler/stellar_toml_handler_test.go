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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/assets"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	sigMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
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
	req := httptest.NewRequest("GET", "https://test.com/.well-known/stellar.toml", nil)
	req.Host = "test.com"
	tenantDistAccPublicKey := "GDEWLTJMGKABNF3GBA3VTVBYPES3FXQHHJVJVI6X3CRKKFH5EMLRT5JZ"
	distAccount := schema.NewDefaultStellarTransactionAccount(tenantDistAccPublicKey)

	// Create a tenant with BaseURL for testing
	testTenant := &schema.Tenant{
		ID:      "test-tenant-id",
		Name:    "test-tenant",
		BaseURL: func() *string { s := "https://tenant.example.com"; return &s }(),
	}

	testCases := []struct {
		name              string
		isTenantInContext bool
		tenantInContext   *schema.Tenant
		s                 StellarTomlHandler
		wantLines         []string
	}{
		{
			name:              "pubnet with anchor platform enabled (without tenant in context)",
			isTenantInContext: false,
			s: StellarTomlHandler{
				// DistributionAccountResolver: <---- this is being injected in the test below
				NetworkPassphrase:     network.PublicNetworkPassphrase,
				Sep10SigningPublicKey: "GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S",
				BaseURL:               "https://sdp-domain",
			},
			wantLines: []string{
				`ACCOUNTS=["GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S"]`,
				`SIGNING_KEY="GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S"`,
				fmt.Sprintf("NETWORK_PASSPHRASE=%q", network.PublicNetworkPassphrase),
				fmt.Sprintf("HORIZON_URL=%q", horizonPubnetURL),
				`WEB_AUTH_ENDPOINT="https://test.com/auth"`,
				`TRANSFER_SERVER_SEP0024="https://test.com/sep24"`,
			},
		},
		{
			name:              "pubnet with anchor platform disabled (without tenant in context)",
			isTenantInContext: false,
			s: StellarTomlHandler{
				// DistributionAccountResolver: <---- this is being injected in the test below
				NetworkPassphrase:     network.PublicNetworkPassphrase,
				Sep10SigningPublicKey: "GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S",
				BaseURL:               "https://sdp-domain",
			},
			wantLines: []string{
				`ACCOUNTS=["GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S"]`,
				`SIGNING_KEY="GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S"`,
				fmt.Sprintf("NETWORK_PASSPHRASE=%q", network.PublicNetworkPassphrase),
				fmt.Sprintf("HORIZON_URL=%q", horizonPubnetURL),
				`WEB_AUTH_ENDPOINT="https://test.com/auth"`,
				`TRANSFER_SERVER_SEP0024="https://test.com/sep24"`,
			},
		},
		{
			name:              "pubnet with anchor platform enabled (with tenant in context)",
			isTenantInContext: true,
			tenantInContext:   testTenant,
			s: StellarTomlHandler{
				// DistributionAccountResolver: <---- this is being injected in the test below
				NetworkPassphrase:     network.PublicNetworkPassphrase,
				Sep10SigningPublicKey: "GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S",
				BaseURL:               "https://sdp-domain",
			},
			wantLines: []string{
				fmt.Sprintf(`ACCOUNTS=[%q, "GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S"]`, tenantDistAccPublicKey),
				`SIGNING_KEY="GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S"`,
				fmt.Sprintf("NETWORK_PASSPHRASE=%q", network.PublicNetworkPassphrase),
				fmt.Sprintf("HORIZON_URL=%q", horizonPubnetURL),
				`WEB_AUTH_ENDPOINT="https://tenant.example.com/auth"`,
				`TRANSFER_SERVER_SEP0024="https://tenant.example.com/sep24"`,
			},
		},
		{
			name:              "pubnet with anchor platform disabled (with tenant in context)",
			isTenantInContext: true,
			s: StellarTomlHandler{
				// DistributionAccountResolver: <---- this is being injected in the test below
				NetworkPassphrase:     network.PublicNetworkPassphrase,
				Sep10SigningPublicKey: "GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S",
				BaseURL:               "https://sdp-domain",
			},
			wantLines: []string{
				fmt.Sprintf(`ACCOUNTS=[%q, "GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S"]`, tenantDistAccPublicKey),
				`SIGNING_KEY="GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S"`,
				fmt.Sprintf("NETWORK_PASSPHRASE=%q", network.PublicNetworkPassphrase),
				fmt.Sprintf("HORIZON_URL=%q", horizonPubnetURL),
				`WEB_AUTH_ENDPOINT="https://tenant.example.com/auth"`,
				`TRANSFER_SERVER_SEP0024="https://tenant.example.com/sep24"`,
			},
		},
		{
			name:              "testnet with anchor platform enabled (without tenant in context)",
			isTenantInContext: false,
			s: StellarTomlHandler{
				// DistributionAccountResolver: <---- this is being injected in the test below
				NetworkPassphrase:     network.TestNetworkPassphrase,
				Sep10SigningPublicKey: "GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S",
				BaseURL:               "https://sdp-domain",
			},
			wantLines: []string{
				`ACCOUNTS=["GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S"]`,
				`SIGNING_KEY="GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S"`,
				fmt.Sprintf("NETWORK_PASSPHRASE=%q", network.TestNetworkPassphrase),
				fmt.Sprintf("HORIZON_URL=%q", horizonTestnetURL),
				`WEB_AUTH_ENDPOINT="https://test.com/auth"`,
				`TRANSFER_SERVER_SEP0024="https://test.com/sep24"`,
			},
		},
		{
			name:              "testnet with anchor platform disabled (without tenant in context)",
			isTenantInContext: false,
			s: StellarTomlHandler{
				// DistributionAccountResolver: <---- this is being injected in the test below
				NetworkPassphrase:     network.TestNetworkPassphrase,
				Sep10SigningPublicKey: "GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S",
				BaseURL:               "https://sdp-domain",
			},
			wantLines: []string{
				`ACCOUNTS=["GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S"]`,
				`SIGNING_KEY="GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S"`,
				fmt.Sprintf("NETWORK_PASSPHRASE=%q", network.TestNetworkPassphrase),
				fmt.Sprintf("HORIZON_URL=%q", horizonTestnetURL),
				`WEB_AUTH_ENDPOINT="https://test.com/auth"`,
				`TRANSFER_SERVER_SEP0024="https://test.com/sep24"`,
			},
		},
		{
			name:              "testnet with anchor platform enabled (with tenant in context)",
			isTenantInContext: true,
			tenantInContext:   testTenant,
			s: StellarTomlHandler{
				// DistributionAccountResolver: <---- this is being injected in the test below
				NetworkPassphrase:     network.TestNetworkPassphrase,
				Sep10SigningPublicKey: "GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S",
				BaseURL:               "https://sdp-domain",
			},
			wantLines: []string{
				fmt.Sprintf(`ACCOUNTS=[%q, "GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S"]`, tenantDistAccPublicKey),
				`SIGNING_KEY="GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S"`,
				fmt.Sprintf("NETWORK_PASSPHRASE=%q", network.TestNetworkPassphrase),
				fmt.Sprintf("HORIZON_URL=%q", horizonTestnetURL),
				`WEB_AUTH_ENDPOINT="https://tenant.example.com/auth"`,
				`TRANSFER_SERVER_SEP0024="https://tenant.example.com/sep24"`,
			},
		},
		{
			name:              "testnet with anchor platform disabled (with tenant in context)",
			isTenantInContext: true,
			s: StellarTomlHandler{
				// DistributionAccountResolver: <---- this is being injected in the test below
				NetworkPassphrase:     network.TestNetworkPassphrase,
				Sep10SigningPublicKey: "GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S",
				BaseURL:               "https://sdp-domain",
			},
			wantLines: []string{
				fmt.Sprintf(`ACCOUNTS=[%q, "GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S"]`, tenantDistAccPublicKey),
				`SIGNING_KEY="GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S"`,
				fmt.Sprintf("NETWORK_PASSPHRASE=%q", network.TestNetworkPassphrase),
				fmt.Sprintf("HORIZON_URL=%q", horizonTestnetURL),
				`WEB_AUTH_ENDPOINT="https://tenant.example.com/auth"`,
				`TRANSFER_SERVER_SEP0024="https://tenant.example.com/sep24"`,
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()

			// Set up tenant context if needed
			if tc.isTenantInContext {
				if tc.tenantInContext != nil {
					ctx = sdpcontext.SetTenantInContext(ctx, tc.tenantInContext)
				} else {
					tenantBaseURL := "https://tenant.example.com"
					mockTenant := &schema.Tenant{
						BaseURL: &tenantBaseURL,
					}
					ctx = sdpcontext.SetTenantInContext(ctx, mockTenant)
				}
			}

			// Prepare mock
			mDistAccResolver := sigMocks.NewMockDistributionAccountResolver(t)
			if tc.isTenantInContext {
				mDistAccResolver.
					On("DistributionAccountFromContext", ctx).
					Return(distAccount, nil).
					Once()
			} else {
				mDistAccResolver.
					On("DistributionAccountFromContext", ctx).
					Return(schema.TransactionAccount{}, sdpcontext.ErrTenantNotFoundInContext).
					Once()
			}
			tc.s.DistributionAccountResolver = mDistAccResolver

			generalInformation := tc.s.buildGeneralInformation(ctx, req)
			generalInformation = strings.TrimSpace(generalInformation)
			generalInformation = strings.ReplaceAll(generalInformation, "\t", "")

			generalInformationLines := strings.Split(generalInformation, "\n")
			assert.Equal(t, len(tc.wantLines), len(generalInformationLines))
			assert.ElementsMatch(t, tc.wantLines, generalInformationLines)
		})
	}
}

func Test_StellarTomlHandler_buildOrganizationDocumentation(t *testing.T) {
	stellarTomlHandler := StellarTomlHandler{}
	testCases := []struct {
		name string
		want string
	}{
		{
			name: "FOO Org",
			want: `
		[DOCUMENTATION]
		ORG_NAME="FOO Org"
	`,
		},
		{
			name: "BAR Org",
			want: `
		[DOCUMENTATION]
		ORG_NAME="BAR Org"
	`,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			orgInformation := stellarTomlHandler.buildOrganizationDocumentation(tc.name)
			assert.Equal(t, tc.want, orgInformation)
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

	tenant, ctx := tenant.LoadDefaultTenantInContext(t, dbConnectionPool)
	data.ClearAndCreateAssetFixtures(t, ctx, dbConnectionPool)

	// Update organization name to match test expectations
	orgUpdate := &data.OrganizationUpdate{Name: "AdeptusMinistorum"}
	err = models.Organizations.Update(ctx, orgUpdate)
	require.NoError(t, err)

	distAccResolver, err := signing.NewDistributionAccountResolver(signing.DistributionAccountResolverOptions{
		AdminDBConnectionPool:            dbConnectionPool,
		HostDistributionAccountPublicKey: "GCWFIKOB7FO6KTXUKZIPPPZ42UT2V7HVZD5STVROKVJVQU24FSP7OLZK",
	})
	require.NoError(t, err)

	t.Run("build testnet toml for org", func(t *testing.T) {
		tomlHandler := StellarTomlHandler{
			DistributionAccountResolver: distAccResolver,
			NetworkPassphrase:           network.TestNetworkPassphrase,
			Sep10SigningPublicKey:       "GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S",
			Models:                      models,
			BaseURL:                     "https://sdp-domain",
		}

		r := chi.NewRouter()
		r.Get("/.well-known/stellar.toml", tomlHandler.ServeHTTP)

		req, err := http.NewRequestWithContext(ctx, "GET", "/.well-known/stellar.toml", nil)
		require.NoError(t, err)
		req.Host = *tenant.BaseURL
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		wantToml := fmt.Sprintf(`
			ACCOUNTS=["GDIVVKL6QYF6C6K3C5PZZBQ2NQDLN2OSLMVIEQRHS6DZE7WRL33ZDNXL", "GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S"]
			SIGNING_KEY="GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S"
			NETWORK_PASSPHRASE=%q
			HORIZON_URL=%q
			WEB_AUTH_ENDPOINT="http://default-tenant.stellar.local:8000/auth"
			TRANSFER_SERVER_SEP0024="http://default-tenant.stellar.local:8000/sep24"

			[DOCUMENTATION]
			ORG_NAME="AdeptusMinistorum"

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
			DistributionAccountResolver: distAccResolver,
			NetworkPassphrase:           network.PublicNetworkPassphrase,
			Sep10SigningPublicKey:       "GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S",
			Models:                      models,
			BaseURL:                     "https://sdp-domain",
		}

		r := chi.NewRouter()
		r.Get("/.well-known/stellar.toml", tomlHandler.ServeHTTP)

		req, err := http.NewRequestWithContext(ctx, "GET", "/.well-known/stellar.toml", nil)
		require.NoError(t, err)
		req.Host = *tenant.BaseURL
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		wantToml := fmt.Sprintf(`
			ACCOUNTS=["GDIVVKL6QYF6C6K3C5PZZBQ2NQDLN2OSLMVIEQRHS6DZE7WRL33ZDNXL", "GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S"]
			SIGNING_KEY="GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S"
			NETWORK_PASSPHRASE=%q
			HORIZON_URL=%q
			WEB_AUTH_ENDPOINT="http://default-tenant.stellar.local:8000/auth"
			TRANSFER_SERVER_SEP0024="http://default-tenant.stellar.local:8000/sep24"

			[DOCUMENTATION]
			ORG_NAME="AdeptusMinistorum"

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

	t.Run("build general pubnet toml for instance", func(t *testing.T) {
		tomlHandler := StellarTomlHandler{
			DistributionAccountResolver: distAccResolver,
			NetworkPassphrase:           network.PublicNetworkPassphrase,
			Sep10SigningPublicKey:       "GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S",
			Models:                      models,
			InstanceName:                "SDP Pubnet",
			BaseURL:                     "https://sdp-domain",
		}

		r := chi.NewRouter()
		r.Get("/.well-known/stellar.toml", tomlHandler.ServeHTTP)

		req, err := http.NewRequest("GET", "/.well-known/stellar.toml", nil)
		require.NoError(t, err)
		req.Host = "instance.example.com" // Set host for request
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		wantToml := fmt.Sprintf(`
			ACCOUNTS=["GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S"]
			SIGNING_KEY="GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S"
			NETWORK_PASSPHRASE=%q
			HORIZON_URL=%q
			WEB_AUTH_ENDPOINT="https://instance.example.com/auth"
			TRANSFER_SERVER_SEP0024="https://instance.example.com/sep24"

			[DOCUMENTATION]
			ORG_NAME="SDP Pubnet"

			[[CURRENCIES]]
			code = %q
			issuer = %q
			is_asset_anchored = true
			anchor_asset_type = "fiat"
			status = "live"
			desc = %q

			[[CURRENCIES]]
			code = %q
			issuer = %q
			is_asset_anchored = true
			anchor_asset_type = "fiat"
			status = "live"
			desc = %q

			[[CURRENCIES]]
			code = "native"
			status = "live"
			is_asset_anchored = true
			anchor_asset_type = "crypto"
			desc = "XLM, the native token of the Stellar Network."
		`,
			network.PublicNetworkPassphrase, horizonPubnetURL,
			assets.EURCAssetCode, assets.EURCAssetIssuerPubnet, assets.EURCAssetCode,
			assets.USDCAssetCode, assets.USDCAssetIssuerPubnet, assets.USDCAssetCode)
		wantToml = strings.TrimSpace(wantToml)
		wantToml = strings.ReplaceAll(wantToml, "\t", "")
		assert.Equal(t, wantToml, rr.Body.String())
	})

	t.Run("build toml without assets in database", func(t *testing.T) {
		data.DeleteAllAssetFixtures(t, ctx, dbConnectionPool)

		tomlHandler := StellarTomlHandler{
			DistributionAccountResolver: distAccResolver,
			NetworkPassphrase:           network.PublicNetworkPassphrase,
			Sep10SigningPublicKey:       "GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S",
			Models:                      models,
			BaseURL:                     "https://sdp-domain",
		}

		r := chi.NewRouter()
		r.Get("/.well-known/stellar.toml", tomlHandler.ServeHTTP)

		req, err := http.NewRequestWithContext(ctx, "GET", "/.well-known/stellar.toml", nil)
		require.NoError(t, err)
		req.Host = *tenant.BaseURL
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		wantToml := fmt.Sprintf(`
			ACCOUNTS=["GDIVVKL6QYF6C6K3C5PZZBQ2NQDLN2OSLMVIEQRHS6DZE7WRL33ZDNXL", "GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S"]
			SIGNING_KEY="GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S"
			NETWORK_PASSPHRASE=%q
			HORIZON_URL=%q
			WEB_AUTH_ENDPOINT="http://default-tenant.stellar.local:8000/auth"
			TRANSFER_SERVER_SEP0024="http://default-tenant.stellar.local:8000/sep24"

			[DOCUMENTATION]
			ORG_NAME="AdeptusMinistorum"
		`, network.PublicNetworkPassphrase, horizonPubnetURL)
		wantToml = strings.TrimSpace(wantToml)
		wantToml = strings.ReplaceAll(wantToml, "\t", "")
		assert.Equal(t, wantToml, rr.Body.String())
	})

	t.Run("build toml with anchor platform disabled (SDP native URLs)", func(t *testing.T) {
		data.ClearAndCreateAssetFixtures(t, ctx, dbConnectionPool)

		tomlHandler := StellarTomlHandler{
			DistributionAccountResolver: distAccResolver,
			NetworkPassphrase:           network.PublicNetworkPassphrase,
			Sep10SigningPublicKey:       "GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S",
			Models:                      models,
			BaseURL:                     "https://sdp-domain",
		}

		r := chi.NewRouter()
		r.Get("/.well-known/stellar.toml", tomlHandler.ServeHTTP)

		req, err := http.NewRequestWithContext(ctx, "GET", "/.well-known/stellar.toml", nil)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		wantToml := fmt.Sprintf(`
			ACCOUNTS=["GDIVVKL6QYF6C6K3C5PZZBQ2NQDLN2OSLMVIEQRHS6DZE7WRL33ZDNXL", "GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S"]
			SIGNING_KEY="GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S"
			NETWORK_PASSPHRASE=%q
			HORIZON_URL=%q
			WEB_AUTH_ENDPOINT="http://default-tenant.stellar.local:8000/auth"
			TRANSFER_SERVER_SEP0024="http://default-tenant.stellar.local:8000/sep24"

			[DOCUMENTATION]
			ORG_NAME="AdeptusMinistorum"

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
}
