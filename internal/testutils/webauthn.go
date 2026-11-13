package testutils

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"math/big"
	"slices"
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/require"
)

// RSAPackedAttestationBody builds a passkey registration/finish body that makes the server verify an RSA key of
// modulusLen bytes, as the credential key or, with viaCertificate, inside an x5c certificate. Tests use it to check
// that oversized keys are rejected before verification. The signature is garbage; keep modulusLen small.
func RSAPackedAttestationBody(t *testing.T, rpID, origin, challenge string, modulusLen int, viaCertificate bool) []byte {
	t.Helper()
	modulus := bytes.Repeat([]byte{0xff}, modulusLen)
	modulus[0], modulus[modulusLen-1] = 0xc3, 0x01 // odd, top bits set
	signature := bytes.Repeat([]byte{0x5a}, modulusLen)
	signature[0] = 0x01 // numerically below the modulus
	const algRS256 = -257

	attStmt := map[string]any{"alg": algRS256, "sig": signature}
	var coseKey []byte
	var err error
	if viaCertificate {
		ecKey, genErr := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, genErr)
		coseKey, err = cbor.Marshal(map[int]any{
			1: 2, 3: -7, -1: 1, -2: ecKey.X.FillBytes(make([]byte, 32)), -3: ecKey.Y.FillBytes(make([]byte, 32)),
		})
		require.NoError(t, err)
		template := &x509.Certificate{
			SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "attestation"},
			NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		}
		rsaPub := &rsa.PublicKey{N: new(big.Int).SetBytes(modulus), E: 65537}
		der, certErr := x509.CreateCertificate(rand.Reader, template, template, rsaPub, ecKey)
		require.NoError(t, certErr)
		attStmt["x5c"] = []any{der}
	} else {
		// COSE_Key: kty 3 (RSA), alg RS256, -1 = n, -2 = e
		coseKey, err = cbor.Marshal(map[int]any{1: 3, 3: algRS256, -1: modulus, -2: []byte{0x01, 0x00, 0x01}})
		require.NoError(t, err)
	}

	rawID := []byte("rsa-credential")
	rpIDHash := sha256.Sum256([]byte(rpID))
	const flags = 0x45 // user present, user verified, attested credential data present
	authenticatorData := slices.Concat(rpIDHash[:], []byte{flags, 0, 0, 0, 0},
		make([]byte, 16), binary.BigEndian.AppendUint16(nil, uint16(len(rawID))), rawID, coseKey)
	attestationObject, err := cbor.Marshal(map[string]any{"fmt": "packed", "attStmt": attStmt, "authData": authenticatorData})
	require.NoError(t, err)
	clientDataJSON := fmt.Appendf(nil, `{"type":"webauthn.create","challenge":%q,"origin":%q,"crossOrigin":false}`, challenge, origin)

	b64 := base64.RawURLEncoding.EncodeToString
	return fmt.Appendf(nil, `{"id":%q,"rawId":%q,"type":"public-key","response":{"clientDataJSON":%q,"attestationObject":%q}}`,
		b64(rawID), b64(rawID), b64(clientDataJSON), b64(attestationObject))
}
