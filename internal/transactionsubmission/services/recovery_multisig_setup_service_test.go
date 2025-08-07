package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/network"
	"github.com/stellar/go/protocols/horizon"
	"github.com/stellar/go/support/render/problem"
	"github.com/stellar/go/txnbuild"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_NewRecoveryMultisigSetupService(t *testing.T) {
	tests := []struct {
		name              string
		networkPassphrase string
		horizonClient     horizonclient.ClientInterface
		wantErrContains   string
		wantResult        RecoveryMultisigSetupService
	}{
		{
			name:              "🔴horizon_cannot_be_nil",
			networkPassphrase: network.TestNetworkPassphrase,
			horizonClient:     nil,
			wantErrContains:   "horizon client cannot be nil",
		},
		{
			name:              "🔴network_passphrase_cannot_be_empty",
			networkPassphrase: "",
			horizonClient:     &horizonclient.MockClient{},
			wantErrContains:   "network passphrase cannot be empty",
		},
		{
			name:              "🟢success",
			networkPassphrase: network.TestNetworkPassphrase,
			horizonClient:     &horizonclient.MockClient{},
			wantErrContains:   "",
			wantResult: RecoveryMultisigSetupService{
				NetworkPassphrase: network.TestNetworkPassphrase,
				HorizonClient:     &horizonclient.MockClient{},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NewRecoveryMultisigSetupService(tc.networkPassphrase, tc.horizonClient)
			if tc.wantErrContains != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErrContains)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.wantResult, got)
			}
		})
	}
}

func Test_isAccountMultisigConfigured(t *testing.T) {
	ctx := context.Background()

	// Generate test keypairs
	adminKP, err := keypair.Random()
	require.NoError(t, err)

	cosignerKP, err := keypair.Random()
	require.NoError(t, err)

	tests := []struct {
		name              string
		account           horizon.Account
		cosignerPublicKey string
		expectedResult    bool
	}{
		{
			name: "🟢already_configured_multisig",
			account: horizon.Account{
				AccountID: adminKP.Address(),
				Thresholds: horizon.AccountThresholds{
					LowThreshold:  2,
					MedThreshold:  2,
					HighThreshold: 2,
				},
				Signers: []horizon.Signer{
					{
						Weight: 1,
						Key:    cosignerKP.Address(),
						Type:   "ed25519_public_key",
					},
					{
						Weight: 1,
						Key:    adminKP.Address(),
						Type:   "ed25519_public_key",
					},
				},
			},
			cosignerPublicKey: cosignerKP.Address(),
			expectedResult:    true,
		},
		{
			name: "🔴wrong_threshold",
			account: horizon.Account{
				AccountID: adminKP.Address(),
				Thresholds: horizon.AccountThresholds{
					LowThreshold:  1, // Wrong
					MedThreshold:  2,
					HighThreshold: 2,
				},
				Signers: []horizon.Signer{
					{
						Weight: 1,
						Key:    cosignerKP.Address(),
						Type:   "ed25519_public_key",
					},
					{
						Weight: 1,
						Key:    adminKP.Address(),
						Type:   "ed25519_public_key",
					},
				},
			},
			cosignerPublicKey: cosignerKP.Address(),
			expectedResult:    false,
		},
		{
			name: "🔴wrong_signer_weight",
			account: horizon.Account{
				AccountID: adminKP.Address(),
				Thresholds: horizon.AccountThresholds{
					LowThreshold:  2,
					MedThreshold:  2,
					HighThreshold: 2,
				},
				Signers: []horizon.Signer{
					{
						Weight: 2, // Wrong - should be 1
						Key:    cosignerKP.Address(),
						Type:   "ed25519_public_key",
					},
					{
						Weight: 1,
						Key:    adminKP.Address(),
						Type:   "ed25519_public_key",
					},
				},
			},
			cosignerPublicKey: cosignerKP.Address(),
			expectedResult:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := isAccountMultisigConfigured(ctx, tc.account, tc.cosignerPublicKey)
			assert.Equal(t, tc.expectedResult, result)
		})
	}
}

