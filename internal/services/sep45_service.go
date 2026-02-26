package services

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/stellar/go-stellar-sdk/clients/stellartoml"
	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/network"
	protocol "github.com/stellar/go-stellar-sdk/protocols/rpc"
	"github.com/stellar/go-stellar-sdk/strkey"
	"github.com/stellar/go-stellar-sdk/txnbuild"
	"github.com/stellar/go-stellar-sdk/xdr"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/sepauth"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/seputil"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/stellar"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

const (
	// The number of ledgers after which the server-signed authorization entry expires.
	signatureExpirationLedgers = 10
	// The ledger close time in seconds.
	defaultLedgerCloseTime = 5 * time.Second
)

// The default expiration duration for SEP-45 JWTs.
const defaultSEP45JWTExpiration = 2 * time.Hour

// DefaultSEP45NonceExpiration is the default expiration duration for SEP-45 nonces.
const DefaultSEP45NonceExpiration = time.Duration(signatureExpirationLedgers) * defaultLedgerCloseTime

var (
	ErrSEP45Validation = errors.New("sep45 validation error")
	ErrSEP45Internal   = errors.New("sep45 internal error")
)

//go:generate mockery --name=SEP45Service --case=underscore --structname=MockSEP45Service --filename=sep45_service_mock.go --inpackage
type SEP45Service interface {
	// CreateChallenge creates a new challenge for the given contract account and home domain.
	CreateChallenge(ctx context.Context, req SEP45ChallengeRequest) (*SEP45ChallengeResponse, error)
	// ValidateChallenge validates the given challenge and returns a JWT if valid.
	ValidateChallenge(ctx context.Context, req SEP45ValidationRequest) (*SEP45ValidationResponse, error)
}

type sep45Service struct {
	rpcClient         stellar.RPCClient
	tomlClient        stellartoml.ClientInterface
	jwtManager        *sepauth.JWTManager
	networkPassphrase string
	contractID        xdr.ContractId
	signingKP         *keypair.Full
	signingPKBytes    []byte
	allowHTTPRetry    bool
	baseURL           string
	jwtExpiration     time.Duration
	nonceStore        NonceStoreInterface
}

type SEP45ChallengeRequest struct {
	Account      string  `json:"account" query:"account"`
	HomeDomain   string  `json:"home_domain" query:"home_domain"`
	ClientDomain *string `json:"client_domain,omitempty" query:"client_domain"`
}

func (r SEP45ChallengeRequest) Validate() error {
	if strings.TrimSpace(r.Account) == "" {
		return fmt.Errorf("account is required")
	}
	if !strkey.IsValidContractAddress(r.Account) {
		return fmt.Errorf("account must be a valid contract address")
	}
	if strings.TrimSpace(r.HomeDomain) == "" {
		return fmt.Errorf("home_domain is required")
	}
	return nil
}

type SEP45ChallengeResponse struct {
	AuthorizationEntries string `json:"authorization_entries"`
	NetworkPassphrase    string `json:"network_passphrase"`
}

type SEP45ValidationRequest struct {
	AuthorizationEntries string `json:"authorization_entries" form:"authorization_entries"`
}

func (r SEP45ValidationRequest) Validate() error {
	if strings.TrimSpace(r.AuthorizationEntries) == "" {
		return fmt.Errorf("authorization_entries is required")
	}
	return nil
}

type SEP45ValidationResponse struct {
	Token string `json:"token"`
}

type SEP45ServiceOptions struct {
	RPCClient               stellar.RPCClient
	TOMLClient              stellartoml.ClientInterface
	JWTManager              *sepauth.JWTManager
	NetworkPassphrase       string
	WebAuthVerifyContractID string
	ServerSigningKeypair    *keypair.Full
	BaseURL                 string
	AllowHTTPRetry          bool
	NonceStore              NonceStoreInterface
}

