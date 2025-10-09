package wallet

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"time"

	jwtgo "github.com/golang-jwt/jwt/v4"
)

var (
	ErrInvalidWalletToken = errors.New("invalid wallet token")
	ErrExpiredWalletToken = errors.New("expired wallet token")
	ErrMissingSubClaim    = errors.New("missing sub claim in wallet token")
)

// WalletJWTManager defines the interface for wallet JWT token operations.
//
//go:generate mockery --name=WalletJWTManager --case=underscore --structname=MockWalletJWTManager --filename=jwt_manager.go
type WalletJWTManager interface {
	GenerateToken(ctx context.Context, contractAddress string, expiresAt time.Time) (string, error)
	ValidateToken(ctx context.Context, tokenString string) (contractAddress string, err error)
}

type walletClaims struct {
	jwtgo.RegisteredClaims
}

type defaultWalletJWTManager struct {
	privateKey *ecdsa.PrivateKey
}

// NewWalletJWTManager creates a new WalletJWTManager instance.
func NewWalletJWTManager(privateKeyPEM string) (WalletJWTManager, error) {
	esPrivateKey, err := jwtgo.ParseECPrivateKeyFromPEM([]byte(privateKeyPEM))
	if err != nil {
		return nil, fmt.Errorf("parsing EC private key: %w", err)
	}

	return &defaultWalletJWTManager{
		privateKey: esPrivateKey,
	}, nil
}

var _ WalletJWTManager = (*defaultWalletJWTManager)(nil)

func (m *defaultWalletJWTManager) GenerateToken(ctx context.Context, contractAddress string, expiresAt time.Time) (string, error) {
	claims := &walletClaims{
		RegisteredClaims: jwtgo.RegisteredClaims{
			Subject:   contractAddress,
			ExpiresAt: jwtgo.NewNumericDate(expiresAt),
			IssuedAt:  jwtgo.NewNumericDate(time.Now()),
		},
	}

	token := jwtgo.NewWithClaims(jwtgo.SigningMethodES256, claims)

	tokenString, err := token.SignedString(m.privateKey)
	if err != nil {
		return "", fmt.Errorf("signing token: %w", err)
	}

	return tokenString, nil
}

func (m *defaultWalletJWTManager) ValidateToken(ctx context.Context, tokenString string) (contractAddress string, err error) {
	claims := &walletClaims{}

	token, err := jwtgo.ParseWithClaims(tokenString, claims, func(t *jwtgo.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwtgo.SigningMethodECDSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}

		return &m.privateKey.PublicKey, nil
	})
	if err != nil {
		if errors.Is(err, jwtgo.ErrTokenExpired) {
			return "", ErrExpiredWalletToken
		}

		if vErr, ok := err.(*jwtgo.ValidationError); ok {
			if vErr.Errors&jwtgo.ValidationErrorExpired != 0 {
				return "", ErrExpiredWalletToken
			}
		}

		return "", fmt.Errorf("%w: %v", ErrInvalidWalletToken, err)
	}

	if !token.Valid {
		return "", ErrInvalidWalletToken
	}

	if claims.Subject == "" {
		return "", ErrMissingSubClaim
	}

	return claims.Subject, nil
}
