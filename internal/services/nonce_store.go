package services

import (
	"context"
	"fmt"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
)

//go:generate mockery --name=NonceStoreInterface --case=underscore --structname=MockNonceStore --filename=nonce_store.go

// NonceStoreInterface defines the interface for nonce storage.
type NonceStoreInterface interface {
	Store(ctx context.Context, nonce string) error
	Consume(ctx context.Context, nonce string) (bool, error)
}

var _ NonceStoreInterface = (*nonceStore)(nil)

type nonceStore struct {
	model *data.SEPNonceModel
	ttl   time.Duration
}

// NewNonceStore creates a new NonceStore.
func NewNonceStore(dbConnectionPool db.DBConnectionPool, ttl time.Duration) (NonceStoreInterface, error) {
	if dbConnectionPool == nil {
		return nil, fmt.Errorf("dbConnectionPool cannot be nil")
	}
	if ttl <= 0 {
		return nil, fmt.Errorf("ttl must be greater than zero")
	}

	store := &nonceStore{
		model: data.NewSEPNonceModel(dbConnectionPool),
		ttl:   ttl,
	}

	return store, nil
}

// Store saves a nonce in the database with an expiration time.
func (s *nonceStore) Store(ctx context.Context, nonce string) error {
	if nonce == "" {
		return fmt.Errorf("nonce cannot be empty")
	}
	expiresAt := time.Now().UTC().Add(s.ttl)
	if err := s.model.Store(ctx, nonce, expiresAt); err != nil {
		return fmt.Errorf("storing nonce: %w", err)
	}
	return nil
}

// Consume validates and deletes a nonce in a single operation.
func (s *nonceStore) Consume(ctx context.Context, nonce string) (bool, error) {
	if nonce == "" {
		return false, fmt.Errorf("nonce cannot be empty")
	}

	expiresAt, ok, err := s.model.Consume(ctx, nonce)
	if err != nil {
		return false, fmt.Errorf("consuming nonce: %w", err)
	}
	if !ok {
		return false, nil
	}

	if time.Now().UTC().After(expiresAt) {
		return false, nil
	}
	return true, nil
}
