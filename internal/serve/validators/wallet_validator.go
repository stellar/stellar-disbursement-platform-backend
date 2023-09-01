package validators

import (
	"net/url"
	"strings"
	"github.com/stellar/go/support/log"
)

type CreateWalletRequest struct {
	Name              string   `json:"name"`
	Homepage          string   `json:"homepage"`
	DeepLinkSchema    string   `json:"deep_link_schema"`
	SEP10ClientDomain string   `json:"sep_10_client_domain"`
	AssetsIDs         []string `json:"assets_ids"`
	CountriesCodes    []string `json:"countries_codes"`
}

type WalletValidator struct {
	*Validator
}

func NewWalletValidator() *WalletValidator {
	return &WalletValidator{Validator: NewValidator()}
}

func (wv *WalletValidator) ValidateCreateWalletRequest(reqBody *CreateWalletRequest) {
	wv.Check(reqBody != nil, "body", "request body is empty")

	if wv.HasErrors() {
		return
	}

	name := strings.TrimSpace(reqBody.Name)
	homepage := strings.TrimSpace(reqBody.Homepage)
	deepLinkSchema := strings.TrimSpace(reqBody.DeepLinkSchema)
	sep10ClientDomain := strings.TrimSpace(reqBody.SEP10ClientDomain)

	wv.Check(name != "", "name", "name is required")
	wv.Check(homepage != "", "homepage", "homepage is required")
	wv.Check(deepLinkSchema != "", "deep_link_schema", "deep_link_schema is required")
	wv.Check(sep10ClientDomain != "", "sep_10_client_domain", "sep_10_client_domain is required")
	wv.Check(len(reqBody.AssetsIDs) != 0, "assets_ids", "provide at least one asset ID")
	wv.Check(len(reqBody.CountriesCodes) != 0, "countries_codes", "provide at least one country code")

	if wv.HasErrors() {
		return
	}

	homepageURL, err := url.ParseRequestURI(homepage)
	if err != nil {
		log.Errorf("parsing homepage URL: %v", err)
		wv.Check(false, "homepage", "invalid homepage URL provided")
	}

	deepLinkSchemaURL, err := url.ParseRequestURI(deepLinkSchema)
	if err != nil {
		log.Errorf("parsing deep link schema: %v", err)
		wv.Check(false, "deep_link_schema", "invalid deep link schema provided")
	}

	sep10URL, err := url.Parse(sep10ClientDomain)
	if err != nil {
		log.Errorf("parsing SEP-10 client domain URL: %v", err)
		wv.Check(false, "sep_10_client_domain", "invalid SEP-10 client domain URL provided")
	}

	if wv.HasErrors() {
		return
	}

	sep10Host := sep10URL.Host
	if sep10Host == "" {
		sep10Host = sep10URL.String()
	}

	reqBody.Name = name
	reqBody.Homepage = homepageURL.String()
	reqBody.DeepLinkSchema = deepLinkSchemaURL.String()
	reqBody.SEP10ClientDomain = sep10Host
}