func NewSEP45Service(opts SEP45ServiceOptions) (SEP45Service, error) {
	if opts.RPCClient == nil {
		return nil, fmt.Errorf("rpc client cannot be nil")
	}
	if opts.JWTManager == nil {
		return nil, fmt.Errorf("jwt manager cannot be nil")
	}
	if strings.TrimSpace(opts.NetworkPassphrase) == "" {
		return nil, fmt.Errorf("network passphrase cannot be empty")
	}
	if strings.TrimSpace(opts.WebAuthVerifyContractID) == "" {
		return nil, fmt.Errorf("web_auth_verify contract ID cannot be empty")
	}
	if opts.ServerSigningKeypair == nil {
		return nil, fmt.Errorf("server signing keypair cannot be nil")
	}
	if strings.TrimSpace(opts.BaseURL) == "" {
		return nil, fmt.Errorf("base URL cannot be empty")
	}
	if opts.NonceStore == nil {
		return nil, fmt.Errorf("nonce store cannot be nil")
	}

	signingKP := opts.ServerSigningKeypair
	signingPKBytes, err := strkey.Decode(strkey.VersionByteAccountID, signingKP.Address())
	if err != nil {
		return nil, fmt.Errorf("decoding signing public key: %w", err)
	}

	rawContractID, err := strkey.Decode(strkey.VersionByteContract, opts.WebAuthVerifyContractID)
	if err != nil {
		return nil, fmt.Errorf("decoding contract ID: %w", err)
	}
	var contractID xdr.ContractId
	copy(contractID[:], rawContractID)

	tomlClient := opts.TOMLClient
	if tomlClient == nil {
		tomlClient = stellartoml.DefaultClient
	}

	return &sep45Service{
		rpcClient:         opts.RPCClient,
		tomlClient:        tomlClient,
		jwtManager:        opts.JWTManager,
		networkPassphrase: opts.NetworkPassphrase,
		contractID:        contractID,
		signingKP:         signingKP,
		signingPKBytes:    signingPKBytes,
		allowHTTPRetry:    opts.AllowHTTPRetry,
		baseURL:           opts.BaseURL,
		jwtExpiration:     defaultSEP45JWTExpiration,
		nonceStore:        opts.NonceStore,
	}, nil
}

