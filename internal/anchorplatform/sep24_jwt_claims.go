package anchorplatform

import (
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/stellar/go/keypair"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

type SEP24JWTClaims struct {
	// Fields expected according with https://github.com/stellar/java-stellar-anchor-sdk/blob/bfa9b1d735f099bc6a21f0b9c55bd381a50c16b8/platform/src/main/java/org/stellar/anchor/platform/service/SimpleInteractiveUrlConstructor.java#L47-L56
	ClientDomainClaim string `json:"client_domain"`
	jwt.RegisteredClaims
}

func (c *SEP24JWTClaims) TransactionID() string {
	return c.ID
}

func (c *SEP24JWTClaims) SEP10StellarAccount() string {
	// The SEP-10 account will be in the format "account:memo", in case there's a memo.
	// That's why we'll split the string on ":" and get the first element.
	// ref: https://github.com/stellar/java-stellar-anchor-sdk/blob/bfa9b1d735f099bc6a21f0b9c55bd381a50c16b8/platform/src/main/java/org/stellar/anchor/platform/service/SimpleInteractiveUrlConstructor.java#L47-L50
	splits := strings.Split(c.Subject, ":")
	return splits[0]
}

func (c *SEP24JWTClaims) SEP10StellarMemo() string {
	// The SEP-10 account will be in the format "account:memo", in case there's a memo.
	// That's why we'll split the string on ":" and get the second element.
	// ref: https://github.com/stellar/java-stellar-anchor-sdk/blob/bfa9b1d735f099bc6a21f0b9c55bd381a50c16b8/platform/src/main/java/org/stellar/anchor/platform/service/SimpleInteractiveUrlConstructor.java#L47-L50
	splits := strings.Split(c.Subject, ":")
	if len(splits) > 1 {
		return splits[1]
	}
	return ""
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

	_, err = keypair.ParseAddress(c.SEP10StellarAccount())
	if err != nil {
		return fmt.Errorf("stellar account is invalid: %w", err)
	}

	if c.ClientDomain() != "" {
		err = utils.ValidateDNS(c.ClientDomain())
		if err != nil {
			return fmt.Errorf("client_domain is invalid: %w", err)
		}
	}

	return nil
}
