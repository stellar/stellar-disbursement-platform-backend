package wallet

import (
	"fmt"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	cache "github.com/patrickmn/go-cache"
)

type storedSession struct {
	Type    SessionType          `json:"type"`
	Session webauthn.SessionData `json:"session"`
}

// SessionCacheInterface defines the interface for WebAuthn session storage.
type SessionCacheInterface interface {
	Store(key string, sessType SessionType, session webauthn.SessionData, ttl time.Duration) error
	Get(key string, expectedType SessionType) (webauthn.SessionData, error)
	Delete(key string)
}

var _ SessionCacheInterface = (*InMemorySessionCache)(nil)

// InMemorySessionCache provides in-memory storage for WebAuthn sessions.
type InMemorySessionCache struct {
	cache *cache.Cache
}

// NewInMemorySessionCache creates a new InMemorySessionCache.
func NewInMemorySessionCache(defaultExpiration, cleanupInterval time.Duration) *InMemorySessionCache {
	return &InMemorySessionCache{
		cache: cache.New(defaultExpiration, cleanupInterval),
	}
}

// Store saves a WebAuthn session in the cache with the specified TTL.
func (sc *InMemorySessionCache) Store(key string, sessType SessionType, session webauthn.SessionData, ttl time.Duration) error {
	stored := storedSession{
		Type:    sessType,
		Session: session,
	}

	sc.cache.Set(key, stored, ttl)
	return nil
}

// Get retrieves a WebAuthn session from the cache and checks its type.
func (sc *InMemorySessionCache) Get(key string, expectedType SessionType) (webauthn.SessionData, error) {
	item, found := sc.cache.Get(key)
	if !found {
		return webauthn.SessionData{}, ErrSessionNotFound
	}

	stored, ok := item.(storedSession)
	if !ok {
		return webauthn.SessionData{}, fmt.Errorf("invalid session data type in cache")
	}

	if stored.Type != expectedType {
		return webauthn.SessionData{}, ErrSessionTypeMismatch
	}

	return stored.Session, nil
}

// Delete removes a WebAuthn session from the cache.
func (sc *InMemorySessionCache) Delete(key string) {
	sc.cache.Delete(key)
}
