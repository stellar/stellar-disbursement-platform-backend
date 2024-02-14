package signing

import (
	"fmt"

	"golang.org/x/exp/slices"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/preconditions"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/utils"
)

type SignatureService struct {
	ChAccountSigner   SignatureClient
	DistAccountSigner SignatureClient
	HostAccountSigner SignatureClient
	DistributionAccountResolver
}

var _ DistributionAccountResolver = (*SignatureService)(nil)

func (s *SignatureService) Validate() error {
	if s.ChAccountSigner == nil {
		return fmt.Errorf("channel account signer cannot be nil")
	}

	if s.DistAccountSigner == nil {
		return fmt.Errorf("distribution account signer cannot be nil")
	}

	if s.HostAccountSigner == nil {
		return fmt.Errorf("host account signer cannot be nil")
	}

	if s.ChAccountSigner.NetworkPassphrase() != s.DistAccountSigner.NetworkPassphrase() || s.DistAccountSigner.NetworkPassphrase() != s.HostAccountSigner.NetworkPassphrase() {
		return fmt.Errorf("network passphrase of all signers should be the same")
	}

	if s.DistributionAccountResolver == nil {
		return fmt.Errorf("distribution account resolver cannot be nil")
	}

	return nil
}

type SignatureServiceOptions struct {
	// Shared:
	NetworkPassphrase string

	// DistributionAccount:
	DistributionPrivateKey string
	// DistributionAccountEnv:
	DistributionSignerType SignatureClientType

	// ChannelAccountDB:
	DBConnectionPool     db.DBConnectionPool
	EncryptionPassphrase string
	LedgerNumberTracker  preconditions.LedgerNumberTracker
	Encrypter            utils.PrivateKeyEncrypter
}

// NewSignatureService creates a new signature service instance, given the distribution signer type and the options.
func NewSignatureService(opts SignatureServiceOptions) (SignatureService, error) {
	distSignerType := opts.DistributionSignerType
	if !slices.Contains(SignatureClientType("").AllDistribution(), distSignerType) {
		return SignatureService{}, fmt.Errorf("invalid distribution signer type %q", distSignerType)
	}

	sigClientOpts := SignatureClientOptions{
		NetworkPassphrase:      opts.NetworkPassphrase,
		DistributionPrivateKey: opts.DistributionPrivateKey,
		DBConnectionPool:       opts.DBConnectionPool,
		EncryptionPassphrase:   opts.EncryptionPassphrase,
		Encrypter:              opts.Encrypter,
		LedgerNumberTracker:    opts.LedgerNumberTracker,
	}

	chAccountSigner, err := NewSignatureClient(ChannelAccountDBSignatureClientType, sigClientOpts)
	if err != nil {
		return SignatureService{}, fmt.Errorf("creating a new channel account signature client: %w", err)
	}

	distAccSigner, err := NewSignatureClient(distSignerType, sigClientOpts)
	if err != nil {
		return SignatureService{}, fmt.Errorf("creating a new distribution account signature client: %w", err)
	}

	hostAccSigner, err := NewSignatureClient(HostAccountEnvSignatureClientType, sigClientOpts)
	if err != nil {
		return SignatureService{}, fmt.Errorf("creating a new host account signature client: %w", err)
	}

	distAccResolver, ok := distAccSigner.(DistributionAccountResolver)
	if !ok {
		return SignatureService{}, fmt.Errorf("trying to cast a distribution account signer to a distribution account resolver")
	}

	return SignatureService{
		ChAccountSigner:             chAccountSigner,
		DistAccountSigner:           distAccSigner,
		HostAccountSigner:           hostAccSigner,
		DistributionAccountResolver: distAccResolver,
	}, nil
}