func (s *sep45Service) CreateChallenge(ctx context.Context, req SEP45ChallengeRequest) (*SEP45ChallengeResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrSEP45Validation, err)
	}

	webAuthDomain := seputil.GetWebAuthDomain(ctx, s.baseURL)
	if strings.TrimSpace(webAuthDomain) == "" {
		return nil, fmt.Errorf("%w: unable to determine web_auth_domain", ErrSEP45Internal)
	}

	account := strings.TrimSpace(req.Account)
	homeDomain := strings.TrimSpace(req.HomeDomain)
	if homeDomain == "" {
		return nil, fmt.Errorf("%w: home_domain is required", ErrSEP45Validation)
	}

	if !seputil.IsValidHomeDomain(s.baseURL, homeDomain) {
		return nil, fmt.Errorf("%w: home_domain must match %s", ErrSEP45Validation, seputil.GetBaseDomain(s.baseURL))
	}

	clientDomain := ""
	if req.ClientDomain != nil {
		clientDomain = strings.TrimSpace(*req.ClientDomain)
	}

	var clientDomainAccount string
	if clientDomain != "" {
		key, err := s.fetchSigningKeyFromClientDomain(clientDomain)
		if err != nil {
			return nil, fmt.Errorf("%w: fetching signing key for client_domain %s: %w", ErrSEP45Internal, clientDomain, err)
		}
		clientDomainAccount = key
	}

	nonce, err := generateNonce()
	if err != nil {
		return nil, fmt.Errorf("%w: generating nonce: %w", ErrSEP45Internal, err)
	}
	if err = s.nonceStore.Store(ctx, nonce); err != nil {
		return nil, fmt.Errorf("%w: storing nonce: %w", ErrSEP45Internal, err)
	}

	// Build the invocation arguments for the web_auth_verify contract function, ensuring
	// that fields are in lexicographical order.
	fields := []xdr.ScMapEntry{
		utils.NewSymbolStringEntry("account", account),
	}
	if clientDomain != "" {
		fields = append(fields,
			utils.NewSymbolStringEntry("client_domain", clientDomain),
			utils.NewSymbolStringEntry("client_domain_account", clientDomainAccount),
		)
	}
	fields = append(fields,
		utils.NewSymbolStringEntry("home_domain", homeDomain),
		utils.NewSymbolStringEntry("nonce", nonce),
		utils.NewSymbolStringEntry("web_auth_domain", webAuthDomain),
		utils.NewSymbolStringEntry("web_auth_domain_account", s.signingKP.Address()),
	)

	scMap := xdr.ScMap(fields)
	arg, err := xdr.NewScVal(xdr.ScValTypeScvMap, &scMap)
	if err != nil {
		return nil, fmt.Errorf("%w: building invocation arguments: %w", ErrSEP45Internal, err)
	}
	args := xdr.ScVec{arg}

	hostFunction := xdr.HostFunction{
		Type: xdr.HostFunctionTypeHostFunctionTypeInvokeContract,
		InvokeContract: &xdr.InvokeContractArgs{
			ContractAddress: xdr.ScAddress{
				Type:       xdr.ScAddressTypeScAddressTypeContract,
				ContractId: &s.contractID,
			},
			FunctionName: "web_auth_verify",
			Args:         args,
		},
	}

	txParams := txnbuild.TransactionParams{
		// The challenge transaction's source account must be different than the server signing account
		// so that there is an authorization entry generated for the server signing account.
		SourceAccount: &txnbuild.SimpleAccount{
			AccountID: keypair.MustRandom().Address(),
			Sequence:  0,
		},
		BaseFee: int64(txnbuild.MinBaseFee),
		Preconditions: txnbuild.Preconditions{
			TimeBounds: txnbuild.NewTimeout(300),
		},
		Operations: []txnbuild.Operation{&txnbuild.InvokeHostFunction{
			HostFunction: hostFunction,
		}},
	}

	tx, err := txnbuild.NewTransaction(txParams)
	if err != nil {
		return nil, fmt.Errorf("building transaction: %w", err)
	}

	base64EncodedTx, err := tx.Base64()
	if err != nil {
		return nil, fmt.Errorf("encoding transaction: %w", err)
	}

	// Simulate the transaction to obtain the authorization entries.
	//
	// There should be an entry for:
	// 1. The server signing account.
	// 2. The client contract account (corresponding to the `account` argument).
	// 3. The client domain account (if applicable).
	simResult, simErr := s.rpcClient.SimulateTransaction(ctx, protocol.SimulateTransactionRequest{
		Transaction: base64EncodedTx,
	})
	if simErr != nil {
		return nil, s.wrapSimErr(simErr)
	}

	authEntries, err := s.signServerAuthEntry(ctx, simResult)
	if err != nil {
		return nil, err
	}

	rawEntries, err := authEntries.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("encoding authorization entries: %w", err)
	}

	return &SEP45ChallengeResponse{
		AuthorizationEntries: base64.StdEncoding.EncodeToString(rawEntries),
		NetworkPassphrase:    s.networkPassphrase,
	}, nil
}

type webAuthVerifyArgs struct {
	raw                 map[string]string // Useful for comparing arguments
	xdr                 xdr.ScVec         // Original XDR arguments
	clientAccount       string
	clientContractID    xdr.ContractId
	homeDomain          string
	clientDomain        string
	clientDomainAccount string
}

// authEntryTracker tracks which required authorization entries have been found during validation.
type authEntryTracker struct {
	serverVerified    bool
	clientFound       bool
	clientDomainFound bool
}

