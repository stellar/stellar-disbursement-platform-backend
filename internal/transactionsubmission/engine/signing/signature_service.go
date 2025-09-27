package signing

import (
	"fmt"

	sdpUtils "github.com/stellar/stellar-disbursement-platform-backend/internal/utils"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/preconditions"
)

type SignatureService struct {
	SignerRouter
	DistributionAccountResolver
	networkPassphrase string
}

var _ DistributionAccountResolver = (*SignatureService)(nil)

var _ SignerRouter = (*SignatureService)(nil)

func (s *SignatureService) Validate() error {
	if s.SignerRouter == nil {
		return fmt.Errorf("signer router cannot be nil")
	}

	if len(s.SupportedAccountTypes()) == 0 {
		return fmt.Errorf("signer router must support at least one account type")
	}

	if s.DistributionAccountResolver == nil {
		return fmt.Errorf("distribution account resolver cannot be nil")
	}

	return nil
}

type SignatureServiceOptions struct {
	// Shared:
	NetworkPassphrase string

	// DistributionAccountEnv:
	DistributionPrivateKey string

	// DistributionAccountDB:
	DistAccEncryptionPassphrase string

	// ChannelAccountDB:
	ChAccEncryptionPassphrase string

	// *AccountDB:
	DBConnectionPool    db.DBConnectionPool
	LedgerNumberTracker preconditions.LedgerNumberTracker
	Encrypter           sdpUtils.PrivateKeyEncrypter

	// DistributionAccountResolver
	DistributionAccountResolver
}

// NewSignatureService creates a new signature service instance, given the distribution signer type and the options.
func NewSignatureService(opts SignatureServiceOptions) (SignatureService, error) {
	if opts.DistributionAccountResolver == nil {
		return SignatureService{}, fmt.Errorf("distribution account resolver cannot be nil")
	}

	sigRouterOpts := SignatureRouterOptions{
		NetworkPassphrase:           opts.NetworkPassphrase,
		HostPrivateKey:              opts.DistributionPrivateKey, // TODO: pass it from the outside
		DistributionPrivateKey:      opts.DistributionPrivateKey,
		DBConnectionPool:            opts.DBConnectionPool,
		ChAccEncryptionPassphrase:   opts.ChAccEncryptionPassphrase,
		DistAccEncryptionPassphrase: opts.DistAccEncryptionPassphrase,
		Encrypter:                   opts.Encrypter,
		LedgerNumberTracker:         opts.LedgerNumberTracker,
	}

	sigRouter, err := NewSignerRouter(sigRouterOpts)
	if err != nil {
		return SignatureService{}, fmt.Errorf("creating a new signer router: %w", err)
	}

	return SignatureService{
		SignerRouter:                sigRouter,
		DistributionAccountResolver: opts.DistributionAccountResolver,
		networkPassphrase:           opts.NetworkPassphrase,
	}, nil
}

func (s *SignatureService) NetworkPassphrase() string {
	return s.networkPassphrase
}
