package services

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/stellar/go-stellar-sdk/clients/horizonclient"
	"github.com/stellar/go-stellar-sdk/clients/stellartoml"
	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/protocols/horizon"
	"github.com/stellar/go-stellar-sdk/strkey"
	"github.com/stellar/go-stellar-sdk/txnbuild"
	"github.com/stellar/go-stellar-sdk/xdr"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sepauth"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpclient"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/seputil"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

//go:generate mockery --name=SEP10Service --case=underscore --structname=MockSEP10Service --filename=sep10_service_mock.go --inpackage
type SEP10Service interface {
	CreateChallenge(ctx context.Context, req ChallengeRequest) (*ChallengeResponse, error)
	ValidateChallenge(ctx context.Context, req ValidationRequest) (*ValidationResponse, error)
}

// DefaultSEP10AuthTimeout is the default expiration duration for SEP-10 challenge transactions.
const DefaultSEP10AuthTimeout = 15 * time.Minute

// DefaultSEP10NonceExpiration is the default expiration duration for SEP-10 nonces.
const DefaultSEP10NonceExpiration = DefaultSEP10AuthTimeout

type sep10Service struct {
	SEP10SigningKeypair       *keypair.Full
	JWTManager                *sepauth.JWTManager
	HorizonClient             horizonclient.ClientInterface
	HTTPClient                httpclient.HTTPClientInterface
	NetworkPassphrase         string
	BaseURL                   string
	JWTExpiration             time.Duration
	AuthTimeout               time.Duration
	AllowHTTPRetry            bool
	ClientAttributionRequired bool
	nonceStore                NonceStoreInterface
}

// ChallengeValidationError represents a client input error during challenge creation.
type ChallengeValidationError string

func (e ChallengeValidationError) Error() string { return string(e) }

type ChallengeRequest struct {
	Account      string `json:"account" query:"account"`
	Memo         string `json:"memo,omitempty" query:"memo"`
	HomeDomain   string `json:"home_domain,omitempty" query:"home_domain"`
	ClientDomain string `json:"client_domain,omitempty" query:"client_domain"`
}

func (req *ChallengeRequest) Validate() error {
	if req.Account == "" {
		return fmt.Errorf("account is required")
	}

	if !strkey.IsValidEd25519PublicKey(req.Account) {
		return fmt.Errorf("invalid account not a valid ed25519 public key")
	}

	if req.Memo != "" {
		_, memoType, err := schema.ParseMemo(req.Memo)
		if err != nil {
			return fmt.Errorf("invalid memo: %w", err)
		}
		if memoType != schema.MemoTypeID {
			return fmt.Errorf("invalid memo type - expected ID memo, got %s", memoType)
		}
	}

	return nil
}

type ChallengeResponse struct {
	Transaction       string `json:"transaction"`
	NetworkPassphrase string `json:"network_passphrase"`
}

type ValidationRequest struct {
	Transaction string `json:"transaction" form:"transaction"`
}

func (req *ValidationRequest) Validate() error {
	if req.Transaction == "" {
		return fmt.Errorf("transaction is required")
	}
	return nil
}

type ValidationResponse struct {
	Token string `json:"token"`
}

type ChallengeValidationResult struct {
	Transaction     *txnbuild.Transaction
	ClientAccountID string
	HomeDomain      string
	Memo            *txnbuild.MemoID
	ClientDomain    string
	Nonce           string
}

func NewSEP10Service(
	jwtManager *sepauth.JWTManager,
	networkPassphrase string,
	sep10SigningPrivateKey string,
	baseURL string,
	allowHTTPRetry bool,
	horizonClient horizonclient.ClientInterface,
	clientAttributionRequired bool,
	nonceStore NonceStoreInterface,
) (SEP10Service, error) {
	kp, err := keypair.ParseFull(sep10SigningPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("parsing sep10 signing key %w", err)
	}

	return &sep10Service{
		JWTManager:                jwtManager,
		JWTExpiration:             time.Hour * 2,
		NetworkPassphrase:         networkPassphrase,
		SEP10SigningKeypair:       kp,
		AuthTimeout:               DefaultSEP10AuthTimeout,
		BaseURL:                   baseURL,
		AllowHTTPRetry:            allowHTTPRetry,
		HTTPClient:                httpclient.DefaultClient(),
		HorizonClient:             horizonClient,
		ClientAttributionRequired: clientAttributionRequired,
		nonceStore:                nonceStore,
	}, nil
}