// validate ensures all required authorization entries are present.
func (t *authEntryTracker) validate(requireClientDomain bool) error {
	if !t.serverVerified {
		return fmt.Errorf("missing signed server authorization entry")
	}
	if !t.clientFound {
		return fmt.Errorf("missing client account authorization entry")
	}
	if requireClientDomain && !t.clientDomainFound {
		return fmt.Errorf("missing client domain authorization entry")
	}
	return nil
}

// processEntry validates and classifies an authorization entry, updating the tracker state.
func (t *authEntryTracker) processEntry(entry xdr.SorobanAuthorizationEntry, parsedArgs *webAuthVerifyArgs, svc *sep45Service) error {
	addr := entry.Credentials.Address.Address
	switch addr.Type {
	case xdr.ScAddressTypeScAddressTypeAccount:
		if addr.AccountId == nil {
			return fmt.Errorf("authorization entry missing account id")
		}
		// If the account matches the server signing key, we can verify the signature now
		accountAddress := addr.AccountId.Address()
		if accountAddress == svc.signingKP.Address() {
			if err := svc.verifyServerAuthEntry(entry); err != nil {
				return err
			}
			t.serverVerified = true
		} else if parsedArgs != nil && parsedArgs.clientDomainAccount != "" && accountAddress == parsedArgs.clientDomainAccount {
			t.clientDomainFound = true
		} else {
			return fmt.Errorf("unexpected account authorization entry: %s", accountAddress)
		}
	case xdr.ScAddressTypeScAddressTypeContract:
		if addr.ContractId == nil {
			return fmt.Errorf("authorization entry missing contract id")
		}
		if parsedArgs != nil && *addr.ContractId == parsedArgs.clientContractID {
			t.clientFound = true
		} else {
			return fmt.Errorf("unexpected contract authorization entry")
		}
	default:
		return fmt.Errorf("unsupported authorization address type: %d", addr.Type)
	}
	return nil
}

