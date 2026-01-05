package httphandler

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/stellar/go-stellar-sdk/network"
	"github.com/stellar/go-stellar-sdk/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

type StellarTomlHandler struct {
	Models                      *data.Models
	DistributionAccountResolver signing.DistributionAccountResolver
	NetworkPassphrase           string
	Sep10SigningPublicKey       string
	InstanceName                string
	BaseURL                     string
}

const (
	horizonPubnetURL  = "https://horizon.stellar.org"
	horizonTestnetURL = "https://horizon-testnet.stellar.org"
)

func (s *StellarTomlHandler) horizonURL() string {
	if s.NetworkPassphrase == network.PublicNetworkPassphrase {
		return horizonPubnetURL
	}
	return horizonTestnetURL
}

// buildGeneralInformation will create the general informations based on the env vars injected into the handler.
func (s *StellarTomlHandler) buildGeneralInformation(ctx context.Context, req *http.Request) string {
	accounts := fmt.Sprintf("[%q]", s.Sep10SigningPublicKey)
	if perTenantDistributionAccount, err := s.DistributionAccountResolver.DistributionAccountFromContext(ctx); err != nil {
		log.Ctx(ctx).Warnf("Couldn't get distribution account from context in %s%s", req.Host, req.URL.Path)
	} else if perTenantDistributionAccount.IsStellar() {
		accounts = fmt.Sprintf("[%q, %q]", perTenantDistributionAccount.Address, s.Sep10SigningPublicKey)
	}

	var webAuthEndpoint, transferServerSep0024 string

	parsedBaseURL, err := url.Parse(s.BaseURL)
	if err != nil {
		log.Ctx(ctx).Warnf("Invalid environment BaseURL %s: %v", s.BaseURL, err)
		parsedBaseURL = &url.URL{Scheme: "https"}
	}

	t, err := sdpcontext.GetTenantFromContext(ctx)
	if err != nil {
		webAuthEndpoint = fmt.Sprintf("%s://%s/sep10/auth", parsedBaseURL.Scheme, req.Host)
		transferServerSep0024 = fmt.Sprintf("%s://%s/sep24", parsedBaseURL.Scheme, req.Host)
	} else {
		webAuthEndpoint = *t.BaseURL + "/sep10/auth"
		transferServerSep0024 = *t.BaseURL + "/sep24"
	}

	return fmt.Sprintf(`
		ACCOUNTS=%s
		SIGNING_KEY=%q
		NETWORK_PASSPHRASE=%q
		HORIZON_URL=%q
		WEB_AUTH_ENDPOINT=%q
		TRANSFER_SERVER_SEP0024=%q
	`, accounts, s.Sep10SigningPublicKey, s.NetworkPassphrase, s.horizonURL(), webAuthEndpoint, transferServerSep0024)
}

func (s *StellarTomlHandler) buildOrganizationDocumentation(instanceName string) string {
	return fmt.Sprintf(`
		[DOCUMENTATION]
		ORG_NAME=%q
	`, instanceName)
}

// buildCurrencyInformation will create the currency information for all assets register in the application.
func (s *StellarTomlHandler) buildCurrencyInformation(assets []data.Asset) string {
	strAssets := ""
	for _, asset := range assets {
		if asset.Code != "XLM" {
			strAssets += fmt.Sprintf(`
		[[CURRENCIES]]
		code = %q
		issuer = %q
		is_asset_anchored = true
		anchor_asset_type = "fiat"
		status = "live"
		desc = "%s"
	`, asset.Code, asset.Issuer, asset.Code)
		} else {
			strAssets += `
		[[CURRENCIES]]
		code = "native"
		status = "live"
		is_asset_anchored = true
		anchor_asset_type = "crypto"
		desc = "XLM, the native token of the Stellar Network."
	`
		}
	}

	return strAssets
}

// ServeHTTP will serve the stellar.toml file needed to register users through SEP-24.
func (s StellarTomlHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var stellarToml string
	_, err := sdpcontext.GetTenantFromContext(ctx)
	if err != nil {
		// return a general stellar.toml file for this instance because no tenant is present.
		networkType, innerErr := utils.GetNetworkTypeFromNetworkPassphrase(s.NetworkPassphrase)
		if innerErr != nil {
			httperror.InternalError(ctx, "Couldn't generate stellar.toml file for this instance", innerErr, nil).Render(w)
			return
		}
		instanceAssets := services.StellarAssetsNetworkMap[networkType]
		stellarToml = s.buildGeneralInformation(ctx, r) + s.buildOrganizationDocumentation(s.InstanceName) + s.buildCurrencyInformation(instanceAssets)
	} else {
		// return a stellar.toml file for this tenant.
		organization, innerErr := s.Models.Organizations.Get(ctx)
		if innerErr != nil {
			httperror.InternalError(ctx, "Cannot retrieve organization", innerErr, nil).Render(w)
			return
		}

		assets, innerErr := s.Models.Assets.GetAll(ctx)
		if innerErr != nil {
			httperror.InternalError(ctx, "Cannot retrieve assets", innerErr, nil).Render(w)
			return
		}

		stellarToml = s.buildGeneralInformation(ctx, r) + s.buildOrganizationDocumentation(organization.Name) + s.buildCurrencyInformation(assets)
	}

	stellarToml = strings.TrimSpace(stellarToml)
	stellarToml = strings.ReplaceAll(stellarToml, "\t", "")

	_, err = fmt.Fprint(w, stellarToml)
	if err != nil {
		httperror.InternalError(ctx, "Cannot write stellar.toml content", err, nil).Render(w)
		return
	}
}