func Test_RecoveryMultisigSetupService_SetupMultisigAdmin(t *testing.T) {
	ctx := context.Background()
	networkPassphrase := network.TestNetworkPassphrase

	cosignerKP := keypair.MustRandom()
	account := keypair.MustRandom()

	hAccount := horizon.Account{
		AccountID: account.Address(),
		Thresholds: horizon.AccountThresholds{
			LowThreshold:  1,
			MedThreshold:  1,
			HighThreshold: 1,
		},
		Signers: []horizon.Signer{
			{
				Weight: 1,
				Key:    account.Address(),
				Type:   "ed25519_public_key",
			},
		},
	}

	hAlreadyConfiguredAccount := horizon.Account{
		AccountID: account.Address(),
		Thresholds: horizon.AccountThresholds{
			LowThreshold:  2,
			MedThreshold:  2,
			HighThreshold: 2,
		},
		Signers: []horizon.Signer{
			{
				Weight: 1,
				Key:    account.Address(),
				Type:   "ed25519_public_key",
			},
			{
				Weight: 1,
				Key:    cosignerKP.Address(),
				Type:   "ed25519_public_key",
			},
		},
	}

	tests := []struct {
		name          string
		opts          RecoveryMultisigSetupOptions
		prepareMocks  func(t *testing.T, mHorizonClient *horizonclient.MockClient)
		expectedError string
	}{
		{
			name: "🔴invalid_admin_secret_key",
			opts: RecoveryMultisigSetupOptions{
				MasterPublicKey:   account.Address(),
				MasterPrivateKey:  "invalid_secret_key",
				CosignerPublicKey: cosignerKP.Address(),
			},
			expectedError: "invalid admin secret key:",
		},
		{
			name: "🔴invalid_cosigner_public_key",
			opts: RecoveryMultisigSetupOptions{
				MasterPublicKey:   account.Address(),
				MasterPrivateKey:  account.Seed(),
				CosignerPublicKey: "invalid_public_key",
			},
			expectedError: "invalid cosigner public key=invalid_public_key",
		},
		{
			name: "🔴account_detail_failure",
			opts: RecoveryMultisigSetupOptions{
				MasterPublicKey:   account.Address(),
				MasterPrivateKey:  account.Seed(),
				CosignerPublicKey: cosignerKP.Address(),
			},
			prepareMocks: func(t *testing.T, mHorizonClient *horizonclient.MockClient) {
				mHorizonClient.
					On("AccountDetail", horizonclient.AccountRequest{AccountID: account.Address()}).
					Return(horizon.Account{}, errors.New("horizon error")).
					Once()
			},
			expectedError: "failed to get account details: horizon error",
		},
		{
			name: "🟢already_configured_multisig",
			opts: RecoveryMultisigSetupOptions{
				MasterPublicKey:   account.Address(),
				MasterPrivateKey:  account.Seed(),
				CosignerPublicKey: cosignerKP.Address(),
			},
			prepareMocks: func(t *testing.T, mHorizonClient *horizonclient.MockClient) {
				mHorizonClient.
					On("AccountDetail", horizonclient.AccountRequest{AccountID: account.Address()}).
					Return(hAlreadyConfiguredAccount, nil).
					Once()
			},
		},
		{
			name: "🔴transaction_submission_failure",
			opts: RecoveryMultisigSetupOptions{
				MasterPublicKey:   account.Address(),
				MasterPrivateKey:  account.Seed(),
				CosignerPublicKey: cosignerKP.Address(),
			},
			prepareMocks: func(t *testing.T, mHorizonClient *horizonclient.MockClient) {
				mHorizonClient.
					On("AccountDetail", horizonclient.AccountRequest{AccountID: account.Address()}).
					Return(hAccount, nil).
					Once().
					On("SubmitTransaction", mock.AnythingOfType("*txnbuild.Transaction")).
					Return(horizon.Transaction{}, errors.New("submission error")).
					Once()
			},
			expectedError: "failed to submit transaction: horizon response error: submission error",
		},
		{
			name: "🔴horizon_error_response",
			opts: RecoveryMultisigSetupOptions{
				MasterPublicKey:   account.Address(),
				MasterPrivateKey:  account.Seed(),
				CosignerPublicKey: cosignerKP.Address(),
			},
			prepareMocks: func(t *testing.T, mHorizonClient *horizonclient.MockClient) {
				mHorizonClient.
					On("AccountDetail", horizonclient.AccountRequest{AccountID: account.Address()}).
					Return(hAccount, nil).
					Once().
					On("SubmitTransaction", mock.AnythingOfType("*txnbuild.Transaction")).
					Return(horizon.Transaction{}, horizonclient.Error{
						Problem: problem.P{
							Type:   "https://stellar.org/horizon-errors/transaction_failed",
							Title:  "Transaction Failed",
							Status: 400,
							Detail: "Transaction failed",
						},
					}).
					Once()
			},
			expectedError: "failed to submit transaction:",
		},
		{
			name: "🔴transaction_unsuccessful",
			opts: RecoveryMultisigSetupOptions{
				MasterPublicKey:   account.Address(),
				MasterPrivateKey:  account.Seed(),
				CosignerPublicKey: cosignerKP.Address(),
			},
			prepareMocks: func(t *testing.T, mHorizonClient *horizonclient.MockClient) {
				mHorizonClient.
					On("AccountDetail", horizonclient.AccountRequest{AccountID: account.Address()}).
					Return(hAccount, nil).
					Once().
					On("SubmitTransaction", mock.AnythingOfType("*txnbuild.Transaction")).
					Return(horizon.Transaction{Successful: false, ResultXdr: "tx_failed"}, nil).
					Once()
			},
			expectedError: "transaction failed with ResultXdr=tx_failed",
		},
		{
			name: "🟢successfully_setup_multisig",
			opts: RecoveryMultisigSetupOptions{
				MasterPublicKey:   account.Address(),
				MasterPrivateKey:  account.Seed(),
				CosignerPublicKey: cosignerKP.Address(),
			},
			prepareMocks: func(t *testing.T, mHorizonClient *horizonclient.MockClient) {
				mHorizonClient.
					On("AccountDetail", horizonclient.AccountRequest{AccountID: account.Address()}).
					Return(hAccount, nil).
					Once().
					On("SubmitTransaction", mock.AnythingOfType("*txnbuild.Transaction")).
					Run(func(args mock.Arguments) {
						submittedTx := args.Get(0).(*txnbuild.Transaction)
						assert.InDelta(t, time.Now().Add(300*time.Second).Unix(), submittedTx.Timebounds().MaxTime, 5)

						wantTx, err := txnbuild.NewTransaction(txnbuild.TransactionParams{
							SourceAccount: &hAccount,
							Operations: []txnbuild.Operation{
								&txnbuild.SetOptions{
									// Set thresholds to 2
									LowThreshold:    txnbuild.NewThreshold(2),
									MediumThreshold: txnbuild.NewThreshold(2),
									HighThreshold:   txnbuild.NewThreshold(2),
									// Add signer with weight=1
									Signer: &txnbuild.Signer{
										Address: cosignerKP.Address(),
										Weight:  1,
									},
									// Set master signer weight=1
									MasterWeight: txnbuild.NewThreshold(1),
								},
							},
							BaseFee: txnbuild.MinBaseFee,
							Preconditions: txnbuild.Preconditions{
								TimeBounds: submittedTx.Timebounds(),
							},
							IncrementSequenceNum: true,
						})
						require.NoError(t, err)
						wantTx, err = wantTx.Sign(networkPassphrase, account)
						require.NoError(t, err)

						assert.Equal(t, wantTx, submittedTx)
					}).
					Return(horizon.Transaction{Successful: true}, nil).
					Once()
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mHorizonClient := &horizonclient.MockClient{}

			s := &RecoveryMultisigSetupService{
				NetworkPassphrase: networkPassphrase,
				HorizonClient:     mHorizonClient,
			}

			if tc.prepareMocks != nil {
				tc.prepareMocks(t, mHorizonClient)
			}

			err := s.SetupMultisigAdmin(ctx, tc.opts)

			if tc.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedError)
			} else {
				require.NoError(t, err)
			}

			mHorizonClient.AssertExpectations(t)
		})
	}
}
