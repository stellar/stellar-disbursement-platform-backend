package httphandler

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/stellar/go/network"
	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

type StellarTomlHandler struct {
	AnchorPlatformBaseSepURL    string
	DistributionAccountResolver signing.DistributionAccountResolver
	NetworkPassphrase           string
	Models                      *data.Models
	Sep10SigningPublicKey       string
	InstanceName                string
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
	distributionPublicKey, err := s.DistributionAccountResolver.DistributionAccountFromContext(ctx)
	if err != nil {
		log.Warnf("Couldn't get distribution account from context in %s%s", req.Host, req.URL.Path)
		distributionPublicKey = s.DistributionAccountResolver.HostDistributionAccount()
	}

	webAuthEndpoint := s.AnchorPlatformBaseSepURL + "/auth"
	transferServerSep0024 := s.AnchorPlatformBaseSepURL + "/sep24"
	accounts := fmt.Sprintf("[%q, %q]", distributionPublicKey, s.Sep10SigningPublicKey)

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
	_, err := tenant.GetTenantFromContext(ctx)
	if err != nil {
		// return a general stellar.toml file for this instance because no tenant is present.
		networkType, innerErr := utils.GetNetworkTypeFromNetworkPassphrase(s.NetworkPassphrase)
		if innerErr != nil {
			httperror.InternalError(ctx, "Couldn't generate stellar.toml file for this instance", innerErr, nil).Render(w)
			return
		}
		instanceAssets := services.DefaultAssetsNetworkMap[networkType]
		stellarToml = s.buildGeneralInformation(ctx, r) + s.buildOrganizationDocumentation(s.InstanceName) + s.buildCurrencyInformation(instanceAssets)
	} else {
		// return a stellar.toml file for this tenant.
		organization, innerErr := s.Models.Organizations.Get(r.Context())
		if innerErr != nil {
			httperror.InternalError(ctx, "Cannot retrieve organization", err, nil).Render(w)
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