func (s *sep10Service) CreateChallenge(ctx context.Context, req ChallengeRequest) (*ChallengeResponse, error) {
	webAuthDomain := seputil.GetWebAuthDomain(ctx, s.BaseURL)

	req.ClientDomain = strings.TrimSpace(req.ClientDomain)
	req.HomeDomain = strings.TrimSpace(req.HomeDomain)

	// Only require client_domain if ClientAttributionRequired is true for the backwards compatibility
	if s.ClientAttributionRequired && strings.TrimSpace(req.ClientDomain) == "" {
		return nil, ChallengeValidationError("client_domain is required")
	}

	if req.HomeDomain == "" {
		req.HomeDomain = seputil.GetBaseDomain(s.BaseURL)
		if req.HomeDomain == "" {
			return nil, ChallengeValidationError("home_domain is required")
		}
	}

	if !seputil.IsValidHomeDomain(s.BaseURL, req.HomeDomain) {
		return nil, ChallengeValidationError(fmt.Sprintf("invalid home_domain must match %s", seputil.GetBaseDomain(s.BaseURL)))
	}

	if _, err := xdr.AddressToAccountId(req.Account); err != nil {
		return nil, ChallengeValidationError(fmt.Sprintf("%s is not a valid account id", req.Account))
	}

	if req.Account == s.SEP10SigningKeypair.Address() {
		return nil, ChallengeValidationError("account must not be the server signing key")
	}

	var clientSigningKey string
	if req.ClientDomain != "" {
		var err error
		clientSigningKey, err = s.fetchSigningKeyFromClientDomain(req.ClientDomain)
		if err != nil {
			return nil, fmt.Errorf("fetching client domain signing key: %w", err)
		}
	}

	var memoParam *txnbuild.MemoID
	if req.Memo != "" {
		memo, memoType, parseErr := schema.ParseMemo(req.Memo)
		if parseErr != nil {
			return nil, ChallengeValidationError(fmt.Sprintf("invalid memo: %s", parseErr))
		}
		if memoType != schema.MemoTypeID {
			return nil, ChallengeValidationError(fmt.Sprintf("invalid memo type - expected ID memo, got %s", memoType))
		}
		if memoID, ok := memo.(txnbuild.MemoID); ok {
			memoParam = &memoID
		}
	}

	tx, err := s.buildChallengeTx(ctx, req.Account, webAuthDomain, req.HomeDomain, req.ClientDomain, clientSigningKey, memoParam)
	if err != nil {
		return nil, fmt.Errorf("building challenge transaction %w", err)
	}

	txBase64, err := tx.Base64()
	if err != nil {
		return nil, fmt.Errorf("encoding transaction %w", err)
	}

	return &ChallengeResponse{
		Transaction:       txBase64,
		NetworkPassphrase: s.NetworkPassphrase,
	}, nil
}

