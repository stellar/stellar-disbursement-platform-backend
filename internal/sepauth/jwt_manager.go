package sepauth

import (
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v4"
)

var ErrInvalidToken = fmt.Errorf("invalid token")

type Sep10JWTClaims struct {
	jwt.RegisteredClaims
	ClientDomain string `json:"client_domain,omitempty"`
	HomeDomain   string `json:"home_domain,omitempty"`
}

func (c Sep10JWTClaims) Valid() error {
	if c.Issuer == "" {
		return fmt.Errorf("issuer is required")
	}

	if c.Subject == "" {
		return fmt.Errorf("subject is required")
	}

	if c.ID == "" {
		return fmt.Errorf("jti (JWT ID) is required")
	}

	if c.IssuedAt == nil {
		return fmt.Errorf("iat (issued at) is required")
	}

	if c.ExpiresAt == nil {
		return fmt.Errorf("exp (expires at) is required")
	}

	err := c.RegisteredClaims.Valid()
	if err != nil {
		return fmt.Errorf("validating registered claims: %w", err)
	}

	if c.ClientDomain != "" && len(strings.TrimSpace(c.ClientDomain)) == 0 {
		return fmt.Errorf("client_domain cannot be empty if provided")
	}

	if c.HomeDomain != "" && len(strings.TrimSpace(c.HomeDomain)) == 0 {
		return fmt.Errorf("home_domain cannot be empty if provided")
	}

	return nil
}

type JWTManager struct {
	secret                []byte
	expirationMiliseconds int64
}

// NewJWTManager creates a new JWTManager instance based on the provided secret and expirationMiliseconds.
func NewJWTManager(secret string, expirationMiliseconds int64) (*JWTManager, error) {
	const minSecretSize = 12
	if len(secret) < minSecretSize {
		return nil, fmt.Errorf("secret is required to have at least %d characteres", minSecretSize)
	}

	const minExpirationMiliseconds = 5000
	if expirationMiliseconds < minExpirationMiliseconds {
		return nil, fmt.Errorf("expiration miliseconds is required to be at least %d", minExpirationMiliseconds)
	}

	return &JWTManager{secret: []byte(secret), expirationMiliseconds: expirationMiliseconds}, nil
}

// GenerateSEP24Token will generate a JWT token string using the token manager and the provided parameters.
// The parameters are validated before generating the token.
func (manager *JWTManager) GenerateSEP24Token(stellarAccount, stellarMemo, clientDomain, homeDomain, transactionID string) (string, error) {
	subject := stellarAccount
	if stellarMemo != "" {
		subject = fmt.Sprintf("%s:%s", stellarAccount, stellarMemo)
	}

	claims := SEP24JWTClaims{
		ClientDomainClaim: clientDomain,
		HomeDomainClaim:   homeDomain,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        transactionID,
			Subject:   subject,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Millisecond * time.Duration(manager.expirationMiliseconds))),
		},
	}

	token, err := manager.signToken(claims)
	if err != nil {
		return "", fmt.Errorf("generating SEP24 token: %w", err)
	}
	return token, nil
}

// GenerateSEP24MoreInfoToken will generate a JWT token string for more info URLs with transaction data.
func (manager *JWTManager) GenerateSEP24MoreInfoToken(stellarAccount, stellarMemo, clientDomain, homeDomain, transactionID, lang string, transactionData map[string]string) (string, error) {
	subject := stellarAccount
	if stellarMemo != "" {
		subject = fmt.Sprintf("%s:%s", stellarAccount, stellarMemo)
	}

	// Prepare transaction data including language
	data := make(map[string]string)
	if lang != "" {
		data["lang"] = lang
	}

	// Add provided transaction data
	maps.Copy(data, transactionData)

	claims := SEP24JWTClaims{
		ClientDomainClaim: clientDomain,
		HomeDomainClaim:   homeDomain,
		TransactionData:   data,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        transactionID,
			Subject:   subject,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Millisecond * time.Duration(manager.expirationMiliseconds))),
		},
	}

	token, err := manager.signToken(claims)
	if err != nil {
		return "", fmt.Errorf("generating SEP24 more info token: %w", err)
	}
	return token, nil
}

func (manager *JWTManager) GenerateSEP10Token(issuer, subject, jti, clientDomain, homeDomain string, iat, exp time.Time) (string, error) {
	claims := Sep10JWTClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Subject:   subject,
			ID:        jti,
			IssuedAt:  jwt.NewNumericDate(iat),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
		ClientDomain: clientDomain,
		HomeDomain:   homeDomain,
	}

	token, err := manager.signToken(claims)
	if err != nil {
		return "", fmt.Errorf("generating SEP10 token: %w", err)
	}
	return token, nil
}

// ParseDefaultTokenClaims will parse the default claims from a JWT token string.
func (manager *JWTManager) ParseDefaultTokenClaims(tokenString string) (*jwt.RegisteredClaims, error) {
	return parseTokenClaims(manager.secret, tokenString, &jwt.RegisteredClaims{}, "default")
}

// ParseSEP10TokenClaims will parse the provided token string and return the Sep10JWTClaims, if possible.
// If the token is not a valid SEP-10 token, an error is returned instead.
func (manager *JWTManager) ParseSEP10TokenClaims(tokenString string) (*Sep10JWTClaims, error) {
	return parseTokenClaims(manager.secret, tokenString, &Sep10JWTClaims{}, "SEP10")
}

// ParseSEP24TokenClaims will parse the provided token string and return the SEP24JWTClaims, if possible.
// If the token is not a valid SEP-24 token, an error is returned instead.
func (manager *JWTManager) ParseSEP24TokenClaims(tokenString string) (*SEP24JWTClaims, error) {
	return parseTokenClaims(manager.secret, tokenString, &SEP24JWTClaims{}, "SEP24")
}

func (manager *JWTManager) signToken(claims jwt.Claims) (string, error) {
	if validator, ok := claims.(interface{ Valid() error }); ok {
		if err := validator.Valid(); err != nil {
			return "", fmt.Errorf("validating token claims: %w", err)
		}
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, err := token.SignedString(manager.secret)
	if err != nil {
		return "", fmt.Errorf("signing token: %w", err)
	}

	return signedToken, nil
}

func parseTokenClaims[T jwt.Claims](secret []byte, tokenString string, claims T, operation string) (T, error) {
	var zero T

	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (any, error) {
		return secret, nil
	})
	if err != nil {
		return zero, fmt.Errorf("parsing %s token: %w", operation, err)
	}

	parsedClaims, ok := token.Claims.(T)
	if !ok || !token.Valid {
		return zero, ErrInvalidToken
	}

	return parsedClaims, nil
}
