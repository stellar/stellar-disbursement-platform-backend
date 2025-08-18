package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/stellar/go/support/config"
	"github.com/stellar/go/support/log"

	cmdUtils "github.com/stellar/stellar-disbursement-platform-backend/cmd/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/dependencyinjection"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/services"
)

type MultisigSetupCmdSvcInterface interface {
	SetupRecoveryMultisig(ctx context.Context, svc services.RecoveryMultisigSetupService, opts services.RecoveryMultisigSetupOptions) error
}

type MultisigSetupCmdSvc struct{}

func (s *MultisigSetupCmdSvc) SetupRecoveryMultisig(ctx context.Context, svc services.RecoveryMultisigSetupService, opts services.RecoveryMultisigSetupOptions) error {
	return svc.SetupMultisigAdmin(ctx, opts)
}

// MultisigSetupCommand is the command that sets up the multisig for the recovery account used for embedded wallets.
type MultisigSetupCommand struct {
	multisigSvc services.RecoveryMultisigSetupService
}

func (c *MultisigSetupCommand) Command(svc MultisigSetupCmdSvcInterface) *cobra.Command {
	var horizonURL string
	var opts services.RecoveryMultisigSetupOptions

	configOpts := config.ConfigOptions{
		cmdUtils.HorizonURL(&horizonURL),
		cmdUtils.EmbeddedWalletsRecoveryAddress(&opts.MasterPublicKey),
		cmdUtils.EmbeddedWalletsRecoveryMasterPrivateKey(&opts.MasterPrivateKey),
		cmdUtils.EmbeddedWalletsRecoveryCosignerPublicKey(&opts.CosignerPublicKey),
	}

	setupMultisigAdminCmd := &cobra.Command{
		Use:   "multisig-setup",
		Short: "Sets up the multisig for the recovery account used for embedded wallets",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			cmd.Parent().PersistentPreRun(cmd.Parent(), args)
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			// Validate & ingest input parameters
			configOpts.Require()
			err := configOpts.SetValues()
			if err != nil {
				return fmt.Errorf("setting values of config options: %w", err)
			}

			// Setup Horizon client
			hClient, err := dependencyinjection.NewHorizonClient(ctx, horizonURL)
			if err != nil {
				return fmt.Errorf("setting up horizon client: %w", err)
			}

			c.multisigSvc, err = services.NewRecoveryMultisigSetupService(globalOptions.NetworkPassphrase, hClient)
			if err != nil {
				return fmt.Errorf("setting up recovery multisig service: %w", err)
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			return svc.SetupRecoveryMultisig(ctx, c.multisigSvc, opts)
		},
	}

	err := configOpts.Init(setupMultisigAdminCmd)
	if err != nil {
		log.Ctx(setupMultisigAdminCmd.Context()).Fatalf("Error initializing multisig-setup command: %v", err)
	}

	return setupMultisigAdminCmd
}
