package httphandler

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/stellar/go/network"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
)

type StellarTomlHandler struct {
	AnchorPlatformBaseSepURL string
	DistributionPublicKey    string
	NetworkPassphrase        string
	Models                   *data.Models
	Sep10SigningPublicKey    string
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
func (s *StellarTomlHandler) buildGeneralInformation() string {
	webAuthEndpoint := s.AnchorPlatformBaseSepURL + "/auth"
	transferServerSep0024 := s.AnchorPlatformBaseSepURL + "/sep24"
	accounts := fmt.Sprintf("[%q, %q]", s.DistributionPublicKey, s.Sep10SigningPublicKey)

	return fmt.Sprintf(`
		ACCOUNTS=%s
		SIGNING_KEY=%q
		NETWORK_PASSPHRASE=%q
		HORIZON_URL=%q
		WEB_AUTH_ENDPOINT=%q
		TRANSFER_SERVER_SEP0024=%q
	`, accounts, s.Sep10SigningPublicKey, s.NetworkPassphrase, s.horizonURL(), webAuthEndpoint, transferServerSep0024)
}

func (s *StellarTomlHandler) buildOrganizationDocumentation(organization data.Organization) string {
	return fmt.Sprintf(`
		[DOCUMENTATION]
		ORG_NAME=%q
	`, organization.Name)
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
	assets, err := s.Models.Assets.GetAll(r.Context())
	ctx := r.Context()
	if err != nil {
		httperror.InternalError(ctx, "Cannot retrieve assets", err, nil).Render(w)
		return
	}

	organization, err := s.Models.Organizations.Get(r.Context())
	if err != nil {
		httperror.InternalError(ctx, "Cannot retrieve organization", err, nil).Render(w)
		return
	}

	stellarToml := s.buildGeneralInformation() + s.buildOrganizationDocumentation(*organization) + s.buildCurrencyInformation(assets)
	stellarToml = strings.TrimSpace(stellarToml)
	stellarToml = strings.ReplaceAll(stellarToml, "\t", "")

	_, err = fmt.Fprint(w, stellarToml)
	if err != nil {
		httperror.InternalError(ctx, "Cannot write stellar.toml content", err, nil).Render(w)
		return
	}
}
