package services

import (
	"context"
)

type EmbeddedWalletService struct {
}

func NewEmbeddedWalletService() *EmbeddedWalletService {
	return &EmbeddedWalletService{}
}

func (s *EmbeddedWalletService) QueueAccountCreation(ctx context.Context, credentialID, publicKey, claimToken string) error {
	return nil
}
