package anchorplatform

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v4"
)

var ErrInvalidToken = fmt.Errorf("invalid token")

type Sep10JWTClaims struct {
	jwt.RegisteredClaims
	ClientDomain string `json:"client_domain,omitempty"`
	HomeDomain   string `json:"home_domain,omitempty"`
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

	return manager.signToken(claims, "SEP24", true)
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

	return manager.signToken(claims, "SEP10", false)
}

// GenerateDefaultToken will generate a JWT token string using the token manager and only the default claims.
func (manager *JWTManager) GenerateDefaultToken(id string) (string, error) {
	claims := jwt.RegisteredClaims{
		ID:        id,
		Subject:   "stellar-disbursement-platform-backend",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Millisecond * time.Duration(manager.expirationMiliseconds))),
	}

	return manager.signToken(claims, "default", true)
}

// ParseDefaultTokenClaims will parse the default claims from a JWT token string.
func (manager *JWTManager) ParseDefaultTokenClaims(tokenString string) (*jwt.RegisteredClaims, error) {
	return parseTokenClaims(manager.secret, tokenString, &jwt.RegisteredClaims{}, "default")
}

func (manager *JWTManager) ParseSEP10TokenClaims(tokenString string) (*Sep10JWTClaims, error) {
	return parseTokenClaims(manager.secret, tokenString, &Sep10JWTClaims{}, "SEP10")
}

// ParseSEP24TokenClaims will parse the provided token string and return the SEP24JWTClaims, if possible.
// If the token is not a valid SEP-24 token, an error is returned instead.
func (manager *JWTManager) ParseSEP24TokenClaims(tokenString string) (*SEP24JWTClaims, error) {
	return parseTokenClaims(manager.secret, tokenString, &SEP24JWTClaims{}, "SEP24")
}

// signToken handles the common token signing logic
func (manager *JWTManager) signToken(claims jwt.Claims, tokenType string, validate bool) (string, error) {
	if validate {
		if validator, ok := claims.(interface{ Valid() error }); ok {
			if err := validator.Valid(); err != nil {
				return "", fmt.Errorf("validating %s token claims: %w", tokenType, err)
			}
		}
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, err := token.SignedString(manager.secret)
	if err != nil {
		return "", fmt.Errorf("signing %s token: %w", tokenType, err)
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
