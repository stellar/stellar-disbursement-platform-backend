package cmd

import (
	"context"
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/network"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/services"
)

type mockMultisigSetup struct {
	mock.Mock
}

// Making sure that mockMultisigSetup implements MultisigSetupCmdSvcInterface
var _ MultisigSetupCmdSvcInterface = (*mockMultisigSetup)(nil)

func (m *mockMultisigSetup) SetupRecoveryMultisig(ctx context.Context, svc services.RecoveryMultisigSetupService, opts services.RecoveryMultisigSetupOptions) error {
	return m.Called(ctx, svc, opts).Error(0)
}

func Test_MultisigSetupCommand_Command(t *testing.T) {
	globalOptions.NetworkPassphrase = network.TestNetworkPassphrase

	masterKP := keypair.MustRandom()
	cosignerKP := keypair.MustRandom()

	expectedOpts := services.RecoveryMultisigSetupOptions{
		MasterPublicKey:   masterKP.Address(),
		MasterPrivateKey:  masterKP.Seed(),
		CosignerPublicKey: cosignerKP.Address(),
	}

	t.Run("returns error when MultisigSetupService fails", func(t *testing.T) {
		serviceMock := &mockMultisigSetup{}
		serviceMock.
			On("SetupRecoveryMultisig", context.Background(), mock.AnythingOfType("services.RecoveryMultisigSetupService"), expectedOpts).
			Return(errors.New("fake error")).
			Once()

		cmd := (&MultisigSetupCommand{}).Command(serviceMock)
		rootCmmd := &cobra.Command{
			PersistentPreRun: func(cmd *cobra.Command, args []string) {},
		}
		rootCmmd.AddCommand(cmd)

		t.Setenv("HORIZON_URL", "https://horizon-testnet.stellar.org")
		t.Setenv("NETWORK_PASSPHRASE", network.TestNetworkPassphrase)
		rootCmmd.SetArgs([]string{
			"multisig-setup",
			"--embedded-wallets-recovery-address", masterKP.Address(),
			"--embedded-wallets-recovery-master-private-key", masterKP.Seed(),
			"--embedded-wallets-recovery-cosigner-public-key", cosignerKP.Address(),
		})

		err := rootCmmd.Execute()
		assert.ErrorContains(t, err, "fake error")

		serviceMock.AssertExpectations(t)
	})

	t.Run("executes the multisig setup command successfully", func(t *testing.T) {
		serviceMock := &mockMultisigSetup{}
		serviceMock.
			On("SetupRecoveryMultisig", context.Background(), mock.AnythingOfType("services.RecoveryMultisigSetupService"), expectedOpts).
			Return(nil)

		rootCmmd := &cobra.Command{
			PersistentPreRun: func(cmd *cobra.Command, args []string) {},
		}

		cmd := (&MultisigSetupCommand{}).Command(serviceMock)
		rootCmmd.AddCommand(cmd)

		t.Setenv("HORIZON_URL", "https://horizon-testnet.stellar.org")
		t.Setenv("NETWORK_PASSPHRASE", network.TestNetworkPassphrase)
		rootCmmd.SetArgs([]string{
			"multisig-setup",
			"--embedded-wallets-recovery-address", masterKP.Address(),
			"--embedded-wallets-recovery-master-private-key", masterKP.Seed(),
			"--embedded-wallets-recovery-cosigner-public-key", cosignerKP.Address(),
		})

		err := rootCmmd.Execute()
		require.NoError(t, err)

		serviceMock.AssertExpectations(t)
	})
}