func (s *sep10Service) ValidateChallenge(ctx context.Context, req ValidationRequest) (*ValidationResponse, error) {
	allowedHomeDomains := s.getAllowedHomeDomains(ctx)
	webAuthDomain := seputil.GetWebAuthDomain(ctx, s.BaseURL)

	result, err := s.validateChallengeCustom(
		req.Transaction,
		s.SEP10SigningKeypair.Address(),
		s.NetworkPassphrase,
		webAuthDomain,
		allowedHomeDomains,
	)
	if err != nil {
		return nil, fmt.Errorf("reading challenge transaction %w", err)
	}

	account, err := s.fetchAccountFromHorizon(result.ClientAccountID)
	if err != nil {
		// Check if it's a 404 (account not found) error
		var hErr *horizonclient.Error
		if errors.As(err, &hErr) && hErr.Problem.Status == 404 {
			// Account doesn't exist - verify with just the client's master key
			if verifyErr := s.verifySignaturesForNonExistentAccount(
				result.Transaction,
				result.ClientAccountID,
				result.ClientDomain,
			); verifyErr != nil {
				return nil, verifyErr
			}

			// Check and consume nonce
			validNonce, consumeErr := s.nonceStore.Consume(ctx, result.Nonce)
			if consumeErr != nil {
				return nil, fmt.Errorf("consuming nonce: %w", consumeErr)
			}
			if !validNonce {
				return nil, fmt.Errorf("nonce is invalid or expired")
			}

			// Generate token without threshold check for non-existent account
			return s.generateToken(result.Transaction, result.ClientAccountID, result.HomeDomain, result.Memo, result.ClientDomain)
		}
		return nil, fmt.Errorf("fetching account from horizon: %w", err)
	}

	// Account exists - verify with threshold
	if err = s.verifySignaturesWithThreshold(
		result.Transaction,
		result.ClientAccountID,
		result.ClientDomain,
		account,
	); err != nil {
		return nil, err
	}

	// Check and consume nonce
	validNonce, err := s.nonceStore.Consume(ctx, result.Nonce)
	if err != nil {
		return nil, fmt.Errorf("consuming nonce: %w", err)
	}
	if !validNonce {
		return nil, fmt.Errorf("nonce is invalid or expired")
	}

	return s.generateToken(result.Transaction, result.ClientAccountID, result.HomeDomain, result.Memo, result.ClientDomain)
}