func (s *sep45Service) ValidateChallenge(ctx context.Context, req SEP45ValidationRequest) (*SEP45ValidationResponse, error) {
	webAuthDomain := strings.TrimSpace(seputil.GetWebAuthDomain(ctx, s.baseURL))
	if webAuthDomain == "" {
		return nil, fmt.Errorf("%w: unable to determine web_auth_domain", ErrSEP45Internal)
	}

	encodedEntries := strings.TrimSpace(req.AuthorizationEntries)
	if encodedEntries == "" {
		return nil, fmt.Errorf("%w: authorization_entries is required", ErrSEP45Validation)
	}

	rawEntries, err := base64.StdEncoding.DecodeString(encodedEntries)
	if err != nil {
		return nil, fmt.Errorf("%w: decoding authorization entries: %w", ErrSEP45Validation, err)
	}

	var entries xdr.SorobanAuthorizationEntries
	if err = entries.UnmarshalBinary(rawEntries); err != nil {
		return nil, fmt.Errorf("%w: unmarshalling authorization entries: %w", ErrSEP45Validation, err)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("%w: authorization entries cannot be empty", ErrSEP45Validation)
	}

	var (
		parsedArgs *webAuthVerifyArgs
		tracker    authEntryTracker
		contractFn *xdr.InvokeContractArgs
	)

	for _, entry := range entries {
		contractFn, err = s.ensureWebAuthInvocation(entry)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrSEP45Validation, err)
		}

		// Extract the invocation arguments and make sure they are valid and consistent across entries
		if parsedArgs, err = s.validateArguments(contractFn.Args, parsedArgs, webAuthDomain); err != nil {
			return nil, fmt.Errorf("%w: validating invocation arguments: %w", ErrSEP45Validation, err)
		}

		// Check that we have the expected authorization entries
		if err = tracker.processEntry(entry, parsedArgs, s); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrSEP45Validation, err)
		}
	}

	if parsedArgs == nil {
		return nil, fmt.Errorf("%w: missing authorization arguments", ErrSEP45Validation)
	}
	if err = tracker.validate(parsedArgs.clientDomainAccount != ""); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrSEP45Validation, err)
	}
	validNonce, err := s.nonceStore.Consume(ctx, parsedArgs.raw["nonce"])
	if err != nil {
		return nil, fmt.Errorf("%w: consuming nonce: %w", ErrSEP45Internal, err)
	}
	if !validNonce {
		return nil, fmt.Errorf("%w: nonce is invalid or expired", ErrSEP45Validation)
	}
	if len(parsedArgs.xdr) == 0 {
		return nil, fmt.Errorf("%w: unable to rebuild invocation arguments", ErrSEP45Internal)
	}

	contractID := s.contractID
	hostFunction := xdr.HostFunction{
		Type: xdr.HostFunctionTypeHostFunctionTypeInvokeContract,
		InvokeContract: &xdr.InvokeContractArgs{
			ContractAddress: xdr.ScAddress{
				Type:       xdr.ScAddressTypeScAddressTypeContract,
				ContractId: &contractID,
			},
			FunctionName: "web_auth_verify",
			Args:         parsedArgs.xdr,
		},
	}

	authEntries := make([]xdr.SorobanAuthorizationEntry, len(entries))
	copy(authEntries, entries)

	txParams := txnbuild.TransactionParams{
		SourceAccount: &txnbuild.SimpleAccount{
			AccountID: keypair.MustRandom().Address(),
			Sequence:  0,
		},
		BaseFee: int64(txnbuild.MinBaseFee),
		Preconditions: txnbuild.Preconditions{
			TimeBounds: txnbuild.NewTimeout(300),
		},
		Operations: []txnbuild.Operation{&txnbuild.InvokeHostFunction{
			SourceAccount: s.signingKP.Address(),
			HostFunction:  hostFunction,
			Auth:          authEntries,
		}},
	}

	tx, err := txnbuild.NewTransaction(txParams)
	if err != nil {
		return nil, fmt.Errorf("%w: building transaction: %w", ErrSEP45Internal, err)
	}

	txB64, err := tx.Base64()
	if err != nil {
		return nil, fmt.Errorf("%w: encoding transaction: %w", ErrSEP45Internal, err)
	}

	// Simulate the transaction to validate the authorization entries and ensure the contract invocation is valid.
	// We don't care about the result here, just that it succeeds.
	if _, simErr := s.rpcClient.SimulateTransaction(ctx, protocol.SimulateTransactionRequest{Transaction: txB64}); simErr != nil {
		return nil, s.wrapSimErr(simErr)
	}

	jti, err := s.deriveJTI(entries)
	if err != nil {
		return nil, fmt.Errorf("%w: deriving JTI: %w", ErrSEP45Internal, err)
	}

	protocolScheme := "http"
	if parsedURL, parseErr := url.Parse(s.baseURL); parseErr == nil && parsedURL.Scheme != "" {
		protocolScheme = parsedURL.Scheme
	}

	iat := time.Now().UTC()
	exp := iat.Add(s.jwtExpiration)

	issuer := fmt.Sprintf("%s://%s/sep45/auth", protocolScheme, parsedArgs.homeDomain)

	token, err := s.jwtManager.GenerateSEP45Token(
		issuer,
		parsedArgs.clientAccount,
		jti,
		parsedArgs.clientDomain,
		parsedArgs.homeDomain,
		iat,
		exp,
	)
	if err != nil {
		return nil, fmt.Errorf("%w: generating SEP45 JWT: %w", ErrSEP45Internal, err)
	}

	return &SEP45ValidationResponse{Token: token}, nil
}

