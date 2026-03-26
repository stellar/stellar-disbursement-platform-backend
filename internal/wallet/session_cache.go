package wallet

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/stellar/go-stellar-sdk/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
)

// SessionCacheInterface defines the interface for WebAuthn session storage.
type SessionCacheInterface interface {
	// Store saves a WebAuthn session by key and type.
	Store(ctx context.Context, key string, sessType SessionType, session webauthn.SessionData) error
	// Get retrieves a WebAuthn session by key and validates its type.
	Get(ctx context.Context, key string, expectedType SessionType) (webauthn.SessionData, error)
	// Delete removes a WebAuthn session by key.
	Delete(ctx context.Context, key string)
}

var _ SessionCacheInterface = (*sessionCache)(nil)

type sessionCache struct {
	model *data.PasskeySessionModel
	ttl   time.Duration
}

// NewSessionCache creates a new SessionCache backed by the passkey sessions table.
func NewSessionCache(dbConnectionPool db.DBConnectionPool, ttl time.Duration) (SessionCacheInterface, error) {
	if dbConnectionPool == nil {
		return nil, fmt.Errorf("dbConnectionPool cannot be nil")
	}
	if ttl <= 0 {
		return nil, fmt.Errorf("ttl must be greater than zero")
	}

	cache := &sessionCache{
		model: data.NewPasskeySessionModel(dbConnectionPool),
		ttl:   ttl,
	}

	return cache, nil
}

func (sc *sessionCache) Store(ctx context.Context, key string, sessType SessionType, session webauthn.SessionData) error {
	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}

	sessionBytes, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("marshaling session: %w", err)
	}

	expiresAt := time.Now().UTC().Add(sc.ttl)
	if err := sc.model.Store(ctx, key, string(sessType), sessionBytes, expiresAt); err != nil {
		return fmt.Errorf("storing session: %w", err)
	}

	return nil
}

func (sc *sessionCache) Get(ctx context.Context, key string, expectedType SessionType) (webauthn.SessionData, error) {
	if key == "" {
		return webauthn.SessionData{}, ErrSessionNotFound
	}

	session, err := sc.model.Get(ctx, key)
	if err != nil {
		return webauthn.SessionData{}, fmt.Errorf("retrieving session: %w", err)
	}
	if session == nil {
		return webauthn.SessionData{}, ErrSessionNotFound
	}
	if !session.ExpiresAt.After(time.Now().UTC()) {
		if delErr := sc.model.Delete(ctx, key); delErr != nil {
			return webauthn.SessionData{}, fmt.Errorf("deleting expired session: %w", delErr)
		}
		return webauthn.SessionData{}, ErrSessionNotFound
	}
	if session.SessionType != string(expectedType) {
		return webauthn.SessionData{}, ErrSessionTypeMismatch
	}

	var sessionData webauthn.SessionData
	if err := json.Unmarshal(session.SessionData, &sessionData); err != nil {
		return webauthn.SessionData{}, fmt.Errorf("unmarshaling session: %w", err)
	}

	return sessionData, nil
}

func (sc *sessionCache) Delete(ctx context.Context, key string) {
	if key == "" {
		return
	}

	if err := sc.model.Delete(ctx, key); err != nil {
		log.Ctx(ctx).Errorf("deleting passkey session %s: %v", key, err)
	}
}