// validateChallengeCustom provides custom SEP-10 challenge validation that properly handles client_domain operations.
func (s *sep10Service) validateChallengeCustom(challengeTx, serverAccountID, network, webAuthDomain string, homeDomains []string) (*ChallengeValidationResult, error) {
	if _, err := utils.ValidateAndDecodeBase64XDR(challengeTx, utils.DefaultMaxXDRDecodedSize); err != nil {
		return nil, fmt.Errorf("invalid challenge transaction: %w", err)
	}

	parsed, err := txnbuild.TransactionFromXDR(challengeTx)
	if err != nil {
		return nil, fmt.Errorf("could not parse challenge: %w", err)
	}

	tx, isSimple := parsed.Transaction()
	if !isSimple {
		return nil, fmt.Errorf("challenge cannot be a fee bump transaction")
	}

	if tx.SourceAccount().AccountID != serverAccountID {
		return nil, fmt.Errorf("transaction source account is not equal to server's account")
	}

	if tx.SourceAccount().Sequence != 0 {
		return nil, fmt.Errorf("transaction sequence number must be 0")
	}

	if tx.Timebounds().MaxTime == txnbuild.TimeoutInfinite {
		return nil, fmt.Errorf("transaction requires non-infinite timebounds")
	}

	const gracePeriod = 5 * 60
	currentTime := time.Now().UTC().Unix()
	if currentTime+gracePeriod < tx.Timebounds().MinTime || currentTime > tx.Timebounds().MaxTime {
		return nil, fmt.Errorf("transaction is not within range of the specified timebounds (currentTime=%d, MinTime=%d, MaxTime=%d)",
			currentTime, tx.Timebounds().MinTime, tx.Timebounds().MaxTime)
	}

	operations := tx.Operations()
	if len(operations) < 1 {
		return nil, fmt.Errorf("transaction requires at least one manage_data operation")
	}

	op, ok := operations[0].(*txnbuild.ManageData)
	if !ok {
		return nil, fmt.Errorf("operation type should be manage_data")
	}

	if op.SourceAccount == "" {
		return nil, fmt.Errorf("operation should have a source account")
	}

	var matchedHomeDomain string
	for _, homeDomain := range homeDomains {
		if op.Name == homeDomain+" auth" {
			matchedHomeDomain = homeDomain
			break
		}
	}
	if matchedHomeDomain == "" {
		return nil, fmt.Errorf("operation key does not match any homeDomains passed (key=%q, homeDomains=%v)", op.Name, homeDomains)
	}

	clientAccountID := op.SourceAccount
	if clientAccountID == serverAccountID {
		return nil, fmt.Errorf("client account must not be the server account")
	}

	var memo *txnbuild.MemoID
	if tx.Memo() != nil {
		if memoID, ok := tx.Memo().(txnbuild.MemoID); ok {
			memo = &memoID
		} else {
			return nil, fmt.Errorf("invalid memo, only ID memos are permitted")
		}
	}

	nonceB64 := string(op.Value)
	if len(nonceB64) != 64 {
		return nil, fmt.Errorf("random nonce encoded as base64 should be 64 bytes long")
	}
	nonceBytes, err := base64.StdEncoding.DecodeString(nonceB64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode random nonce provided in manage_data operation: %w", err)
	}
	if len(nonceBytes) != 48 {
		return nil, fmt.Errorf("random nonce before encoding as base64 should be 48 bytes long")
	}

	var clientDomain string
	foundClientDomain := false
	for i, operation := range operations[1:] {
		manageDataOp, ok := operation.(*txnbuild.ManageData)
		if !ok {
			return nil, fmt.Errorf("subsequent operation %d type should be manage_data", i+1)
		}

		if manageDataOp.SourceAccount == "" {
			return nil, fmt.Errorf("subsequent operation %d should have a source account", i+1)
		}

		switch manageDataOp.Name {
		case "web_auth_domain":
			if manageDataOp.SourceAccount != serverAccountID {
				return nil, fmt.Errorf("web auth domain operation must have server source account")
			}
			if !bytes.Equal(manageDataOp.Value, []byte(webAuthDomain)) {
				return nil, fmt.Errorf("web auth domain operation value is %q but expect %q", string(manageDataOp.Value), webAuthDomain)
			}
		case "client_domain":
			if _, err := xdr.AddressToAccountId(manageDataOp.SourceAccount); err != nil {
				return nil, fmt.Errorf("client_domain operation has invalid source account: %w", err)
			}
			clientDomain = string(manageDataOp.Value)
			foundClientDomain = true
		default:
			if manageDataOp.SourceAccount != serverAccountID {
				return nil, fmt.Errorf("unknown subsequent operation %q must have server account as source", manageDataOp.Name)
			}
		}
	}

	if s.ClientAttributionRequired && !foundClientDomain {
		return nil, fmt.Errorf("client_domain manage_data operation is required")
	}

	if err := s.verifyServerSignature(tx, network, serverAccountID); err != nil {
		return nil, fmt.Errorf("verifying server signature: %w", err)
	}

	return &ChallengeValidationResult{
		Transaction:     tx,
		ClientAccountID: clientAccountID,
		HomeDomain:      matchedHomeDomain,
		Memo:            memo,
		ClientDomain:    clientDomain,
		Nonce:           nonceB64,
	}, nil
}

// verifyServerSignature does not consume signatures, so it is only safe for the server key, whose signature is consumed again during attribution.
func (s *sep10Service) verifyServerSignature(tx *txnbuild.Transaction, network, serverAccountID string) error {
	hash, err := tx.Hash(network)
	if err != nil {
		return fmt.Errorf("computing transaction hash: %w", err)
	}

	signatures := tx.Signatures()
	if len(signatures) == 0 {
		return fmt.Errorf("transaction has no signatures")
	}

	kp, err := keypair.ParseAddress(serverAccountID)
	if err != nil {
		return fmt.Errorf("parsing server account: %w", err)
	}

	for _, sig := range signatures {
		if err := kp.Verify(hash[:], sig.Signature); err == nil {
			return nil
		}
	}

	return fmt.Errorf("transaction is not signed by server account %s", serverAccountID)
}

