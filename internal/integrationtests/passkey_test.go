package integrationtests

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"math/big"
	"slices"
	"testing"

	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/network"
	"github.com/stellar/go-stellar-sdk/strkey"
	"github.com/stellar/go-stellar-sdk/xdr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

func Test_passkey_signSEP45AuthEntries(t *testing.T) {
	pk, err := newPasskey()
	require.NoError(t, err)

	contractID := randomContractID(t)
	contractAddress := strkey.MustEncode(strkey.VersionByteContract, contractID[:])
	serverAccountID := xdr.MustAddress(keypair.MustRandom().Address())

	contractEntry := newTestSEP45AuthEntry(xdr.ScAddress{Type: xdr.ScAddressTypeScAddressTypeContract, ContractId: &contractID})
	serverEntry := newTestSEP45AuthEntry(xdr.ScAddress{Type: xdr.ScAddressTypeScAddressTypeAccount, AccountId: &serverAccountID})
	challenge, err := xdr.MarshalBase64(xdr.SorobanAuthorizationEntries{serverEntry, contractEntry})
	require.NoError(t, err)

	t.Run("signs only the contract's entry, with an assertion the public key verifies", func(t *testing.T) {
		const expirationLedger = uint32(1234)
		signedChallenge, err := pk.signSEP45AuthEntries(challenge, contractAddress, expirationLedger, network.TestNetworkPassphrase)
		require.NoError(t, err)

		var signedEntries xdr.SorobanAuthorizationEntries
		require.NoError(t, xdr.SafeUnmarshalBase64(signedChallenge, &signedEntries))
		require.Len(t, signedEntries, 2)
		assert.Equal(t, serverEntry, signedEntries[0])

		credentials := signedEntries[1].Credentials.Address
		assert.Equal(t, xdr.Uint32(expirationLedger), credentials.SignatureExpirationLedger)
		require.NotNil(t, credentials.Signature.Map)
		assertion := **credentials.Signature.Map
		require.Len(t, assertion, 3)
		authenticatorData := []byte(*assertion[0].Val.Bytes)
		clientDataJSON := []byte(*assertion[1].Val.Bytes)
		signature := []byte(*assertion[2].Val.Bytes)

		// The challenge is the entry's authorization payload, and the flags are the ones the contract requires.
		payload, err := utils.BuildAuthorizationPayload(signedEntries[1], network.TestNetworkPassphrase)
		require.NoError(t, err)
		assert.Contains(t, string(clientDataJSON), `"challenge":"`+base64.RawURLEncoding.EncodeToString(payload[:])+`"`)
		assert.Equal(t, byte(passkeyAuthenticatorFlags), authenticatorData[32])

		// The signature covers authenticatorData || SHA-256(clientDataJSON) and is low-S.
		clientDataHash := sha256.Sum256(clientDataJSON)
		digest := sha256.Sum256(slices.Concat(authenticatorData, clientDataHash[:]))
		r, s := new(big.Int).SetBytes(signature[:32]), new(big.Int).SetBytes(signature[32:])
		assert.True(t, ecdsa.Verify(&pk.privateKey.PublicKey, digest[:], r, s))
		assert.LessOrEqual(t, s.Cmp(new(big.Int).Rsh(elliptic.P256().Params().N, 1)), 0)
	})

	t.Run("returns an error when no entry belongs to the contract", func(t *testing.T) {
		otherContractID := randomContractID(t)
		otherContractAddress := strkey.MustEncode(strkey.VersionByteContract, otherContractID[:])

		_, err := pk.signSEP45AuthEntries(challenge, otherContractAddress, 1234, network.TestNetworkPassphrase)
		assert.EqualError(t, err, "no authorization entry found for contract "+otherContractAddress)
	})
}

func randomContractID(t *testing.T) xdr.ContractId {
	t.Helper()
	var contractID xdr.ContractId
	_, err := rand.Read(contractID[:])
	require.NoError(t, err)
	return contractID
}

func newTestSEP45AuthEntry(address xdr.ScAddress) xdr.SorobanAuthorizationEntry {
	return xdr.SorobanAuthorizationEntry{
		Credentials: xdr.SorobanCredentials{
			Type: xdr.SorobanCredentialsTypeSorobanCredentialsAddress,
			Address: &xdr.SorobanAddressCredentials{
				Address:   address,
				Nonce:     1,
				Signature: xdr.ScVal{Type: xdr.ScValTypeScvVoid},
			},
		},
		RootInvocation: xdr.SorobanAuthorizedInvocation{
			Function: xdr.SorobanAuthorizedFunction{
				Type: xdr.SorobanAuthorizedFunctionTypeSorobanAuthorizedFunctionTypeContractFn,
				ContractFn: &xdr.InvokeContractArgs{
					ContractAddress: address,
					FunctionName:    "web_auth_verify",
				},
			},
		},
	}
}