func (s *sep45Service) deriveJTI(entries xdr.SorobanAuthorizationEntries) (string, error) {
	if len(entries) == 0 {
		return "", fmt.Errorf("authorization entries cannot be empty")
	}

	invocationBytes, err := entries[0].RootInvocation.MarshalBinary()
	if err != nil {
		return "", fmt.Errorf("marshalling root invocation: %w", err)
	}

	networkID := network.ID(s.networkPassphrase)
	buffer := append(networkID[:], invocationBytes...)
	hash := sha256.Sum256(buffer)

	return hex.EncodeToString(hash[:]), nil
}

func (s *sep45Service) signServerAuthEntry(ctx context.Context, result *stellar.SimulationResult) (xdr.SorobanAuthorizationEntries, error) {
	if result == nil || len(result.Response.Results) == 0 {
		return nil, fmt.Errorf("missing simulation results")
	}
	authXDR := result.Response.Results[0].AuthXDR
	if authXDR == nil {
		return nil, fmt.Errorf("missing authorization entries")
	}

	ledgerNumber, err := s.rpcClient.GetLatestLedgerSequence(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetching latest ledger: %w", err)
	}
	validUntil := ledgerNumber + uint32(signatureExpirationLedgers)

	signedEntries := make(xdr.SorobanAuthorizationEntries, 0, len(*authXDR))
	for _, entryB64 := range *authXDR {
		var entry xdr.SorobanAuthorizationEntry
		if err := xdr.SafeUnmarshalBase64(entryB64, &entry); err != nil {
			return nil, fmt.Errorf("unmarshalling authorization entry: %w", err)
		}

		signedEntry, err := utils.SignAuthEntry(entry, validUntil, s.signingKP, s.networkPassphrase)
		if err != nil {
			return nil, fmt.Errorf("signing authorization entry: %w", err)
		}
		signedEntries = append(signedEntries, signedEntry)
	}

	return signedEntries, nil
}

func (s *sep45Service) fetchSigningKeyFromClientDomain(clientDomain string) (string, error) {
	resp, err := s.tomlClient.GetStellarToml(clientDomain)
	if err != nil && s.allowHTTPRetry {
		if client, ok := s.tomlClient.(*stellartoml.Client); ok {
			fallback := *client
			fallback.UseHTTP = true
			resp, err = fallback.GetStellarToml(clientDomain)
		} else {
			fallback := &stellartoml.Client{UseHTTP: true}
			resp, err = fallback.GetStellarToml(clientDomain)
		}
	}
	if err != nil {
		return "", fmt.Errorf("fetching stellar.toml for %s: %w", clientDomain, err)
	}
	if resp == nil || strings.TrimSpace(resp.SigningKey) == "" {
		return "", fmt.Errorf("stellar.toml at %s missing SIGNING_KEY", clientDomain)
	}
	if !strkey.IsValidEd25519PublicKey(resp.SigningKey) {
		return "", fmt.Errorf("stellar.toml SIGNING_KEY at %s is invalid", clientDomain)
	}
	return resp.SigningKey, nil
}

func generateNonce() (string, error) {
	var buf [4]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("generating nonce: %w", err)
	}
	return fmt.Sprintf("%d", binary.BigEndian.Uint32(buf[:])), nil
}

func (s *sep45Service) ensureWebAuthInvocation(entry xdr.SorobanAuthorizationEntry) (*xdr.InvokeContractArgs, error) {
	if entry.Credentials.Type != xdr.SorobanCredentialsTypeSorobanCredentialsAddress || entry.Credentials.Address == nil {
		return nil, fmt.Errorf("authorization entry missing address credentials")
	}
	if len(entry.RootInvocation.SubInvocations) > 0 {
		return nil, fmt.Errorf("authorization entries cannot contain sub-invocations")
	}
	if entry.RootInvocation.Function.Type != xdr.SorobanAuthorizedFunctionTypeSorobanAuthorizedFunctionTypeContractFn || entry.RootInvocation.Function.ContractFn == nil {
		return nil, fmt.Errorf("authorization entry must invoke contract function")
	}

	contractFn := entry.RootInvocation.Function.ContractFn
	if contractFn.ContractAddress.Type != xdr.ScAddressTypeScAddressTypeContract || contractFn.ContractAddress.ContractId == nil {
		return nil, fmt.Errorf("authorization entry missing contract address")
	}
	if *contractFn.ContractAddress.ContractId != s.contractID {
		return nil, fmt.Errorf("authorization entry targets unexpected contract")
	}
	if contractFn.FunctionName != "web_auth_verify" {
		return nil, fmt.Errorf("authorization entry must call web_auth_verify")
	}
	return contractFn, nil
}

