package signing

import (
	"fmt"

	"github.com/stellar/go/network"
	"github.com/stretchr/testify/mock"
	"golang.org/x/exp/slices"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/preconditions"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/utils"
)

type SignatureService struct {
	ChAccSigner       SignatureClient
	DistAccountSigner SignatureClient
	HostSigner        SignatureClient
	DistributionAccountResolver
}

func (s *SignatureService) Validate() error {
	if s.ChAccSigner == nil {
		return fmt.Errorf("channel account signer cannot be nil")
	}

	if s.DistAccountSigner == nil {
		return fmt.Errorf("distribution account signer cannot be nil")
	}

	if s.HostSigner == nil {
		return fmt.Errorf("host account signer cannot be nil")
	}

	if s.ChAccSigner.NetworkPassphrase() != s.DistAccountSigner.NetworkPassphrase() || s.DistAccountSigner.NetworkPassphrase() != s.HostSigner.NetworkPassphrase() {
		return fmt.Errorf("network passphrase of all signers should be the same")
	}

	if s.DistributionAccountResolver == nil {
		return fmt.Errorf("distribution account resolver cannot be nil")
	}

	return nil
}

var _ DistributionAccountResolver = (*SignatureService)(nil)

type mockConstructorTestingTNewMockSignatureService interface {
	mock.TestingT
	Cleanup(func())
	Helper()
}

type SignatureServiceOptions struct {
	// Shared:
	NetworkPassphrase string

	// DistributionAccountEnv:
	DistributionPrivateKey string

	// ChannelAccountDB:
	DBConnectionPool     db.DBConnectionPool
	EncryptionPassphrase string
	LedgerNumberTracker  preconditions.LedgerNumberTracker
	Encrypter            utils.PrivateKeyEncrypter
}

func NewSignatureService(distSignerType SignatureClientType, opts SignatureServiceOptions) (SignatureService, error) {
	if !slices.Contains(SignatureClientType("").AllDistribution(), distSignerType) {
		return SignatureService{}, fmt.Errorf("invalid distribution signer type %q", distSignerType)
	}

	sigClientOpts := SignatureClientOptions{
		Type:                   distSignerType,
		NetworkPassphrase:      opts.NetworkPassphrase,
		DistributionPrivateKey: opts.DistributionPrivateKey,
		DBConnectionPool:       opts.DBConnectionPool,
		EncryptionPassphrase:   opts.EncryptionPassphrase,
		Encrypter:              opts.Encrypter,
		LedgerNumberTracker:    opts.LedgerNumberTracker,
	}

	sigClientOpts.Type = SignatureClientTypeChannelAccountDB
	chAccSigner, err := NewSignatureClient(sigClientOpts)
	if err != nil {
		return SignatureService{}, fmt.Errorf("creating a new channel account signature client: %w", err)
	}

	sigClientOpts.Type = distSignerType
	distAccSigner, err := NewSignatureClient(sigClientOpts)
	if err != nil {
		return SignatureService{}, fmt.Errorf("creating a new distribution account signature client: %w", err)
	}

	distAccResolver, ok := distAccSigner.(DistributionAccountResolver)
	if !ok {
		return SignatureService{}, fmt.Errorf("trying to cast a distribution account signer to a distribution account resolver")
	}

	return SignatureService{
		ChAccSigner:                 chAccSigner,
		DistAccountSigner:           distAccSigner,
		HostSigner:                  distAccSigner,
		DistributionAccountResolver: distAccResolver,
	}, nil
}

// NewMockSignatureService is a constructor for the SignatureService with mock clients.
func NewMockSignatureService(t mockConstructorTestingTNewMockSignatureService) (
	sigService SignatureService,
	chAccSigClient *mocks.MockSignatureClient,
	distAccSigClient *mocks.MockSignatureClient,
	hostAccSigClient *mocks.MockSignatureClient,
	distAccResolver *mocks.MockDistributionAccountResolver,
) {
	t.Helper()

	chAccSigClient = mocks.NewMockSignatureClient(t)
	chAccSigClient.On("NetworkPassphrase").Return(network.TestNetworkPassphrase).Maybe()

	distAccSigClient = mocks.NewMockSignatureClient(t)
	distAccSigClient.On("NetworkPassphrase").Return(network.TestNetworkPassphrase).Maybe()

	hostAccSigClient = mocks.NewMockSignatureClient(t)
	hostAccSigClient.On("NetworkPassphrase").Return(network.TestNetworkPassphrase).Maybe()

	distAccResolver = mocks.NewMockDistributionAccountResolver(t)
	sigService = SignatureService{
		ChAccSigner:                 chAccSigClient,
		DistAccountSigner:           distAccSigClient,
		HostSigner:                  hostAccSigClient,
		DistributionAccountResolver: distAccResolver,
	}

	return sigService, chAccSigClient, distAccSigClient, hostAccSigClient, distAccResolver
}
