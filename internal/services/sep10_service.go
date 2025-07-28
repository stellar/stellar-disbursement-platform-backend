package services

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/stellar/go/keypair"
	"github.com/stellar/go/strkey"
	"github.com/stellar/go/support/log"
	"github.com/stellar/go/txnbuild"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/anchorplatform"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

type SEP10Service struct {
	JWTManager          *anchorplatform.JWTManager
	JWTExpiration       time.Duration
	NetworkPassphrase   string
	Sep10SigningKeypair *keypair.Full
	AuthTimeout         time.Duration
	BaseURL             string
	Models              *data.Models
}

type ChallengeRequest struct {
	Account      string `json:"account" query:"account"`
	Memo         string `json:"memo,omitempty" query:"memo"`
	HomeDomain   string `json:"home_domain,omitempty" query:"home_domain"`
	ClientDomain string `json:"client_domain,omitempty" query:"client_domain"`
}

type ChallengeResponse struct {
	Transaction       string `json:"transaction"`
	NetworkPassphrase string `json:"network_passphrase"`
}

type ValidationRequest struct {
	Transaction string `json:"transaction" form:"transaction"`
}

type ValidationResponse struct {
	Token string `json:"token"`
}

func NewSEP10Service(
	jwtManager *anchorplatform.JWTManager,
	networkPassphrase string,
	sep10SigningPrivateKey string,
	authTimeout time.Duration,
	jwtExpiration time.Duration,
	baseURL string,
	models *data.Models,
) (*SEP10Service, error) {
	kp, err := keypair.ParseFull(sep10SigningPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("parsing sep10 signing key: %w", err)
	}

	return &SEP10Service{
		JWTManager:          jwtManager,
		JWTExpiration:       jwtExpiration,
		NetworkPassphrase:   networkPassphrase,
		Sep10SigningKeypair: kp,
		AuthTimeout:         authTimeout,
		BaseURL:             baseURL,
		Models:              models,
	}, nil
}

func (s *SEP10Service) CreateChallenge(ctx context.Context, req ChallengeRequest) (*ChallengeResponse, error) {
	if !strkey.IsValidEd25519PublicKey(req.Account) {
		return nil, fmt.Errorf("invalid account: not a valid ed25519 public key")
	}

	allowedHomeDomains, webAuthDomain := s.getAllowedHomeDomains(ctx)

	if req.HomeDomain == "" {
		if len(allowedHomeDomains) > 0 {
			req.HomeDomain = allowedHomeDomains[0]
		} else {
			return nil, fmt.Errorf("home_domain is required")
		}
	}

	if !s.isValidHomeDomain(ctx, req.HomeDomain) {
		return nil, fmt.Errorf("invalid home_domain: must match %s", s.getBaseDomain(ctx))
	}

	// Validate client domain (wallet domain) if provided
	if req.ClientDomain != "" {
		if err := s.validateClientDomain(ctx, req.ClientDomain); err != nil {
			return nil, fmt.Errorf("invalid client_domain: %w", err)
		}
	}

	var memoParam *txnbuild.MemoID
	if req.Memo != "" {
		parsed, err := strconv.ParseUint(req.Memo, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid memo: must be a positive integer")
		}
		memo := txnbuild.MemoID(parsed)
		memoParam = &memo
	}

	tx, err := txnbuild.BuildChallengeTx(
		s.Sep10SigningKeypair.Seed(),
		req.Account,
		webAuthDomain,
		req.HomeDomain,
		s.NetworkPassphrase,
		s.AuthTimeout,
		memoParam,
	)
	if err != nil {
		return nil, fmt.Errorf("building challenge transaction: %w", err)
	}

	if req.ClientDomain != "" {
		tx, err = s.addClientDomainOperation(tx, req.ClientDomain)
		if err != nil {
			return nil, fmt.Errorf("adding client domain: %w", err)
		}
	}

	txBase64, err := tx.Base64()
	if err != nil {
		return nil, fmt.Errorf("encoding transaction: %w", err)
	}

	return &ChallengeResponse{
		Transaction:       txBase64,
		NetworkPassphrase: s.NetworkPassphrase,
	}, nil
}

