package services

import (
	"context"
	"fmt"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/protocols/horizon"
	"github.com/stellar/go/strkey"
	"github.com/stellar/go/support/log"
	"github.com/stellar/go/txnbuild"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/utils"
)

type RecoveryMultisigSetupOptions struct {
	MasterPublicKey   string
	MasterPrivateKey  string
	CosignerPublicKey string
}

type RecoveryMultisigSetupService struct {
	NetworkPassphrase string
	HorizonClient     horizonclient.ClientInterface
}

func NewRecoveryMultisigSetupService(networkPassphrase string, horizonClient horizonclient.ClientInterface) (RecoveryMultisigSetupService, error) {
	if horizonClient == nil {
		return RecoveryMultisigSetupService{}, fmt.Errorf("horizon client cannot be nil")
	}
	if networkPassphrase == "" {
		return RecoveryMultisigSetupService{}, fmt.Errorf("network passphrase cannot be empty")
	}

	return RecoveryMultisigSetupService{
		NetworkPassphrase: networkPassphrase,
		HorizonClient:     horizonClient,
	}, nil
}

// SetupMultisigAdmin sets up the (provided) Master account to use a multisig with the (provided) Cosigner account, so
// that all transactions for the Master account require 2 signatures, one from the Master key and another from the
// Cosigner key.
func (s *RecoveryMultisigSetupService) SetupMultisigAdmin(ctx context.Context, opts RecoveryMultisigSetupOptions) error {
	// Parse keypairs
	recoveryMasterKP, err := keypair.ParseFull(opts.MasterPrivateKey)
	if err != nil {
		return fmt.Errorf("invalid admin secret key: %w", err)
	}

	if !strkey.IsValidEd25519PublicKey(opts.CosignerPublicKey) {
		return fmt.Errorf("invalid cosigner public key=%s", opts.CosignerPublicKey)
	}

	// Get recoveryAccount details
	recoveryAccount, err := s.HorizonClient.AccountDetail(horizonclient.AccountRequest{
		AccountID: opts.MasterPublicKey,
	})
	if err != nil {
		return fmt.Errorf("failed to get account details: %w", err)
	}

	if isAccountMultisigConfigured(ctx, recoveryAccount, opts.CosignerPublicKey) {
		log.Ctx(ctx).Infof("✅ Account %s already has a multisig configured", opts.MasterPublicKey)
		return nil
	}

	log.Ctx(ctx).Debugf("Setting up multisig for account=%s with cosigner=%s...", opts.MasterPublicKey, opts.CosignerPublicKey)

	tx, err := txnbuild.NewTransaction(txnbuild.TransactionParams{
		SourceAccount: &recoveryAccount,
		Operations: []txnbuild.Operation{
			&txnbuild.SetOptions{
				// Set thresholds to 2
				LowThreshold:    txnbuild.NewThreshold(2),
				MediumThreshold: txnbuild.NewThreshold(2),
				HighThreshold:   txnbuild.NewThreshold(2),
				// Add signer with weight=1
				Signer: &txnbuild.Signer{
					Address: opts.CosignerPublicKey,
					Weight:  1,
				},
				// Set master signer weight=1
				MasterWeight: txnbuild.NewThreshold(1),
			},
		},
		BaseFee: txnbuild.MinBaseFee,
		Preconditions: txnbuild.Preconditions{
			TimeBounds: txnbuild.NewTimeout(300),
		},
		IncrementSequenceNum: true,
	})
	if err != nil {
		return fmt.Errorf("failed to build transaction: %w", err)
	}

	// Sign transaction
	tx, err = tx.Sign(s.NetworkPassphrase, recoveryMasterKP)
	if err != nil {
		return fmt.Errorf("failed to sign transaction: %w", err)
	}

	txHash, err := tx.HashHex(s.NetworkPassphrase)
	if err != nil {
		return fmt.Errorf("failed to get transaction hash: %w", err)
	}

	// Submit transaction
	log.Ctx(ctx).Infof("Submitting transaction with hash=%s...", txHash)
	resp, err := s.HorizonClient.SubmitTransaction(tx)
	if err != nil {
		hErr := utils.NewHorizonErrorWrapper(err)
		log.Ctx(ctx).Errorf("horizon error=%+v", hErr)
		return fmt.Errorf("failed to submit transaction: %w", hErr)
	}

	if !resp.Successful {
		return fmt.Errorf("transaction failed with ResultXdr=%s", resp.ResultXdr)
	}

	log.Ctx(ctx).Infof("✅ Account %s now requires 2 signatures for all operations", opts.MasterPublicKey)

	return nil
}

// isAccountMultisigConfigured checks if the account has a multisig configured with the given cosigner public key.
// It asserts that the account has thresholds [2,2,2] and signers weights [cosigner=1,admin=1].
func isAccountMultisigConfigured(_ context.Context, account horizon.Account, cosignerPublicKey string) bool {
	// Assert thresholds: [2,2,2]
	wantThresholds := horizon.AccountThresholds{
		LowThreshold:  2,
		MedThreshold:  2,
		HighThreshold: 2,
	}
	if account.Thresholds != wantThresholds {
		return false
	}

	// Assert signers weights: (cosigner=1,admin=1)
	wantSignersMap := map[string]horizon.Signer{
		cosignerPublicKey: {
			Weight: 1,
			Key:    cosignerPublicKey,
			Type:   "ed25519_public_key",
		},
		account.AccountID: {
			Weight: 1,
			Key:    account.AccountID,
			Type:   "ed25519_public_key",
		},
	}

	if len(account.Signers) != len(wantSignersMap) {
		return false
	}
	for _, signer := range account.Signers {
		wantSigner, ok := wantSignersMap[signer.Key]
		if !ok || signer != wantSigner {
			return false
		}
	}

	return true
}
