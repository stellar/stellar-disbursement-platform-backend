package cmd

import (
	"context"
	"fmt"
	"go/types"

	"github.com/spf13/cobra"
	"github.com/stellar/go-stellar-sdk/support/config"
	"github.com/stellar/go-stellar-sdk/support/log"

	cmdUtils "github.com/stellar/stellar-disbursement-platform-backend/cmd/utils"
	tssservices "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/services"
)

// rotateWalletDEKExec verifies the wallet's key decrypts under the current KEK, then (unless
// dryRun) re-encrypts it under a fresh DEK and re-verifies. Sibling wallets' rows are never
// touched, so this is safe to run per wallet on a live system.
func rotateWalletDEKExec(ctx context.Context, svc tssservices.DistributionWalletKeyServiceInterface, publicKey string, dryRun bool) error {
	if _, err := svc.GetPrivateKey(ctx, publicKey); err != nil {
		return fmt.Errorf("verifying wallet key for %q before rotation: %w", publicKey, err)
	}

	if dryRun {
		log.Ctx(ctx).Infof("[dry-run] wallet key for %q decrypts under the current KEK; rotation would re-encrypt it under a new DEK (no changes made)", publicKey)
		return nil
	}

	if err := svc.RotateWalletDEK(ctx, publicKey); err != nil {
		return fmt.Errorf("rotating wallet DEK for %q: %w", publicKey, err)
	}

	if _, err := svc.GetPrivateKey(ctx, publicKey); err != nil {
		return fmt.Errorf("post-rotation verification failed for %q: %w", publicKey, err)
	}

	log.Ctx(ctx).Infof("wallet DEK rotated for %q: private key re-encrypted under a new DEK and verified", publicKey)
	return nil
}

// RotateWalletDEKCommand exposes per-wallet DEK rotation to operators:
// `distribution-account rotate-wallet-dek --wallet-public-key G... [--dry-run]`.
// Unlike `rotate` (which replaces a tenant's distribution ACCOUNT on-chain), this only
// re-encrypts one wallet's stored private key under a new data-encryption key.
func (c *DistributionAccountCommand) RotateWalletDEKCommand() *cobra.Command {
	var kek, walletPublicKey string
	var dryRun bool

	configOpts := config.ConfigOptions{
		{
			Name:           "distribution-account-encryption-passphrase",
			Usage:          "A Stellar-compliant ed25519 private key used to encrypt and decrypt the private keys of tenants' distribution accounts (the KEK wrapping each wallet's DEK).",
			OptType:        types.String,
			CustomSetValue: cmdUtils.SetConfigOptionStellarPrivateKey,
			ConfigKey:      &kek,
			Required:       true,
		},
		{
			Name:           "wallet-public-key",
			Usage:          "Public key of the distribution wallet whose DEK should be rotated.",
			OptType:        types.String,
			CustomSetValue: cmdUtils.SetConfigOptionStellarPublicKey,
			ConfigKey:      &walletPublicKey,
			Required:       true,
		},
		{
			Name:        "dry-run",
			Usage:       "Verify the wallet key decrypts under the current KEK without writing any changes.",
			OptType:     types.Bool,
			ConfigKey:   &dryRun,
			FlagDefault: false,
			Required:    false,
		},
	}

	rotateWalletDEKCmd := &cobra.Command{
		Use:   "rotate-wallet-dek",
		Short: "Re-encrypt one distribution wallet's private key under a new DEK",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			cmd.Parent().PersistentPreRun(cmd.Parent(), args)
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			configOpts.Require()
			if err := configOpts.SetValues(); err != nil {
				log.Ctx(ctx).Fatalf("Error setting values of config options in %s: %v", cmd.Name(), err)
			}
		},
		Run: func(cmd *cobra.Command, _ []string) {
			ctx := cmd.Context()

			walletKeyService, err := tssservices.NewDistributionWalletKeyService(c.TSSDBConnectionPool, kek)
			if err != nil {
				log.Ctx(ctx).Fatalf("Error creating distribution wallet key service: %v", err)
			}

			if err := rotateWalletDEKExec(ctx, walletKeyService, walletPublicKey, dryRun); err != nil {
				c.CrashTrackerClient.LogAndReportErrors(ctx, err, "Cmd distribution-account rotate-wallet-dek crash")
				log.Ctx(ctx).Fatalf("Error rotating wallet DEK: %v", err)
			}
		},
	}
	if err := configOpts.Init(rotateWalletDEKCmd); err != nil {
		log.Ctx(rotateWalletDEKCmd.Context()).Fatalf("Error initializing %s command: %v", rotateWalletDEKCmd.Name(), err)
	}

	return rotateWalletDEKCmd
}
