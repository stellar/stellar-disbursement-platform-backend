package utils

import (
	"testing"

	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/network"
	"github.com/stellar/go-stellar-sdk/strkey"
	"github.com/stellar/go-stellar-sdk/xdr"
	"github.com/stretchr/testify/require"
)

func Test_NewSymbolStringEntry(t *testing.T) {
	entry := NewSymbolStringEntry("foo", "bar")
	require.Equal(t, xdr.ScValTypeScvSymbol, entry.Key.Type)
	require.NotNil(t, entry.Key.Sym)
	require.Equal(t, xdr.ScSymbol("foo"), *entry.Key.Sym)

	require.Equal(t, xdr.ScValTypeScvString, entry.Val.Type)
	require.NotNil(t, entry.Val.Str)
	require.Equal(t, xdr.ScString("bar"), *entry.Val.Str)
}

func Test_BuildAuthorizationPayload(t *testing.T) {
	testCases := []struct {
		name        string
		entry       xdr.SorobanAuthorizationEntry
		expectError string
	}{
		{
			name: "missing address credentials returns error",
			entry: xdr.SorobanAuthorizationEntry{
				Credentials: xdr.SorobanCredentials{Type: xdr.SorobanCredentialsTypeSorobanCredentialsAddress},
			},
			expectError: "authorization entry missing address credentials",
		},
		{
			name:  "returns payload for valid entry",
			entry: newTestAuthEntry(t, keypair.MustRandom().Address()),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			payload, err := BuildAuthorizationPayload(tc.entry, network.TestNetworkPassphrase)
			if tc.expectError != "" {
				require.ErrorContains(t, err, tc.expectError)
				return
			}
			require.NoError(t, err)
			require.NotEqual(t, [32]byte{}, payload)
		})
	}
}

func Test_SignAuthEntry(t *testing.T) {
	serverKP := keypair.MustRandom()

	testCases := []struct {
		name           string
		buildEntry     func(t *testing.T) xdr.SorobanAuthorizationEntry
		validUntil     uint32
		shouldSign     bool
		signatureCount int
	}{
		{
			name:       "non-server account remains unchanged",
			validUntil: 200,
			buildEntry: func(t *testing.T) xdr.SorobanAuthorizationEntry {
				return newTestAuthEntry(t, keypair.MustRandom().Address())
			},
		},
		{
			name:       "server account gets signed",
			validUntil: 500,
			shouldSign: true,
			buildEntry: func(t *testing.T) xdr.SorobanAuthorizationEntry {
				return newTestAuthEntry(t, serverKP.Address())
			},
			signatureCount: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			entry := tc.buildEntry(t)
			origCopy := entry
			result, err := SignAuthEntry(entry, tc.validUntil, serverKP, network.TestNetworkPassphrase)
			require.NoError(t, err)

			if !tc.shouldSign {
				require.Equal(t, origCopy, result)
				return
			}

			require.Equal(t, xdr.Uint32(tc.validUntil), result.Credentials.Address.SignatureExpirationLedger)
			sigVec, ok := result.Credentials.Address.Signature.GetVec()
			require.True(t, ok)
			require.NotNil(t, sigVec)
			require.Len(t, *sigVec, tc.signatureCount)

			sigMap, ok := (*sigVec)[0].GetMap()
			require.True(t, ok)
			require.NotNil(t, sigMap)
			require.Len(t, *sigMap, 2)

			var (
				extractedPub []byte
				extractedSig []byte
			)
			for _, entry := range *sigMap {
				key := entry.Key.MustSym()
				switch string(key) {
				case "public_key":
					bytesVal, ok := entry.Val.GetBytes()
					require.True(t, ok)
					extractedPub = append([]byte(nil), bytesVal...)
				case "signature":
					bytesVal, ok := entry.Val.GetBytes()
					require.True(t, ok)
					extractedSig = append([]byte(nil), bytesVal...)
				}
			}
			require.NotEmpty(t, extractedPub)
			require.NotEmpty(t, extractedSig)

			serverPubRaw, err := strkey.Decode(strkey.VersionByteAccountID, serverKP.Address())
			require.NoError(t, err)
			require.Equal(t, serverPubRaw, extractedPub)

			payload, err := BuildAuthorizationPayload(result, network.TestNetworkPassphrase)
			require.NoError(t, err)
			require.NoError(t, serverKP.Verify(payload[:], extractedSig))
		})
	}
}