func (s *sep45Service) validateArguments(args xdr.ScVec, parsedArgs *webAuthVerifyArgs, webAuthDomain string) (*webAuthVerifyArgs, error) {
	argsMap, err := utils.ExtractArgsMap(args)
	if err != nil {
		return nil, fmt.Errorf("extracting authorization arguments: %w", err)
	}
	if parsedArgs == nil {
		parsedArgs, err = s.buildChallengeArgs(argsMap, args, webAuthDomain)
		if err != nil {
			return nil, err
		}
	} else if err := compareArgs(argsMap, parsedArgs.raw); err != nil {
		return nil, err
	}
	return parsedArgs, nil
}

func compareArgs(current, expected map[string]string) error {
	if len(current) != len(expected) {
		return fmt.Errorf("authorization entry arguments mismatch")
	}
	for k, v := range expected {
		if current[k] != v {
			return fmt.Errorf("authorization entry arguments mismatch")
		}
	}
	return nil
}

func (s *sep45Service) buildChallengeArgs(args map[string]string, argsXDR xdr.ScVec, webAuthDomain string) (*webAuthVerifyArgs, error) {
	clientAccount := strings.TrimSpace(args["account"])
	if clientAccount == "" {
		return nil, fmt.Errorf("account argument is required")
	}
	rawContractID, err := strkey.Decode(strkey.VersionByteContract, clientAccount)
	if err != nil {
		return nil, fmt.Errorf("account must be a valid contract address: %w", err)
	}
	var contractID xdr.ContractId
	copy(contractID[:], rawContractID)

	homeDomain := strings.TrimSpace(args["home_domain"])
	if homeDomain == "" {
		return nil, fmt.Errorf("home_domain is required")
	}
	if !seputil.IsValidHomeDomain(s.baseURL, homeDomain) {
		return nil, fmt.Errorf("home_domain must match %s", seputil.GetBaseDomain(s.baseURL))
	}

	challengeWebAuthDomain := strings.TrimSpace(args["web_auth_domain"])
	if challengeWebAuthDomain == "" {
		return nil, fmt.Errorf("web_auth_domain is required")
	}
	if !strings.EqualFold(challengeWebAuthDomain, webAuthDomain) {
		return nil, fmt.Errorf("web_auth_domain must equal %s", webAuthDomain)
	}

	webAuthDomainAccount := strings.TrimSpace(args["web_auth_domain_account"])
	if webAuthDomainAccount == "" {
		return nil, fmt.Errorf("web_auth_domain_account is required")
	}
	if !strkey.IsValidEd25519PublicKey(webAuthDomainAccount) {
		return nil, fmt.Errorf("web_auth_domain_account must be a valid Stellar account")
	}
	if webAuthDomainAccount != s.signingKP.Address() {
		return nil, fmt.Errorf("web_auth_domain_account must match server signing key")
	}

	clientDomain := strings.TrimSpace(args["client_domain"])
	clientDomainAccount := strings.TrimSpace(args["client_domain_account"])
	if clientDomainAccount != "" && clientDomain == "" {
		return nil, fmt.Errorf("client_domain is required when client_domain_account is provided")
	}
	if clientDomain != "" {
		if clientDomainAccount == "" {
			return nil, fmt.Errorf("client_domain_account is required when client_domain is provided")
		}
		if !strkey.IsValidEd25519PublicKey(clientDomainAccount) {
			return nil, fmt.Errorf("client_domain_account must be a valid Stellar account")
		}
	}

	if strings.TrimSpace(args["nonce"]) == "" {
		return nil, fmt.Errorf("nonce is required")
	}

	return &webAuthVerifyArgs{
		raw:                 args,
		xdr:                 argsXDR,
		clientAccount:       clientAccount,
		clientContractID:    contractID,
		homeDomain:          homeDomain,
		clientDomain:        clientDomain,
		clientDomainAccount: clientDomainAccount,
	}, nil
}