// verifySignaturesForNonExistentAccount verifies signatures for accounts that don't exist on the network
// yet, where only the client account's master key can authenticate it.
func (s *sep10Service) verifySignaturesForNonExistentAccount(
	tx *txnbuild.Transaction,
	clientAccountID string,
	clientDomain string,
) error {
	signers := challengeSigners{
		accountID: clientAccountID,
		server:    s.SEP10SigningKeypair.Address(),
		account:   map[string]int{clientAccountID: 0},
	}

	if clientDomain != "" {
		clientDomainAccountID, err := s.fetchSigningKeyFromClientDomain(clientDomain)
		if err != nil {
			return fmt.Errorf("fetching client domain signing key: %w", err)
		}
		signers.clientDomain = clientDomainAccountID
	}

	return s.verifyChallengeSignatures(tx, signers, 0)
}

// ed25519SignerType is the horizon signer type for the only signer keys that can sign a challenge.
const ed25519SignerType = "ed25519_public_key"

// challengeSigners holds the keys that may legitimately have signed a challenge transaction.
type challengeSigners struct {
	accountID    string
	server       string
	clientDomain string
	account      map[string]int // signers able to authenticate the client account, by weight
}

// addresses returns every candidate key, deduplicated across roles.
func (cs challengeSigners) addresses() []string {
	addresses := make([]string, 0, len(cs.account)+2)
	addresses = append(addresses, cs.server)
	if cs.clientDomain != "" && cs.clientDomain != cs.server {
		addresses = append(addresses, cs.clientDomain)
	}

	for address := range cs.account {
		if address == cs.server || address == cs.clientDomain {
			continue
		}
		addresses = append(addresses, address)
	}

	return addresses
}

func (s *sep10Service) verifySignaturesWithThreshold(
	tx *txnbuild.Transaction,
	clientAccountID string,
	clientDomain string,
	account *horizon.Account,
) error {
	signers := challengeSigners{
		accountID: clientAccountID,
		server:    s.SEP10SigningKeypair.Address(),
		account:   make(map[string]int, len(account.Signers)),
	}

	if clientDomain != "" {
		clientDomainAccountID, err := s.fetchSigningKeyFromClientDomain(clientDomain)
		if err != nil {
			return fmt.Errorf("fetching client domain signing key: %w", err)
		}
		signers.clientDomain = clientDomainAccountID
	}

	for _, signer := range account.Signers {
		if signer.Type != ed25519SignerType {
			continue
		}
		signers.account[signer.Key] = int(signer.Weight)
	}

	return s.verifyChallengeSignatures(tx, signers, int(account.Thresholds.MedThreshold))
}

// verifyChallengeSignatures applies SEP-10's signer rules: every signature must belong to an expected
// signer, and the client account must be represented by at least one signer of its own.
func (s *sep10Service) verifyChallengeSignatures(
	tx *txnbuild.Transaction,
	signers challengeSigners,
	threshold int,
) error {
	matched, unrecognized, err := attributeSignatures(tx, s.NetworkPassphrase, signers.addresses())
	if err != nil {
		return err
	}

	if !matched[signers.server] {
		return fmt.Errorf("challenge is not signed by server account %s", signers.server)
	}

	if signers.clientDomain != "" && !matched[signers.clientDomain] {
		return fmt.Errorf("verifying client domain signature: challenge is not signed by client domain account %s", signers.clientDomain)
	}

	// SEP-10 requires the server's own signature to be excluded when weighing the account's signers.
	totalWeight, signersFound := 0, 0
	for address, weight := range signers.account {
		if address == signers.server || !matched[address] {
			continue
		}
		signersFound++
		totalWeight += weight
	}

	if signersFound == 0 {
		return fmt.Errorf("verifying client signature: challenge is not signed by any signer of account %s", signers.accountID)
	}

	if totalWeight < threshold {
		return fmt.Errorf("verifying signature threshold: signatures with weight %d do not meet threshold %d", totalWeight, threshold)
	}

	if unrecognized > 0 {
		return fmt.Errorf("challenge has %d unrecognized signature(s)", unrecognized)
	}

	return nil
}

