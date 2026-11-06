package integrationtests

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/big"
	"slices"

	"github.com/stellar/go-stellar-sdk/strkey"
	"github.com/stellar/go-stellar-sdk/xdr"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

const (
	// Origin and relying party ID of the simulated WebAuthn ceremony, matching the tenant created by CreateTestData.
	passkeyOrigin = "http://localhost:3000"
	passkeyRPID   = "localhost"
	// User present (0x01) and user verified (0x04), the flags the smart wallet contract requires.
	passkeyAuthenticatorFlags = 0x05
)

// passkey simulates a receiver's WebAuthn passkey: a P-256 key that signs like a browser authenticator.
type passkey struct {
	privateKey *ecdsa.PrivateKey
}

func newPasskey() (*passkey, error) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generating P256 private key: %w", err)
	}
	return &passkey{privateKey: privateKey}, nil
}

// publicKeyHex returns the uncompressed public key, as sent in the embedded wallet creation request.
func (p *passkey) publicKeyHex() (string, error) {
	publicKey, err := p.privateKey.PublicKey.ECDH()
	if err != nil {
		return "", fmt.Errorf("converting P256 public key: %w", err)
	}
	return hex.EncodeToString(publicKey.Bytes()), nil
}

// signSEP45AuthEntries signs the SEP-45 challenge entry of the contract account, like the wallet frontend does, and
// returns the entries re-encoded. Entries of other addresses are left untouched.
func (p *passkey) signSEP45AuthEntries(authEntriesXDR, contractAddress string, expirationLedger uint32, networkPassphrase string) (string, error) {
	rawContractID, err := strkey.Decode(strkey.VersionByteContract, contractAddress)
	if err != nil {
		return "", fmt.Errorf("decoding contract address: %w", err)
	}
	var contractID xdr.ContractId
	copy(contractID[:], rawContractID)

	var entries xdr.SorobanAuthorizationEntries
	if err = xdr.SafeUnmarshalBase64(authEntriesXDR, &entries); err != nil {
		return "", fmt.Errorf("decoding authorization entries: %w", err)
	}

	signed := false
	for i := range entries {
		credentials := entries[i].Credentials.Address
		if credentials == nil || credentials.Address.ContractId == nil || *credentials.Address.ContractId != contractID {
			continue
		}

		credentials.SignatureExpirationLedger = xdr.Uint32(expirationLedger)
		payload, payloadErr := utils.BuildAuthorizationPayload(entries[i], networkPassphrase)
		if payloadErr != nil {
			return "", fmt.Errorf("building authorization payload: %w", payloadErr)
		}
		if credentials.Signature, err = p.webAuthnAssertion(payload[:]); err != nil {
			return "", fmt.Errorf("signing authorization payload: %w", err)
		}
		signed = true
	}
	if !signed {
		return "", fmt.Errorf("no authorization entry found for contract %s", contractAddress)
	}

	signedEntries, err := entries.MarshalBinary()
	if err != nil {
		return "", fmt.Errorf("encoding authorization entries: %w", err)
	}
	return base64.StdEncoding.EncodeToString(signedEntries), nil
}

// webAuthnAssertion returns the signature value the smart wallet contract verifies for the given challenge.
func (p *passkey) webAuthnAssertion(challenge []byte) (xdr.ScVal, error) {
	clientDataJSON := fmt.Appendf(nil, `{"type":"webauthn.get","challenge":"%s","origin":"%s","crossOrigin":false}`,
		base64.RawURLEncoding.EncodeToString(challenge), passkeyOrigin)
	rpIDHash := sha256.Sum256([]byte(passkeyRPID))
	// RP ID hash, flags and a 4-byte signature counter.
	authenticatorData := slices.Concat(rpIDHash[:], []byte{passkeyAuthenticatorFlags, 0, 0, 0, 0})
	clientDataHash := sha256.Sum256(clientDataJSON)
	digest := sha256.Sum256(slices.Concat(authenticatorData, clientDataHash[:]))

	r, s, err := ecdsa.Sign(rand.Reader, p.privateKey, digest[:])
	if err != nil {
		return xdr.ScVal{}, fmt.Errorf("signing WebAuthn assertion: %w", err)
	}
	// Soroban only accepts low-S signatures.
	curveOrder := elliptic.P256().Params().N
	if s.Cmp(new(big.Int).Rsh(curveOrder, 1)) > 0 {
		s.Sub(curveOrder, s)
	}
	signature := make([]byte, 64)
	r.FillBytes(signature[:32])
	s.FillBytes(signature[32:])

	// Soroban maps must have their keys sorted.
	assertion := &xdr.ScMap{
		symbolBytesEntry("authenticator_data", authenticatorData),
		symbolBytesEntry("client_data_json", clientDataJSON),
		symbolBytesEntry("signature", signature),
	}
	return xdr.ScVal{Type: xdr.ScValTypeScvMap, Map: &assertion}, nil
}

func symbolBytesEntry(key string, value []byte) xdr.ScMapEntry {
	symbol := xdr.ScSymbol(key)
	bytes := xdr.ScBytes(value)
	return xdr.ScMapEntry{
		Key: xdr.ScVal{Type: xdr.ScValTypeScvSymbol, Sym: &symbol},
		Val: xdr.ScVal{Type: xdr.ScValTypeScvBytes, Bytes: &bytes},
	}
}
