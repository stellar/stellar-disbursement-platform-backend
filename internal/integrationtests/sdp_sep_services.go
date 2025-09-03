package integrationtests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/stellar/go/keypair"
	"github.com/stellar/go/txnbuild"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpclient"
)

type SDPSepServicesIntegrationTestsInterface interface {
	// SEP-10 Methods
	GetSEP10Challenge(ctx context.Context) (*SEP10ChallengeResponse, error)
	SignSEP10Challenge(challengeResp *SEP10ChallengeResponse) (*SignedSEP10Challenge, error)
	ValidateSEP10Challenge(ctx context.Context, signedChallenge *SignedSEP10Challenge) (*SEP10AuthToken, error)

	// SEP-24 Methods
	InitiateSEP24Deposit(ctx context.Context, sep10Token *SEP10AuthToken) (*SEP24DepositResponse, error)
	GetSEP24Transaction(ctx context.Context, sep10Token *SEP10AuthToken, transactionID string) (*SEP24TransactionStatus, error)
	CompleteReceiverRegistration(ctx context.Context, sep24Token string, registrationData *ReceiverRegistrationRequest) error
}

type SDPSepServicesIntegrationTests struct {
	HTTPClient                httpclient.HttpClientInterface
	SDPBaseURL                string
	TenantName                string
	ReceiverAccountPublicKey  string
	ReceiverAccountPrivateKey string
	ClientDomainPrivateKey    string // Private key for client domain signing
	Sep10SigningPublicKey     string
	DisbursedAssetCode        string
	ClientDomain              string // Required for internal SEP-10
	NetworkPassphrase         string
	SingleTenantMode          bool
	HomeDomain                string // Configurable home domain for SEP-10
}

type SEP10ChallengeResponse struct {
	Transaction       string `json:"transaction"`
	NetworkPassphrase string `json:"network_passphrase"`
	ParsedTx          *txnbuild.Transaction
}

type SignedSEP10Challenge struct {
	*SEP10ChallengeResponse
	SignedTransaction string
}

type SEP10AuthToken struct {
	Token string `json:"token"`
}

type SEP24DepositResponse struct {
	Type          string `json:"type"`
	URL           string `json:"url"`
	TransactionID string `json:"id"`
	Token         string // Extracted from URL
}

type SEP24TransactionStatus struct {
	Transaction struct {
		ID              string     `json:"id"`
		Kind            string     `json:"kind"`
		Status          string     `json:"status"`
		MoreInfoURL     string     `json:"more_info_url,omitempty"`
		To              string     `json:"to,omitempty"`
		DepositMemo     string     `json:"deposit_memo,omitempty"`
		DepositMemoType string     `json:"deposit_memo_type,omitempty"`
		StartedAt       time.Time  `json:"started_at"`
		CompletedAt     *time.Time `json:"completed_at,omitempty"`
	} `json:"transaction"`
}

type ReceiverRegistrationRequest struct {
	PhoneNumber       string `json:"phone_number,omitempty"`
	Email             string `json:"email,omitempty"`
	OTP               string `json:"otp"`
	VerificationValue string `json:"verification"`
	VerificationField string `json:"verification_field"`
	ReCAPTCHAToken    string `json:"recaptcha_token"`
}

