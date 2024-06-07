package signing

import (
	"context"
	"fmt"
	"testing"

	"github.com/stellar/go/keypair"
	"github.com/stellar/go/network"
	"github.com/stellar/go/txnbuild"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_AccountEnvOptions_String_doesntLeakPrivateKey(t *testing.T) {
	opts := AccountEnvOptions{
		DistributionPrivateKey: "SOME_PRIVATE_KEY",
		NetworkPassphrase:      "SOME_PASSPHRASE",
	}

	testCases := []struct {
		name  string
		value string
	}{
		{name: "opts.String()", value: opts.String()},
		{name: "&opts.String()", value: (&opts).String()},
		{name: "%%v value", value: fmt.Sprintf("%v", opts)},
		{name: "%%v pointer", value: fmt.Sprintf("%v", &opts)},
		{name: "%%+v value", value: fmt.Sprintf("%+v", opts)},
		{name: "%%+v pointer", value: fmt.Sprintf("%+v", &opts)},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.NotContains(t, tc.value, "SOME_PRIVATE_KEY")
			assert.Contains(t, tc.value, "SOME_PASSPHRASE")
			assert.Contains(t, tc.value, fmt.Sprintf("%T", opts))
		})
	}
}

func Test_AccountEnvOptions_Validate(t *testing.T) {
	testCases := []struct {
		name              string
		opts              AccountEnvOptions
		wantErrorContains string
	}{
		{
			name:              "returns an error if the network passphrase is empty",
			opts:              AccountEnvOptions{},
			wantErrorContains: "network passphrase cannot be empty",
		},
		{
			name: "returns an error if the distribution private key is empty",
			opts: AccountEnvOptions{
				NetworkPassphrase: network.TestNetworkPassphrase,
			},
			wantErrorContains: "distribution private key is not a valid Ed25519 secret",
		},
		{
			name: "returns an error if the distribution private key is invalid",
			opts: AccountEnvOptions{
				NetworkPassphrase:      network.TestNetworkPassphrase,
				DistributionPrivateKey: "invalid",
			},
			wantErrorContains: "distribution private key is not a valid Ed25519 secret",
		},
		{
			name: "🎉 successfully validate options",
			opts: AccountEnvOptions{
				NetworkPassphrase:      network.TestNetworkPassphrase,
				DistributionPrivateKey: keypair.MustRandom().Seed(),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.opts.Validate()

			if tc.wantErrorContains == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.wantErrorContains)
			}
		})
	}
}

func Test_NewAccountEnvSignatureClient(t *testing.T) {
	distributionKP := keypair.MustRandom()

	testCases := []struct {
		name              string
		opts              AccountEnvOptions
		wantErrorContains string
		wantClient        *AccountEnvSignatureClient
	}{
		{
			name:              "returns an error if the options are invalid",
			opts:              AccountEnvOptions{},
			wantErrorContains: "validating options: network passphrase cannot be empty",
		},
		{
			name: "🎉 successfully create a new AccountEnvSignatureClient",
			opts: AccountEnvOptions{
				NetworkPassphrase:      network.TestNetworkPassphrase,
				DistributionPrivateKey: distributionKP.Seed(),
			},
			wantClient: &AccountEnvSignatureClient{
				networkPassphrase:   network.TestNetworkPassphrase,
				distributionAccount: distributionKP.Address(),
				distributionKP:      distributionKP,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotClient, err := NewAccountEnvSignatureClient(tc.opts)

			if tc.wantErrorContains == "" {
				require.NoError(t, err)
				require.Equal(t, tc.wantClient, gotClient)
			} else {
				require.ErrorContains(t, err, tc.wantErrorContains)
				require.Empty(t, gotClient)
			}
		})
	}
}

func Test_AccountEnvSignatureClient_validateStellarAccounts(t *testing.T) {
	distributionKP := keypair.MustRandom()
	unsupportedAccountKP := keypair.MustRandom()
	distEnvClient, err := NewAccountEnvSignatureClient(AccountEnvOptions{
		NetworkPassphrase:      network.TestNetworkPassphrase,
		DistributionPrivateKey: distributionKP.Seed(),
	})
	require.NoError(t, err)

	testCases := []struct {
		name              string
		stellarAccounts   []string
		wantErrorContains string
	}{
		{
			name:              "returns an error if the stellar accounts are empty",
			stellarAccounts:   []string{},
			wantErrorContains: "stellar accounts cannot be empty in " + distEnvClient.name(),
		},
		{
			name:              "returns an error if an account other than the distribution one is provided",
			stellarAccounts:   []string{unsupportedAccountKP.Address(), distributionKP.Address()},
			wantErrorContains: fmt.Sprintf("stellar account %s is not allowed to sign in %s", unsupportedAccountKP.Address(), distEnvClient.name()),
		},
		{
			name:            "🎉 successfully signs with distribution account",
			stellarAccounts: []string{distributionKP.Address()},
		},
		{
			name:            "🎉 successfully signs with distribution account, even if repeated",
			stellarAccounts: []string{distributionKP.Address(), distributionKP.Address()},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := distEnvClient.validateStellarAccounts(tc.stellarAccounts...)
			if tc.wantErrorContains == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.wantErrorContains)
			}
		})
	}
}

