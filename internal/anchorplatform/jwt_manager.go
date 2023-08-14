package anchorplatform

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v4"
)

var ErrInvalidToken = fmt.Errorf("invalid token")

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
func (manager *JWTManager) GenerateSEP24Token(stellarAccount, stellarMemo, clientDomain, transactionID string) (string, error) {
	subject := stellarAccount
	if stellarMemo != "" {
		subject = fmt.Sprintf("%s:%s", stellarAccount, stellarMemo)
	}

	claims := SEP24JWTClaims{
		ClientDomainClaim: clientDomain,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        transactionID,
			Subject:   subject,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Millisecond * time.Duration(manager.expirationMiliseconds))),
		},
	}
	err := claims.Valid()
	if err != nil {
		return "", fmt.Errorf("validating SEP24 token claims: %w", err)
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, err := token.SignedString(manager.secret)
	if err != nil {
		return "", fmt.Errorf("signing SEP24 token: %w", err)
	}

	return signedToken, nil
}

// ParseSEP24TokenClaims will parse the provided token string and return the SEP24JWTClaims, if possible.
// If the token is not a valid SEP-24 token, an error is returned instead.
func (manager *JWTManager) ParseSEP24TokenClaims(tokenString string) (*SEP24JWTClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &SEP24JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		return manager.secret, nil
	})
	if err != nil {
		return nil, fmt.Errorf("parsing SEP24 token: %w", err)
	}

	claims, ok := token.Claims.(*SEP24JWTClaims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

// GenerateDefaultToken will generate a JWT token string using the token manager and only the default claims.
func (manager *JWTManager) GenerateDefaultToken(id string) (string, error) {
	claims := jwt.RegisteredClaims{
		ID:        id,
		Subject:   "stellar-disbursement-platform-backend",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Millisecond * time.Duration(manager.expirationMiliseconds))),
	}
	err := claims.Valid()
	if err != nil {
		return "", fmt.Errorf("validating token claims: %w", err)
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, err := token.SignedString(manager.secret)
	if err != nil {
		return "", fmt.Errorf("signing default token: %w", err)
	}

	return signedToken, nil
}

// ParseDefaultTokenClaims will parse the default claims from a JWT token string.
func (manager *JWTManager) ParseDefaultTokenClaims(tokenString string) (*jwt.RegisteredClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &jwt.RegisteredClaims{}, func(token *jwt.Token) (interface{}, error) {
		return manager.secret, nil
	})
	if err != nil {
		return nil, fmt.Errorf("parsing default token: %w", err)
	}

	claims, ok := token.Claims.(*jwt.RegisteredClaims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}
