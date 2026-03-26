package sepauth

import (
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/stellar/go-stellar-sdk/strkey"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

type SEP24JWTClaims struct {
	ClientDomainClaim string            `json:"client_domain"`
	HomeDomainClaim   string            `json:"home_domain"`
	TransactionData   map[string]string `json:"data,omitempty"` // Transaction data including lang, kind, status, etc.
	jwt.RegisteredClaims
}

func (c *SEP24JWTClaims) TransactionID() string {
	return c.ID
}

// ParseAccountAndMemo splits a SEP-10 subject into Stellar account and memo.
// accounts with memo use the format "account:memo".
func ParseAccountAndMemo(subject string) (account, memo string) {
	if before, after, found := strings.Cut(subject, ":"); found && before != "" {
		return before, after
	}
	return subject, ""
}

// FormatSubject constructs a SEP-10 subject from account and optional memo.
func FormatSubject(stellarAccount, stellarMemo string) string {
	if stellarMemo != "" {
		return stellarAccount + ":" + stellarMemo
	}
	return stellarAccount
}

func (c *SEP24JWTClaims) Account() string {
	account, _ := ParseAccountAndMemo(c.Subject)
	return account
}

func (c *SEP24JWTClaims) Memo() string {
	_, memo := ParseAccountAndMemo(c.Subject)
	return memo
}

func (c *SEP24JWTClaims) ExpiresAt() *time.Time {
	if c.RegisteredClaims.ExpiresAt == nil {
		return nil
	}
	return &c.RegisteredClaims.ExpiresAt.Time
}

func (c *SEP24JWTClaims) ClientDomain() string {
	return c.ClientDomainClaim
}

func (c *SEP24JWTClaims) HomeDomain() string {
	return c.HomeDomainClaim
}

func (c SEP24JWTClaims) Valid() error {
	if c.ExpiresAt() == nil {
		return fmt.Errorf("expires_at is required")
	}

	err := c.RegisteredClaims.Valid()
	if err != nil {
		return fmt.Errorf("validating registered claims: %w", err)
	}

	if c.TransactionID() == "" {
		return fmt.Errorf("transaction_id is required")
	}

	stellarAccount := c.Account()
	if !strkey.IsValidEd25519PublicKey(stellarAccount) && !strkey.IsValidContractAddress(stellarAccount) {
		return fmt.Errorf("stellar account is invalid")
	}

	if c.ClientDomain() != "" {
		err = utils.ValidateDNS(c.ClientDomain())
		if err != nil {
			return fmt.Errorf("client_domain is invalid: %w", err)
		}
	}

	return nil
}