// attributeSignatures matches every signature to at most one candidate key, consuming both, and reports
// how many signatures could not be attributed.
func attributeSignatures(tx *txnbuild.Transaction, networkPassphrase string, addresses []string) (map[string]bool, int, error) {
	hash, err := tx.Hash(networkPassphrase)
	if err != nil {
		return nil, 0, fmt.Errorf("computing transaction hash: %w", err)
	}

	candidates := make(map[string]*keypair.FromAddress, len(addresses))
	for _, address := range addresses {
		kp, parseErr := keypair.ParseAddress(address)
		if parseErr != nil {
			continue
		}
		candidates[address] = kp
	}

	matched := make(map[string]bool, len(candidates))
	unrecognized := 0

	for _, sig := range tx.Signatures() {
		signer := ""
		for address, kp := range candidates {
			if matched[address] || kp.Hint() != sig.Hint {
				continue
			}
			if kp.Verify(hash[:], sig.Signature) == nil {
				signer = address
				break
			}
		}

		if signer == "" {
			unrecognized++
			continue
		}
		matched[signer] = true
	}

	return matched, unrecognized, nil
}

func (s *sep10Service) generateToken(
	tx *txnbuild.Transaction,
	clientAccountID, matchedHomeDomain string,
	memo *txnbuild.MemoID,
	clientDomain string,
) (*ValidationResponse, error) {
	subject := clientAccountID
	if memo != nil {
		subject = fmt.Sprintf("%s:%d", clientAccountID, uint64(*memo))
	}

	jti, err := tx.HashHex(s.NetworkPassphrase)
	if err != nil {
		return nil, fmt.Errorf("getting transaction hash: %w", err)
	}

	timebounds := tx.Timebounds()
	iat := time.Unix(timebounds.MinTime, 0)
	exp := iat.Add(s.JWTExpiration)

	protocol := "http"
	if parsedURL, parseErr := url.Parse(s.BaseURL); parseErr == nil && parsedURL.Scheme != "" {
		protocol = parsedURL.Scheme
	}

	token, err := s.JWTManager.GenerateSEP10Token(
		fmt.Sprintf("%s://%s/auth", protocol, matchedHomeDomain),
		subject,
		jti,
		clientDomain,
		matchedHomeDomain,
		iat,
		exp,
	)
	if err != nil {
		return nil, fmt.Errorf("generating SEP10 JWT: %w", err)
	}

	return &ValidationResponse{Token: token}, nil
}

func (s *sep10Service) getAllowedHomeDomains(ctx context.Context) []string {
	baseDomain := seputil.GetBaseDomain(s.BaseURL)
	if baseDomain == "" {
		return []string{}
	}

	allowedDomains := []string{baseDomain}

	currentTenant, err := sdpcontext.GetTenantFromContext(ctx)
	if err == nil && currentTenant != nil && currentTenant.BaseURL != nil {
		parsedURL, parseErr := url.Parse(*currentTenant.BaseURL)
		if parseErr == nil && parsedURL.Host != "" {
			allowedDomains = append(allowedDomains, parsedURL.Host)
		}
	}

	return allowedDomains
}

