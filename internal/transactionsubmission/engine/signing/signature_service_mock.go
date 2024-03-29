package signing

import (
	"github.com/stellar/go/network"
	"github.com/stretchr/testify/mock"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing/mocks"
)

type mockConstructorTestingTNewMockSignatureService interface {
	mock.TestingT
	Cleanup(func())
	Helper()
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
		ChAccountSigner:             chAccSigClient,
		DistAccountSigner:           distAccSigClient,
		HostAccountSigner:           hostAccSigClient,
		DistributionAccountResolver: distAccResolver,
	}

	return sigService, chAccSigClient, distAccSigClient, hostAccSigClient, distAccResolver
}
