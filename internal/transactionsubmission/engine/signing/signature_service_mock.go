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
	signerRouter *mocks.MockSignerRouter,
	distAccResolver *mocks.MockDistributionAccountResolver,
) {
	t.Helper()

	signerRouter = mocks.NewMockSignerRouter(t)
	signerRouter.On("NetworkPassphrase").Return(network.TestNetworkPassphrase).Maybe()

	distAccResolver = mocks.NewMockDistributionAccountResolver(t)
	sigService = SignatureService{
		SignerRouter:                signerRouter,
		DistributionAccountResolver: distAccResolver,
	}

	return sigService, signerRouter, distAccResolver
}