func Test_AccountEnvSignatureClient_SignStellarTransaction(t *testing.T) {
	ctx := context.Background()

	distributionKP := keypair.MustRandom()
	unsupportedAccountKP := keypair.MustRandom()
	distEnvClient, err := NewAccountEnvSignatureClient(AccountEnvOptions{
		NetworkPassphrase:      network.TestNetworkPassphrase,
		DistributionPrivateKey: distributionKP.Seed(),
	})
	require.NoError(t, err)

	// create stellar transaction
	chSourceAccount := txnbuild.NewSimpleAccount(distributionKP.Address(), int64(9605939170639897))
	stellarTx, err := txnbuild.NewTransaction(
		txnbuild.TransactionParams{
			SourceAccount:        &chSourceAccount,
			IncrementSequenceNum: true,
			Operations: []txnbuild.Operation{&txnbuild.Payment{
				Destination: "GCCOBXW2XQNUSL467IEILE6MMCNRR66SSVL4YQADUNYYNUVREF3FIV2Z",
				Amount:      "10",
				Asset:       txnbuild.NativeAsset{},
			}},
			BaseFee:       txnbuild.MinBaseFee,
			Preconditions: txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(60)},
		},
	)
	require.NoError(t, err)

	wantSignedStellarTx, err := stellarTx.Sign(network.TestNetworkPassphrase, distributionKP)
	require.NoError(t, err)

	testCases := []struct {
		name                string
		stellarTx           *txnbuild.Transaction
		accounts            []string
		wantErrContains     string
		wantSignedStellarTx *txnbuild.Transaction
	}{
		{
			name:            "return an error if stellar transaction is nil",
			stellarTx:       nil,
			accounts:        []string{},
			wantErrContains: "stellarTx cannot be nil",
		},
		{
			name:            "return stellar account validation fails",
			stellarTx:       stellarTx,
			accounts:        []string{unsupportedAccountKP.Address()},
			wantErrContains: fmt.Sprintf("validating stellar accounts: stellar account %s is not allowed to sign in %s", unsupportedAccountKP.Address(), distEnvClient.name()),
		},
		{
			name:                "🎉 Successfully sign transaction when all incoming addresse is correct",
			stellarTx:           stellarTx,
			accounts:            []string{distributionKP.Address()},
			wantSignedStellarTx: wantSignedStellarTx,
		},
		{
			name:                "🎉 Successfully sign transaction when all incoming addresse is correct, even if repeated",
			stellarTx:           stellarTx,
			accounts:            []string{distributionKP.Address(), distributionKP.Address()},
			wantSignedStellarTx: wantSignedStellarTx,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotSignedStellarTx, err := distEnvClient.SignStellarTransaction(ctx, tc.stellarTx, tc.accounts...)
			if tc.wantErrContains != "" {
				assert.ErrorContains(t, err, tc.wantErrContains)
				assert.Nil(t, gotSignedStellarTx)
			} else {
				require.NoError(t, err)
				assert.ElementsMatch(t, tc.wantSignedStellarTx.Signatures(), gotSignedStellarTx.Signatures())
			}
		})
	}
}

