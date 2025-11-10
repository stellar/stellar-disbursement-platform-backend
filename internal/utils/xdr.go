package utils

import (
	"crypto/sha256"
	"fmt"

	"github.com/stellar/go/keypair"
	"github.com/stellar/go/network"
	"github.com/stellar/go/strkey"
	"github.com/stellar/go/xdr"
)

// NewSymbolStringEntry constructs an ScMapEntry for the provided key/value pair.
func NewSymbolStringEntry(key, value string) xdr.ScMapEntry {
	symbol := xdr.ScSymbol(key)
	str := xdr.ScString(value)
	return xdr.ScMapEntry{
		Key: xdr.ScVal{
			Type: xdr.ScValTypeScvSymbol,
			Sym:  &symbol,
		},
		Val: xdr.ScVal{
			Type: xdr.ScValTypeScvString,
			Str:  &str,
		},
	}
}

// BuildAuthorizationPayload produces the hash payload Soroban expects for signature verification.
func BuildAuthorizationPayload(entry xdr.SorobanAuthorizationEntry, networkPassphrase string) ([32]byte, error) {
	var zero [32]byte
	if entry.Credentials.Address == nil {
		return zero, fmt.Errorf("authorization entry missing address credentials")
	}

	preimage := xdr.HashIdPreimage{
		Type: xdr.EnvelopeTypeEnvelopeTypeSorobanAuthorization,
		SorobanAuthorization: &xdr.HashIdPreimageSorobanAuthorization{
			NetworkId:                 xdr.Hash(network.ID(networkPassphrase)),
			Nonce:                     entry.Credentials.Address.Nonce,
			SignatureExpirationLedger: entry.Credentials.Address.SignatureExpirationLedger,
			Invocation:                entry.RootInvocation,
		},
	}
	preimageBytes, err := preimage.MarshalBinary()
	if err != nil {
		return zero, fmt.Errorf("marshalling authorization preimage: %w", err)
	}
	payload := sha256.Sum256(preimageBytes)
	return payload, nil
}

// SignAuthEntry signs the authorization entry if it belongs to the provided signing account.
func SignAuthEntry(entry xdr.SorobanAuthorizationEntry, validUntil uint32, signingKP *keypair.Full, networkPassphrase string) (xdr.SorobanAuthorizationEntry, error) {
	if entry.Credentials.Type != xdr.SorobanCredentialsTypeSorobanCredentialsAddress {
		return entry, nil
	}
	if entry.Credentials.Address == nil {
		return entry, fmt.Errorf("address credentials missing")
	}

	addr := entry.Credentials.Address.Address
	if addr.Type != xdr.ScAddressTypeScAddressTypeAccount || addr.AccountId == nil {
		return entry, nil
	}

	serverAccountID := xdr.MustAddress(signingKP.Address())
	if !addr.AccountId.Equals(serverAccountID) {
		return entry, nil
	}

	encoded, err := entry.MarshalBinary()
	if err != nil {
		return entry, fmt.Errorf("copying authorization entry: %w", err)
	}

	var clone xdr.SorobanAuthorizationEntry
	if err := clone.UnmarshalBinary(encoded); err != nil {
		return entry, fmt.Errorf("cloning authorization entry: %w", err)
	}

	clone.Credentials.Address.SignatureExpirationLedger = xdr.Uint32(validUntil)

	payload, err := BuildAuthorizationPayload(clone, networkPassphrase)
	if err != nil {
		return entry, fmt.Errorf("encoding authorization preimage: %w", err)
	}

	signature, err := signingKP.Sign(payload[:])
	if err != nil {
		return entry, fmt.Errorf("signing authorization entry: %w", err)
	}
	if err := signingKP.Verify(payload[:], signature); err != nil {
		return entry, fmt.Errorf("signature verification failed: %w", err)
	}

	publicKeyRaw, err := strkey.Decode(strkey.VersionByteAccountID, signingKP.Address())
	if err != nil {
		return entry, fmt.Errorf("decoding signing public key: %w", err)
	}

	pkBytes := xdr.ScBytes(publicKeyRaw)
	sigBytes := xdr.ScBytes(signature)

	publicKeySymbol := xdr.ScSymbol("public_key")
	signatureSymbol := xdr.ScSymbol("signature")
	entries := xdr.ScMap{
		{
			Key: xdr.ScVal{
				Type: xdr.ScValTypeScvSymbol,
				Sym:  &publicKeySymbol,
			},
			Val: xdr.ScVal{
				Type:  xdr.ScValTypeScvBytes,
				Bytes: &pkBytes,
			},
		},
		{
			Key: xdr.ScVal{
				Type: xdr.ScValTypeScvSymbol,
				Sym:  &signatureSymbol,
			},
			Val: xdr.ScVal{
				Type:  xdr.ScValTypeScvBytes,
				Bytes: &sigBytes,
			},
		},
	}

	mapVal, err := xdr.NewScVal(xdr.ScValTypeScvMap, &entries)
	if err != nil {
		return entry, fmt.Errorf("building signature map: %w", err)
	}

	vector := xdr.ScVec{mapVal}
	vecVal, err := xdr.NewScVal(xdr.ScValTypeScvVec, &vector)
	if err != nil {
		return entry, fmt.Errorf("building signature vector: %w", err)
	}

	clone.Credentials.Address.Signature = vecVal
	return clone, nil
}