func Test_ExtractArgsMap(t *testing.T) {
	mapVal := func(entries xdr.ScMap) xdr.ScVal {
		val, err := xdr.NewScVal(xdr.ScValTypeScvMap, &entries)
		require.NoError(t, err)
		return val
	}

	t.Run("success", func(t *testing.T) {
		entries := xdr.ScMap{
			NewSymbolStringEntry("account", " CABC"),
			NewSymbolStringEntry("home_domain", "example.com "),
		}
		args := xdr.ScVec{mapVal(entries)}
		result, err := ExtractArgsMap(args)
		require.NoError(t, err)
		require.Equal(t, map[string]string{
			"account":     "CABC",
			"home_domain": "example.com",
		}, result)
	})

	t.Run("requires single map argument", func(t *testing.T) {
		_, err := ExtractArgsMap(xdr.ScVec{})
		require.ErrorContains(t, err, "single argument map")
	})

	t.Run("argument must be map", func(t *testing.T) {
		str := xdr.ScString("value")
		val := xdr.ScVal{Type: xdr.ScValTypeScvString, Str: &str}
		_, err := ExtractArgsMap(xdr.ScVec{val})
		require.ErrorContains(t, err, "arguments must be a map")
	})

	t.Run("entries must be strings", func(t *testing.T) {
		bytesVal := xdr.ScBytes{0x1}
		entries := xdr.ScMap{
			{
				Key: xdr.ScVal{Type: xdr.ScValTypeScvSymbol, Sym: func() *xdr.ScSymbol { sym := xdr.ScSymbol("account"); return &sym }()},
				Val: xdr.ScVal{Type: xdr.ScValTypeScvBytes, Bytes: &bytesVal},
			},
		}
		args := xdr.ScVec{mapVal(entries)}
		_, err := ExtractArgsMap(args)
		require.ErrorContains(t, err, "must be a string")
	})
}

func newTestAuthEntry(t *testing.T, account string) xdr.SorobanAuthorizationEntry {
	t.Helper()

	accountID := xdr.MustAddress(account)
	accountAddress := xdr.ScAddress{
		Type:      xdr.ScAddressTypeScAddressTypeAccount,
		AccountId: &accountID,
	}
	var contractID xdr.ContractId
	for i := range contractID {
		contractID[i] = byte(i + 1)
	}
	contractAddress := xdr.ScAddress{
		Type:       xdr.ScAddressTypeScAddressTypeContract,
		ContractId: &contractID,
	}

	emptyVec := xdr.ScVec{}
	emptySignature, err := xdr.NewScVal(xdr.ScValTypeScvVec, &emptyVec)
	require.NoError(t, err)

	return xdr.SorobanAuthorizationEntry{
		Credentials: xdr.SorobanCredentials{
			Type: xdr.SorobanCredentialsTypeSorobanCredentialsAddress,
			Address: &xdr.SorobanAddressCredentials{
				Address:                   accountAddress,
				Nonce:                     xdr.Int64(42),
				SignatureExpirationLedger: xdr.Uint32(10),
				Signature:                 emptySignature,
			},
		},
		RootInvocation: xdr.SorobanAuthorizedInvocation{
			Function: xdr.SorobanAuthorizedFunction{
				Type: xdr.SorobanAuthorizedFunctionTypeSorobanAuthorizedFunctionTypeContractFn,
				ContractFn: &xdr.InvokeContractArgs{
					ContractAddress: contractAddress,
					FunctionName:    xdr.ScSymbol("web_auth_verify"),
					Args:            xdr.ScVec{},
				},
			},
			SubInvocations: nil,
		},
	}
}
