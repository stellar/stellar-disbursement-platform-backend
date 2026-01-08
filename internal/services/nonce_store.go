package services

import (
	"fmt"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
)

const DefaultNonceCacheMaxEntries = 1000

// NonceStore defines the interface for nonce storage.
type NonceStore interface {
	Store(nonce string) error
	Consume(nonce string) (bool, error)
}

var _ NonceStore = (*InMemoryNonceStore)(nil)

// InMemoryNonceStore provides in-memory storage for nonces.
type InMemoryNonceStore struct {
	cache *expirable.LRU[string, struct{}]
}

// NewInMemoryNonceStore creates a new InMemoryNonceStore.
func NewInMemoryNonceStore(defaultExpiration time.Duration, maxEntries int) (*InMemoryNonceStore, error) {
	if maxEntries <= 0 {
		return nil, fmt.Errorf("maxEntries must be greater than zero")
	}
	if defaultExpiration <= 0 {
		return nil, fmt.Errorf("defaultExpiration must be greater than zero")
	}

	return &InMemoryNonceStore{
		cache: expirable.NewLRU[string, struct{}](maxEntries, nil, defaultExpiration),
	}, nil
}

// Store saves a nonce in the cache.
func (s *InMemoryNonceStore) Store(nonce string) error {
	if nonce == "" {
		return fmt.Errorf("nonce cannot be empty")
	}
	s.cache.Add(nonce, struct{}{})
	return nil
}

// Consume validates and deletes a nonce in a single operation.
func (s *InMemoryNonceStore) Consume(nonce string) (bool, error) {
	if nonce == "" {
		return false, fmt.Errorf("nonce cannot be empty")
	}
	if _, ok := s.cache.Get(nonce); !ok {
		return false, nil
	}
	s.cache.Remove(nonce)
	return true, nil
}