func Test_AccountEnvSignatureClient_SignFeeBumpStellarTransaction(t *testing.T) {
	ctx := context.Background()

	distributionKP := keypair.MustRandom()
	unsupportedAccountKP := keypair.MustRandom()
	distEnvClient, err := NewAccountEnvSignatureClient(AccountEnvOptions{
		NetworkPassphrase:      network.TestNetworkPassphrase,
		DistributionPrivateKey: distributionKP.Seed(),
	})
	require.NoError(t, err)

	// create stellar transaction
	chSourceAccount := txnbuild.NewSimpleAccount(distributionKP.Address(), int64(9605939170639897))
	stellarTx, err := txnbuild.NewTransaction(
		txnbuild.TransactionParams{
			SourceAccount:        &chSourceAccount,
			IncrementSequenceNum: true,
			Operations: []txnbuild.Operation{&txnbuild.Payment{
				Destination: "GCCOBXW2XQNUSL467IEILE6MMCNRR66SSVL4YQADUNYYNUVREF3FIV2Z",
				Amount:      "10",
				Asset:       txnbuild.NativeAsset{},
			}},
			BaseFee:       txnbuild.MinBaseFee,
			Preconditions: txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(60)},
		},
	)
	require.NoError(t, err)

	signedStellarTx, err := stellarTx.Sign(network.TestNetworkPassphrase, distributionKP)
	require.NoError(t, err)

	feeBumpStellarTx, err := txnbuild.NewFeeBumpTransaction(
		txnbuild.FeeBumpTransactionParams{
			Inner:      signedStellarTx,
			FeeAccount: distributionKP.Address(),
			BaseFee:    txnbuild.MinBaseFee,
		},
	)
	require.NoError(t, err)

	wantSignedFeeBumpStellarTx, err := feeBumpStellarTx.Sign(network.TestNetworkPassphrase, distributionKP)
	assert.NoError(t, err)

	testCases := []struct {
		name                       string
		feeBumpStellarTx           *txnbuild.FeeBumpTransaction
		accounts                   []string
		wantErrContains            string
		wantSignedFeeBumpStellarTx *txnbuild.FeeBumpTransaction
	}{
		{
			name:             "return an error if stellar transaction is nil",
			feeBumpStellarTx: nil,
			accounts:         []string{},
			wantErrContains:  "stellarTx cannot be nil",
		},
		{
			name:             "return stellar account validation fails",
			feeBumpStellarTx: feeBumpStellarTx,
			accounts:         []string{unsupportedAccountKP.Address()},
			wantErrContains:  fmt.Sprintf("validating stellar accounts: stellar account %s is not allowed to sign in %s", unsupportedAccountKP.Address(), distEnvClient.name()),
		},
		{
			name:                       "🎉 Successfully sign transaction when all incoming addresse is correct",
			feeBumpStellarTx:           feeBumpStellarTx,
			accounts:                   []string{distributionKP.Address()},
			wantSignedFeeBumpStellarTx: wantSignedFeeBumpStellarTx,
		},
		{
			name:                       "🎉 Successfully sign transaction when all incoming addresse is correct, even if repeated",
			feeBumpStellarTx:           feeBumpStellarTx,
			accounts:                   []string{distributionKP.Address(), distributionKP.Address()},
			wantSignedFeeBumpStellarTx: wantSignedFeeBumpStellarTx,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotSignedStellarTx, err := distEnvClient.SignFeeBumpStellarTransaction(ctx, tc.feeBumpStellarTx, tc.accounts...)
			if tc.wantErrContains != "" {
				assert.ErrorContains(t, err, tc.wantErrContains)
				assert.Nil(t, gotSignedStellarTx)
			} else {
				require.NoError(t, err)
				assert.ElementsMatch(t, tc.wantSignedFeeBumpStellarTx.Signatures(), gotSignedStellarTx.Signatures())
			}
		})
	}
}

func Test_AccountEnvSignatureClient_BatchInsert(t *testing.T) {
	ctx := context.Background()
	distributionKP := keypair.MustRandom()
	distEnvClient, err := NewAccountEnvSignatureClient(AccountEnvOptions{
		NetworkPassphrase:      network.TestNetworkPassphrase,
		DistributionPrivateKey: distributionKP.Seed(),
	})
	require.NoError(t, err)

	t.Run("number needs to be greated than zero", func(t *testing.T) {
		insertedAccounts, err := distEnvClient.BatchInsert(ctx, 0)
		require.NotErrorIs(t, err, ErrUnsupportedCommand)
		require.ErrorContains(t, err, "number must be greater than 0")
		require.Nil(t, insertedAccounts)
	})

	t.Run("one account returns the list with the error ErrUnsupportedCommand", func(t *testing.T) {
		insertedAccounts, err := distEnvClient.BatchInsert(ctx, 1)
		require.ErrorIs(t, err, ErrUnsupportedCommand)
		require.ElementsMatch(t, []string{distributionKP.Address()}, insertedAccounts)
	})

	t.Run("multiple account returns the list with the error ErrUnsupportedCommand", func(t *testing.T) {
		insertedAccounts, err := distEnvClient.BatchInsert(ctx, 3)
		require.ErrorIs(t, err, ErrUnsupportedCommand)
		require.ElementsMatch(t, []string{distributionKP.Address(), distributionKP.Address(), distributionKP.Address()}, insertedAccounts)
	})
}

func Test_AccountEnvSignatureClient_Delete(t *testing.T) {
	ctx := context.Background()
	distributionKP := keypair.MustRandom()
	unsupportedAccountKP := keypair.MustRandom()
	distEnvClient, err := NewAccountEnvSignatureClient(AccountEnvOptions{
		NetworkPassphrase:      network.TestNetworkPassphrase,
		DistributionPrivateKey: distributionKP.Seed(),
	})
	require.NoError(t, err)

	t.Run("return an error if attempted to delete an unsupported account", func(t *testing.T) {
		err = distEnvClient.Delete(ctx, unsupportedAccountKP.Address())
		require.ErrorContains(t, err, "validating stellar account to delete")
		require.NotErrorIs(t, err, ErrUnsupportedCommand)
	})

	t.Run("return the error ErrUnsupportedCommand if attempted to delete the distribution account", func(t *testing.T) {
		err = distEnvClient.Delete(ctx, distributionKP.Address())
		require.ErrorIs(t, err, ErrUnsupportedCommand)
	})
}