func (s *sep10Service) buildChallengeTx(ctx context.Context, clientAccountID, webAuthDomain, homeDomain, clientDomain string, clientDomainAccountID string, memo *txnbuild.MemoID) (*txnbuild.Transaction, error) {
	if s.AuthTimeout < time.Second {
		return nil, fmt.Errorf("provided timebound must be at least 1s (300s is recommended)")
	}

	randomNonce, err := s.generateRandomNonce(48)
	if err != nil {
		return nil, err
	}
	randomNonceToString := base64.StdEncoding.EncodeToString(randomNonce)
	if len(randomNonceToString) != 64 {
		return nil, fmt.Errorf("64 byte long random nonce required")
	}

	if _, err = xdr.AddressToAccountId(clientAccountID); err != nil {
		return nil, fmt.Errorf("%s is not a valid account id", clientAccountID)
	}

	if clientDomainAccountID != "" {
		if _, parseErr := keypair.ParseAddress(clientDomainAccountID); parseErr != nil {
			return nil, fmt.Errorf("invalid client domain account ID: %s is not a valid Stellar account ID", clientDomainAccountID)
		}
	}

	if err = s.nonceStore.Store(ctx, randomNonceToString); err != nil {
		return nil, fmt.Errorf("storing nonce: %w", err)
	}

	sa := txnbuild.SimpleAccount{
		AccountID: s.SEP10SigningKeypair.Address(),
		Sequence:  -1,
	}

	currentTime := time.Now().UTC()
	maxTime := currentTime.Add(s.AuthTimeout)

	operations := []txnbuild.Operation{
		&txnbuild.ManageData{
			SourceAccount: clientAccountID,
			Name:          homeDomain + " auth",
			Value:         []byte(randomNonceToString),
		},
		&txnbuild.ManageData{
			SourceAccount: s.SEP10SigningKeypair.Address(),
			Name:          "web_auth_domain",
			Value:         []byte(webAuthDomain),
		},
	}

	if clientDomainAccountID != "" {
		operations = append(operations, &txnbuild.ManageData{
			SourceAccount: clientDomainAccountID,
			Name:          "client_domain",
			Value:         []byte(clientDomain),
		})
	}

	txParams := txnbuild.TransactionParams{
		SourceAccount:        &sa,
		IncrementSequenceNum: true,
		Operations:           operations,
		BaseFee:              txnbuild.MinBaseFee,
		Preconditions: txnbuild.Preconditions{
			TimeBounds: txnbuild.NewTimebounds(currentTime.Unix(), maxTime.Unix()),
		},
	}

	if memo != nil {
		txParams.Memo = memo
	}

	tx, err := txnbuild.NewTransaction(txParams)
	if err != nil {
		return nil, fmt.Errorf("creating new transaction: %w", err)
	}

	tx, err = tx.Sign(s.NetworkPassphrase, s.SEP10SigningKeypair)
	if err != nil {
		return nil, fmt.Errorf("signing transaction: %w", err)
	}
	return tx, nil
}

func (s *sep10Service) generateRandomNonce(n int) ([]byte, error) {
	binary := make([]byte, n)
	_, err := rand.Read(binary)
	if err != nil {
		return []byte{}, fmt.Errorf("reading random bytes: %w", err)
	}

	return binary, nil
}

func (s *sep10Service) fetchSigningKeyFromClientDomain(clientDomain string) (string, error) {
	client := &stellartoml.Client{
		HTTP: s.HTTPClient,
	}

	stellarToml, err := client.GetStellarToml(clientDomain)
	if err != nil && s.AllowHTTPRetry {
		// Fallback to HTTP if HTTPS fails and retry is allowed.
		client.UseHTTP = true
		stellarToml, err = client.GetStellarToml(clientDomain)
	}

	if err != nil {
		return "", fmt.Errorf("unable to fetch stellar.toml from %s: %w", clientDomain, err)
	}

	if stellarToml.SigningKey == "" {
		return "", fmt.Errorf("SIGNING_KEY not present in client_domain stellar.toml")
	}

	if _, parseErr := keypair.ParseAddress(stellarToml.SigningKey); parseErr != nil {
		return "", fmt.Errorf("SIGNING_KEY %s is not a valid Stellar account ID", stellarToml.SigningKey)
	}

	// otherwise the server's own challenge signature would satisfy the client_domain signature.
	if stellarToml.SigningKey == s.SEP10SigningKeypair.Address() {
		return "", fmt.Errorf("SIGNING_KEY of client_domain %s must not be the server signing key", clientDomain)
	}

	return stellarToml.SigningKey, nil
}

func (s *sep10Service) fetchAccountFromHorizon(accountID string) (*horizon.Account, error) {
	if s.HorizonClient == nil {
		return nil, fmt.Errorf("horizon client is not configured")
	}

	account, err := s.HorizonClient.AccountDetail(horizonclient.AccountRequest{
		AccountID: accountID,
	})
	if err != nil {
		return nil, fmt.Errorf("loading account %s from horizon: %w", accountID, err)
	}
	return &account, nil
}
