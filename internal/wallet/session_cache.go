package wallet

import (
	"fmt"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/hashicorp/golang-lru/v2/expirable"
)

type storedSession struct {
	Type    SessionType          `json:"type"`
	Session webauthn.SessionData `json:"session"`
}

// SessionCacheInterface defines the interface for WebAuthn session storage.
type SessionCacheInterface interface {
	Store(key string, sessType SessionType, session webauthn.SessionData) error
	Get(key string, expectedType SessionType) (webauthn.SessionData, error)
	Delete(key string)
}

var _ SessionCacheInterface = (*InMemorySessionCache)(nil)

// InMemorySessionCache provides in-memory storage for WebAuthn sessions.
type InMemorySessionCache struct {
	cache *expirable.LRU[string, storedSession]
}

// NewInMemorySessionCache creates a new InMemorySessionCache.
func NewInMemorySessionCache(defaultExpiration time.Duration, maxEntries int) (*InMemorySessionCache, error) {
	if maxEntries <= 0 {
		return nil, fmt.Errorf("maxEntries must be greater than zero")
	}
	if defaultExpiration <= 0 {
		return nil, fmt.Errorf("defaultExpiration must be greater than zero")
	}

	return &InMemorySessionCache{
		cache: expirable.NewLRU[string, storedSession](maxEntries, nil, defaultExpiration),
	}, nil
}

// Store saves a WebAuthn session in the cache.
func (sc *InMemorySessionCache) Store(key string, sessType SessionType, session webauthn.SessionData) error {
	stored := storedSession{
		Type:    sessType,
		Session: session,
	}

	sc.cache.Add(key, stored)
	return nil
}

// Get retrieves a WebAuthn session from the cache and checks its type.
func (sc *InMemorySessionCache) Get(key string, expectedType SessionType) (webauthn.SessionData, error) {
	stored, found := sc.cache.Get(key)
	if !found {
		return webauthn.SessionData{}, ErrSessionNotFound
	}

	if stored.Type != expectedType {
		return webauthn.SessionData{}, ErrSessionTypeMismatch
	}

	return stored.Session, nil
}

// Delete removes a WebAuthn session from the cache.
func (sc *InMemorySessionCache) Delete(key string) {
	sc.cache.Remove(key)
}