// GetSEP10Challenge gets a challenge from SDP's internal SEP-10 service
func (s *SDPSepServicesIntegrationTests) GetSEP10Challenge(ctx context.Context) (*SEP10ChallengeResponse, error) {
	authURL, err := url.JoinPath(s.SDPBaseURL, "auth")
	if err != nil {
		return nil, fmt.Errorf("creating auth URL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, authURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	// Set tenant header for multi-tenant context
	if s.TenantName != "" {
		req.Header.Set("SDP-Tenant-Name", s.TenantName)
	}

	// Build query parameters
	q := req.URL.Query()
	q.Add("account", s.ReceiverAccountPublicKey)

	// Use configurable home domain if provided, otherwise use tenant-based domain
	homeDomain := s.HomeDomain
	if homeDomain == "" && s.TenantName != "" {
		// For multi-tenant mode, we need to use the full tenant domain for SEP-24 to work
		homeDomain = fmt.Sprintf("%s.stellar.local:8000", s.TenantName)
	}
	if homeDomain != "" {
		q.Add("home_domain", homeDomain)
	}

	// Add client_domain parameter - required for internal SEP
	if s.ClientDomain != "" {
		q.Add("client_domain", s.ClientDomain)
	}
	req.URL.RawQuery = q.Encode()

	resp, err := s.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var challenge SEP10ChallengeResponse
	if err = json.NewDecoder(resp.Body).Decode(&challenge); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	// Parse the transaction for signing
	parsedTx, err := txnbuild.TransactionFromXDR(challenge.Transaction)
	if err != nil {
		return nil, fmt.Errorf("parsing challenge transaction: %w", err)
	}

	tx, ok := parsedTx.Transaction()
	if !ok {
		return nil, fmt.Errorf("challenge is not a simple transaction")
	}
	challenge.ParsedTx = tx

	return &challenge, nil
}

// SignSEP10Challenge signs the challenge with receiver's private key AND client domain key
func (s *SDPSepServicesIntegrationTests) SignSEP10Challenge(challengeResp *SEP10ChallengeResponse) (*SignedSEP10Challenge, error) {
	// Sign with receiver's private key
	receiverKP, err := keypair.ParseFull(s.ReceiverAccountPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("parsing receiver private key: %w", err)
	}

	// Start with the original transaction
	signedTx := challengeResp.ParsedTx

	// Sign with receiver's private key
	signedTx, err = signedTx.Sign(challengeResp.NetworkPassphrase, receiverKP)
	if err != nil {
		return nil, fmt.Errorf("signing with receiver key: %w", err)
	}

	// If we have a client domain private key, also sign with it
	if s.ClientDomainPrivateKey != "" {
		var clientDomainKP *keypair.Full
		clientDomainKP, err = keypair.ParseFull(s.ClientDomainPrivateKey)
		if err != nil {
			return nil, fmt.Errorf("parsing client domain private key: %w", err)
		}

		signedTx, err = signedTx.Sign(challengeResp.NetworkPassphrase, clientDomainKP)
		if err != nil {
			return nil, fmt.Errorf("signing with client domain key: %w", err)
		}
	}

	signedTxBase64, err := signedTx.Base64()
	if err != nil {
		return nil, fmt.Errorf("encoding signed transaction: %w", err)
	}

	return &SignedSEP10Challenge{
		SEP10ChallengeResponse: challengeResp,
		SignedTransaction:      signedTxBase64,
	}, nil
}

// ValidateSEP10Challenge submits signed challenge to get JWT token
func (s *SDPSepServicesIntegrationTests) ValidateSEP10Challenge(ctx context.Context, signedChallenge *SignedSEP10Challenge) (*SEP10AuthToken, error) {
	authURL, err := url.JoinPath(s.SDPBaseURL, "auth")
	if err != nil {
		return nil, fmt.Errorf("creating auth URL: %w", err)
	}

	payload := map[string]string{
		"transaction": signedChallenge.SignedTransaction,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshaling payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, authURL, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	// Set tenant header for multi-tenant context
	if s.TenantName != "" {
		req.Header.Set("SDP-Tenant-Name", s.TenantName)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := s.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var token SEP10AuthToken
	if err = json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &token, nil
}

// InitiateSEP24Deposit starts SEP-24 deposit flow with SDP's internal implementation
func (s *SDPSepServicesIntegrationTests) InitiateSEP24Deposit(ctx context.Context, sep10Token *SEP10AuthToken) (*SEP24DepositResponse, error) {
	depositURL, err := url.JoinPath(s.SDPBaseURL, "sep24", "transactions", "deposit", "interactive")
	if err != nil {
		return nil, fmt.Errorf("creating deposit URL: %w", err)
	}

	payload := map[string]string{
		"asset_code":                  s.DisbursedAssetCode,
		"account":                     s.ReceiverAccountPublicKey,
		"lang":                        "en",
		"claimable_balance_supported": "false",
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshaling payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, depositURL, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	// Set tenant header for multi-tenant context
	if s.TenantName != "" {
		req.Header.Set("SDP-Tenant-Name", s.TenantName)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+sep10Token.Token)

	resp, err := s.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var deposit SEP24DepositResponse
	if err := json.NewDecoder(resp.Body).Decode(&deposit); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	// Extract token from URL
	if deposit.URL != "" {
		parsedURL, err := url.Parse(deposit.URL)
		if err == nil {
			deposit.Token = parsedURL.Query().Get("token")
		}
	}

	return &deposit, nil
}

// GetSEP24Transaction checks transaction status
func (s *SDPSepServicesIntegrationTests) GetSEP24Transaction(ctx context.Context, sep10Token *SEP10AuthToken, transactionID string) (*SEP24TransactionStatus, error) {
	txURL, err := url.JoinPath(s.SDPBaseURL, "sep24", "transaction")
	if err != nil {
		return nil, fmt.Errorf("creating transaction URL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, txURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	q := req.URL.Query()
	q.Add("id", transactionID)
	req.URL.RawQuery = q.Encode()

	// Set tenant header for multi-tenant context
	if s.TenantName != "" {
		req.Header.Set("SDP-Tenant-Name", s.TenantName)
	}

	req.Header.Set("Authorization", "Bearer "+sep10Token.Token)

	resp, err := s.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var status SEP24TransactionStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &status, nil
}

// CompleteReceiverRegistration finishes the receiver registration process
func (s *SDPSepServicesIntegrationTests) CompleteReceiverRegistration(ctx context.Context, sep24Token string, registrationData *ReceiverRegistrationRequest) error {
	regURL, err := url.JoinPath(s.SDPBaseURL, "sep24-interactive-deposit", "verification")
	if err != nil {
		return fmt.Errorf("creating registration URL: %w", err)
	}

	jsonPayload, err := json.Marshal(registrationData)
	if err != nil {
		return fmt.Errorf("marshaling payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, regURL, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	// Set tenant header for multi-tenant context
	if s.TenantName != "" {
		req.Header.Set("SDP-Tenant-Name", s.TenantName)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+sep24Token)

	resp, err := s.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
