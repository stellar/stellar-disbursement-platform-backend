package validators

import (
	"net/url"
	"strings"

	"github.com/stellar/go/support/log"
)

type WalletRequest struct {
	Name              string   `json:"name"`
	Homepage          string   `json:"homepage"`
	DeepLinkSchema    string   `json:"deep_link_schema"`
	SEP10ClientDomain string   `json:"sep_10_client_domain"`
	AssetsIDs         []string `json:"assets_ids"`
}

type WalletValidator struct {
	*Validator
}

func NewWalletValidator() *WalletValidator {
	return &WalletValidator{Validator: NewValidator()}
}

func (wv *WalletValidator) ValidateCreateWalletRequest(reqBody *WalletRequest) *WalletRequest {
	wv.Check(reqBody != nil, "body", "request body is empty")

	if wv.HasErrors() {
		return nil
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

	if wv.HasErrors() {
		return nil
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
		return nil
	}

	sep10Host := sep10URL.Host
	if sep10Host == "" {
		sep10Host = sep10URL.String()
	}

	modifiedReq := &WalletRequest{
		Name:              name,
		Homepage:          homepageURL.String(),
		DeepLinkSchema:    deepLinkSchemaURL.String(),
		SEP10ClientDomain: sep10Host,
		AssetsIDs:         reqBody.AssetsIDs,
	}

	return modifiedReq
}