func (s *SEP10Service) ValidateChallenge(ctx context.Context, req ValidationRequest) (*ValidationResponse, error) {
	if req.Transaction == "" {
		return nil, fmt.Errorf("transaction is required")
	}

	allowedHomeDomains, webAuthDomain := s.getAllowedHomeDomains(ctx)

	tx, clientAccountID, matchedHomeDomain, memo, err := txnbuild.ReadChallengeTx(
		req.Transaction,
		s.Sep10SigningKeypair.Address(),
		s.NetworkPassphrase,
		webAuthDomain,
		allowedHomeDomains,
	)
	if err != nil {
		return nil, fmt.Errorf("reading challenge transaction: %w", err)
	}

	signers := []string{clientAccountID}

	clientDomain := s.extractClientDomain(tx)

	if clientDomain != "" {
		if err := s.validateClientDomain(ctx, clientDomain); err != nil {
			log.Ctx(ctx).Warnf("Client domain validation failed: %v", err)
			return nil, fmt.Errorf("invalid client_domain in transaction: %w", err)
		}
	}

	signersFound, err := txnbuild.VerifyChallengeTxSigners(
		req.Transaction,
		s.Sep10SigningKeypair.Address(),
		s.NetworkPassphrase,
		webAuthDomain,
		allowedHomeDomains,
		signers...,
	)
	if err != nil {
		return nil, fmt.Errorf("verifying challenge signatures: %w", err)
	}

	if len(signersFound) == 0 {
		return nil, fmt.Errorf("transaction not signed by client")
	}

	// Build subject (account or account:memo)
	subject := clientAccountID
	if memo != nil {
		memoID := uint64(*memo)
		subject = fmt.Sprintf("%s:%d", clientAccountID, memoID)
	}

	issuer := fmt.Sprintf("https://%s/auth", matchedHomeDomain)

	// Get transaction hash for jti
	jti, err := tx.HashHex(s.NetworkPassphrase)
	if err != nil {
		return nil, fmt.Errorf("getting transaction hash: %w", err)
	}

	timebounds := tx.Timebounds()
	iat := time.Unix(int64(timebounds.MinTime), 0)
	exp := iat.Add(s.JWTExpiration)

	token, err := s.JWTManager.GenerateSEP10Token(
		issuer,
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

	log.Ctx(ctx).Debugf("SEP-10 token generated for account=%s, clientDomain=%s",
		clientAccountID, clientDomain)

	return &ValidationResponse{Token: token}, nil
}

func (s *SEP10Service) getAllowedHomeDomains(ctx context.Context) ([]string, string) {
	baseDomain := s.getBaseDomain(ctx)

	// For SEP-10, we support wildcard matching: *.<base_domain>
	// This means any subdomain of the base domain is allowed
	allowedDomains := []string{baseDomain}

	return allowedDomains, baseDomain
}

func (s *SEP10Service) getBaseDomain(ctx context.Context) string {
	currentTenant, err := tenant.GetTenantFromContext(ctx)
	if err == nil && currentTenant != nil && currentTenant.BaseURL != nil {
		parsedURL, err := url.Parse(*currentTenant.BaseURL)
		if err == nil {
			return parsedURL.Hostname()
		}
	}

	parsedURL, err := url.Parse(s.BaseURL)
	if err == nil {
		return parsedURL.Hostname()
	}

	return ""
}

func (s *SEP10Service) isValidHomeDomain(ctx context.Context, homeDomain string) bool {
	baseDomain := s.getBaseDomain(ctx)
	return homeDomain == baseDomain || strings.HasSuffix(homeDomain, "."+baseDomain)
}

func (s *SEP10Service) validateClientDomain(ctx context.Context, clientDomain string) error {
	wallets, err := s.Models.Wallets.FindWallets(ctx, data.NewFilter(data.FilterEnabledWallets, true))
	if err != nil {
		return fmt.Errorf("fetching wallets: %w", err)
	}

	for _, wallet := range wallets {
		if wallet.SEP10ClientDomain == clientDomain {
			return nil
		}
	}

	return fmt.Errorf("client domain %q not found in registered wallets", clientDomain)
}

func (s *SEP10Service) addClientDomainOperation(tx *txnbuild.Transaction, clientDomain string) (*txnbuild.Transaction, error) {
	clientDomainOp := &txnbuild.ManageData{
		SourceAccount: s.Sep10SigningKeypair.Address(),
		Name:          "client_domain",
		Value:         []byte(clientDomain),
	}

	// Get existing operations and append	
	ops := tx.Operations()
	ops = append(ops, clientDomainOp)

	// Get the source account
	sourceAccount := tx.SourceAccount()

	// Rebuild transaction with new operations
	newTx, err := txnbuild.NewTransaction(
		txnbuild.TransactionParams{
			SourceAccount:        &sourceAccount,
			IncrementSequenceNum: false,
			Operations:           ops,
			BaseFee:              tx.BaseFee(),
			Memo:                 tx.Memo(),
			Preconditions: txnbuild.Preconditions{
				TimeBounds: tx.Timebounds(),
			},
		},
	)
	if err != nil {
		return nil, err
	}

	newTx, err = newTx.Sign(s.NetworkPassphrase, s.Sep10SigningKeypair)
	if err != nil {
		return nil, err
	}

	return newTx, nil
}

func (s *SEP10Service) extractClientDomain(tx *txnbuild.Transaction) string {
	for _, op := range tx.Operations() {
		if md, ok := op.(*txnbuild.ManageData); ok && md.Name == "client_domain" {
			return string(md.Value)
		}
	}
	return ""
}
