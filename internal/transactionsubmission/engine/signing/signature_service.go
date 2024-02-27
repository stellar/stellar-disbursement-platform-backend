package signing

import (
	"fmt"

	"golang.org/x/exp/slices"

	"github.com/stellar/go/keypair"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/preconditions"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/utils"
)

type SignatureService struct {
	ChAccountSigner   SignatureClient
	DistAccountSigner SignatureClient
	HostAccountSigner SignatureClient
	DistributionAccountResolver
	networkPassphrase string
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
	DistributionSignerType SignatureClientType

	// DistributionAccountEnv:
	DistributionPrivateKey string

	// DistributionAccountDB:
	DistAccEncryptionPassphrase string

	// ChannelAccountDB:
	ChAccEncryptionPassphrase string

	// *AccountDB:
	DBConnectionPool    db.DBConnectionPool
	LedgerNumberTracker preconditions.LedgerNumberTracker
	Encrypter           utils.PrivateKeyEncrypter

	// DistributionAccountResolver
	DistributionAccountResolver
}

// NewSignatureService creates a new signature service instance, given the distribution signer type and the options.
func NewSignatureService(opts SignatureServiceOptions) (SignatureService, error) {
	distSignerType := opts.DistributionSignerType
	if !slices.Contains(DistributionSignatureClientTypes(), distSignerType) {
		return SignatureService{}, fmt.Errorf("invalid distribution signer type %q", distSignerType)
	}

	sigClientOpts := SignatureClientOptions{
		NetworkPassphrase:           opts.NetworkPassphrase,
		DistributionPrivateKey:      opts.DistributionPrivateKey,
		DBConnectionPool:            opts.DBConnectionPool,
		ChAccEncryptionPassphrase:   opts.ChAccEncryptionPassphrase,
		DistAccEncryptionPassphrase: opts.DistAccEncryptionPassphrase,
		Encrypter:                   opts.Encrypter,
		LedgerNumberTracker:         opts.LedgerNumberTracker,
	}

	chAccountSigner, err := NewSignatureClient(ChannelAccountDBSignatureClientType, sigClientOpts)
	if err != nil {
		return SignatureService{}, fmt.Errorf("creating a new channel account signature client: %w", err)
	}

	distAccSigner, err := NewSignatureClient(distSignerType, sigClientOpts)
	if err != nil {
		return SignatureService{}, fmt.Errorf("creating a new distribution account signature client with type %v: %w", distSignerType, err)
	}

	hostAccSigner, err := NewSignatureClient(HostAccountEnvSignatureClientType, sigClientOpts)
	if err != nil {
		return SignatureService{}, fmt.Errorf("creating a new host account signature client: %w", err)
	}

	distAccResolver := opts.DistributionAccountResolver
	if distAccResolver == nil {
		hostAccKP, err := keypair.ParseFull(opts.DistributionPrivateKey)
		if err != nil {
			return SignatureService{}, fmt.Errorf("parsing distribution private key: %w", err)
		}

		distAccResolver, err = NewDistributionAccountResolver(DistributionAccountResolverConfig{
			HostDistributionAccountPublicKey: hostAccKP.Address(),
			AdminDBConnectionPool:            opts.DBConnectionPool,
		})
		if err != nil {
			return SignatureService{}, fmt.Errorf("creating a new distribution account resolver: %w", err)
		}
	}

	return SignatureService{
		ChAccountSigner:             chAccountSigner,
		DistAccountSigner:           distAccSigner,
		HostAccountSigner:           hostAccSigner,
		DistributionAccountResolver: distAccResolver,
		networkPassphrase:           opts.NetworkPassphrase,
	}, nil
}

func (ss *SignatureService) NetworkPassphrase() string {
	return ss.networkPassphrase
}
