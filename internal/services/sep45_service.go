package services

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"

	"github.com/stellar/go/clients/stellartoml"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/strkey"
	"github.com/stellar/go/txnbuild"
	"github.com/stellar/go/xdr"
	"github.com/stellar/stellar-rpc/protocol"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/stellar"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

// The number of ledgers after which the server-signed authorization entry expires.
const signatureExpirationLedgers = 10

//go:generate mockery --name=SEP45Service --case=underscore --structname=MockSEP45Service --filename=sep45_service_mock.go --inpackage
type SEP45Service interface {
	// CreateChallenge creates a new challenge for the given contract account and home domain.
	CreateChallenge(ctx context.Context, req SEP45ChallengeRequest) (*SEP45ChallengeResponse, error)
	// ValidateChallenge validates the given challenge and returns a JWT if valid.
	ValidateChallenge(ctx context.Context, req SEP45ValidationRequest) (*SEP45ValidationResponse, error)
}

type sep45Service struct {
	rpcClient                 stellar.RPCClient
	tomlClient                stellartoml.ClientInterface
	networkPassphrase         string
	contractID                xdr.ContractId
	signingKP                 *keypair.Full
	signingPKBytes            []byte
	clientAttributionRequired bool
	allowHTTPRetry            bool
	baseURL                   string
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

type SEP45ValidationResponse struct {
	Token string `json:"token"`
}

type SEP45ServiceOptions struct {
	RPCClient                 stellar.RPCClient
	TOMLClient                stellartoml.ClientInterface
	NetworkPassphrase         string
	WebAuthVerifyContractID   string
	ServerSigningKeypair      *keypair.Full
	BaseURL                   string
	ClientAttributionRequired bool
	AllowHTTPRetry            bool
}

func NewSEP45Service(opts SEP45ServiceOptions) (SEP45Service, error) {
	if opts.RPCClient == nil {
		return nil, fmt.Errorf("rpc client cannot be nil")
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
		rpcClient:                 opts.RPCClient,
		tomlClient:                tomlClient,
		networkPassphrase:         opts.NetworkPassphrase,
		contractID:                contractID,
		signingKP:                 signingKP,
		signingPKBytes:            signingPKBytes,
		clientAttributionRequired: opts.ClientAttributionRequired,
		allowHTTPRetry:            opts.AllowHTTPRetry,
		baseURL:                   opts.BaseURL,
	}, nil
}

func (s *sep45Service) CreateChallenge(ctx context.Context, req SEP45ChallengeRequest) (*SEP45ChallengeResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	webAuthDomain := s.getWebAuthDomain(ctx)
	if strings.TrimSpace(webAuthDomain) == "" {
		return nil, fmt.Errorf("unable to determine web_auth_domain")
	}

	account := strings.TrimSpace(req.Account)
	homeDomain := strings.TrimSpace(req.HomeDomain)
	if homeDomain == "" {
		return nil, fmt.Errorf("home_domain is required")
	}

	if !s.isValidHomeDomain(homeDomain) {
		return nil, fmt.Errorf("invalid home_domain must match %s", s.getBaseDomain())
	}

	clientDomain := ""
	if req.ClientDomain != nil {
		clientDomain = strings.TrimSpace(*req.ClientDomain)
	}
	if s.clientAttributionRequired && clientDomain == "" {
		return nil, fmt.Errorf("client_domain is required")
	}

	var clientDomainAccount string
	if clientDomain != "" {
		key, err := s.fetchSigningKeyFromClientDomain(clientDomain)
		if err != nil {
			return nil, fmt.Errorf("fetching signing key for client_domain %s: %w", clientDomain, err)
		}
		clientDomainAccount = key
	}

	// TODO(philip): We generate a random nonce right now and don't store it anywhere.
	// This is also the case with the SEP-10 implementation, so we should address them together.
	nonce, err := generateNonce()
	if err != nil {
		return nil, fmt.Errorf("generating nonce: %w", err)
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
		return nil, fmt.Errorf("building invocation arguments: %w", err)
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
			SourceAccount: s.signingKP.Address(),
			HostFunction:  hostFunction,
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
		return nil, fmt.Errorf("simulating transaction: %w", simErr)
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

func (s *sep45Service) ValidateChallenge(ctx context.Context, req SEP45ValidationRequest) (*SEP45ValidationResponse, error) {
	return nil, fmt.Errorf("challenge validation is not implemented")
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

// TODO(philip): Below methods are shared with sep10_service.go so they can be moved to a common utility package later.

func (s *sep45Service) getWebAuthDomain(ctx context.Context) string {
	currentTenant, err := sdpcontext.GetTenantFromContext(ctx)
	if err == nil && currentTenant != nil && currentTenant.BaseURL != nil {
		parsedURL, parseErr := url.Parse(*currentTenant.BaseURL)
		if parseErr == nil {
			return parsedURL.Host
		}
	}
	return s.getBaseDomain()
}

func (s *sep45Service) getBaseDomain() string {
	parsed, err := url.Parse(s.baseURL)
	if err != nil {
		return ""
	}
	return parsed.Host
}

func (s *sep45Service) isValidHomeDomain(homeDomain string) bool {
	baseDomain := s.getBaseDomain()
	if baseDomain == "" || homeDomain == "" {
		return false
	}

	baseDomainLower := strings.ToLower(baseDomain)
	homeDomainLower := strings.ToLower(homeDomain)

	if homeDomainLower == baseDomainLower {
		return true
	}

	return strings.HasSuffix(homeDomainLower, "."+baseDomainLower)
}
