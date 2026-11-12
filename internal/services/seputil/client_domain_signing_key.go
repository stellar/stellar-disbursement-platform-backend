package seputil

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"

	"github.com/stellar/go-stellar-sdk/keypair"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpclient"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

const stellarTomlMaxSize = 100 * 1024

const stellarTomlPath = "/.well-known/stellar.toml"

var (
	tomlTableHeaderRe = regexp.MustCompile(`(?m)^[ \t]*\[`)
	tomlSigningKeyRe  = regexp.MustCompile(`(?m)^[ \t]*(?:SIGNING_KEY|"SIGNING_KEY"|'SIGNING_KEY')[ \t]*=[ \t]*` +
		`(?:"""\r?\n?([^"\\\r\n]*)\r?\n?"""|"([^"\\\r\n]*)"|'''\r?\n?([^'\r\n]*)\r?\n?'''|'([^'\r\n]*)')` +
		`[ \t]*(?:#[^\r\n]*)?\r?$`)
)

// FetchClientDomainSigningKey returns the SIGNING_KEY published in the client domain's stellar.toml.
func FetchClientDomainSigningKey(ctx context.Context, httpClient httpclient.HTTPClientInterface, clientDomain string, allowHTTPRetry bool) (string, error) {
	if err := utils.ValidateDNS(clientDomain); err != nil {
		return "", fmt.Errorf("invalid client_domain: %w", err)
	}

	body, err := fetchStellarToml(ctx, httpClient, "https://"+clientDomain+stellarTomlPath)
	if err != nil && allowHTTPRetry {
		body, err = fetchStellarToml(ctx, httpClient, "http://"+clientDomain+stellarTomlPath)
	}
	if err != nil {
		return "", fmt.Errorf("fetching stellar.toml from %s: %w", clientDomain, err)
	}

	signingKey, ok := SigningKeyFromStellarToml(body)
	if !ok {
		return "", fmt.Errorf("SIGNING_KEY not present in client_domain stellar.toml")
	}
	if _, err = keypair.ParseAddress(signingKey); err != nil {
		return "", fmt.Errorf("SIGNING_KEY %s is not a valid Stellar account ID", signingKey)
	}
	return signingKey, nil
}

func fetchStellarToml(ctx context.Context, httpClient httpclient.HTTPClientInterface, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request errored: %w", err)
	}
	defer utils.DeferredClose(ctx, resp.Body, "closing stellar.toml response body")

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("http request failed with status code %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, stellarTomlMaxSize+1))
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}
	if len(body) > stellarTomlMaxSize {
		return nil, fmt.Errorf("stellar.toml response exceeds %d bytes limit", stellarTomlMaxSize)
	}
	return body, nil
}

// SigningKeyFromStellarToml returns the first top-level SIGNING_KEY of a stellar.toml body.
func SigningKeyFromStellarToml(body []byte) (string, bool) {
	body = bytes.TrimPrefix(body, []byte("\xef\xbb\xbf"))
	if loc := tomlTableHeaderRe.FindIndex(body); loc != nil {
		body = body[:loc[0]]
	}
	m := tomlSigningKeyRe.FindSubmatch(body)
	if m == nil {
		return "", false
	}
	for _, group := range m[1:] {
		if len(group) > 0 {
			return string(group), true
		}
	}
	return "", false
}