func (s *sep45Service) verifyServerAuthEntry(entry xdr.SorobanAuthorizationEntry) error {
	if entry.Credentials.Address == nil {
		return fmt.Errorf("server authorization entry missing address credentials")
	}
	sigVal := entry.Credentials.Address.Signature
	if sigVal.Type != xdr.ScValTypeScvVec {
		return fmt.Errorf("server authorization entry missing signature")
	}
	expiration := uint32(entry.Credentials.Address.SignatureExpirationLedger)
	if expiration == 0 {
		return fmt.Errorf("server authorization entry missing expiration ledger")
	}

	publicKey, signature, err := extractSignature(&sigVal)
	if err != nil {
		return err
	}
	if !bytes.Equal(publicKey, s.signingPKBytes) {
		return fmt.Errorf("server authorization entry signed by unexpected key")
	}

	payload, err := utils.BuildAuthorizationPayload(entry, s.networkPassphrase)
	if err != nil {
		return fmt.Errorf("building authorization payload: %w", err)
	}

	// We could also verify that the signature expiration ledger is not
	// expired yet so we can return early but this is also checked during the transaction simulation
	// so we can skip it here to keep the logic simpler.
	if err := s.signingKP.Verify(payload[:], signature); err != nil {
		return fmt.Errorf("server authorization entry signature invalid: %w", err)
	}
	return nil
}

func extractSignature(sigVal *xdr.ScVal) ([]byte, []byte, error) {
	vec, ok := sigVal.GetVec()
	if !ok || vec == nil || len(*vec) == 0 {
		return nil, nil, fmt.Errorf("signature must be a vector")
	}
	sigMapVal := (*vec)[0]
	entries, ok := sigMapVal.GetMap()
	if !ok || entries == nil {
		return nil, nil, fmt.Errorf("signature must be a map")
	}

	var publicKey []byte
	var signature []byte
	for _, entry := range *entries {
		key, ok := entry.Key.GetSym()
		if !ok {
			continue
		}
		switch string(key) {
		case "public_key":
			bytesVal, ok := entry.Val.GetBytes()
			if !ok {
				return nil, nil, fmt.Errorf("signature public key must be bytes")
			}
			publicKey = slices.Clone(bytesVal)
		case "signature":
			bytesVal, ok := entry.Val.GetBytes()
			if !ok {
				return nil, nil, fmt.Errorf("signature bytes missing")
			}
			signature = slices.Clone(bytesVal)
		}
	}
	if len(publicKey) == 0 {
		return nil, nil, fmt.Errorf("signature missing public key")
	}
	if len(signature) == 0 {
		return nil, nil, fmt.Errorf("signature missing value")
	}
	return publicKey, signature, nil
}

func (s *sep45Service) wrapSimErr(simErr *stellar.SimulationError) error {
	if simErr == nil {
		return nil
	}

	switch simErr.Type {
	case stellar.SimulationErrorTypeAuth,
		stellar.SimulationErrorTypeContractExecution,
		stellar.SimulationErrorTypeTransactionInvalid:
		return fmt.Errorf("%w: simulating transaction: %w", ErrSEP45Validation, simErr)
	default:
		return fmt.Errorf("%w: simulating transaction: %w", ErrSEP45Internal, simErr)
	}
}
