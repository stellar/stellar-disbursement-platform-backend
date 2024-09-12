package passkey

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"fmt"

	"github.com/stellar/go/xdr"
)

// Passkey struct holds both the public and private keys
type Passkey struct {
	PrivateKey *ecdsa.PrivateKey
	PublicKey  *ecdsa.PublicKey
}

// NewPasskey generates a new Passkey with the secp256r1 (prime256v1) curve
func NewPasskey() (*Passkey, error) {
	// Generate a secp256r1 (prime256v1) key pair
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate key: %w", err)
	}

	// Return the passkey struct
	return &Passkey{
		PrivateKey: privateKey,
		PublicKey:  &privateKey.PublicKey,
	}, nil
}

// SignTransaction signs the given XDR string using the Passkey's private key
func (pk *Passkey) SignTransaction(txXDR string) ([]byte, error) {
	// Step 1: Hash the XDR transaction using SHA-256
	txHash := sha256.Sum256([]byte(txXDR))

	// Step 2: Sign the hashed transaction using the private key
	r, s, err := ecdsa.Sign(rand.Reader, pk.PrivateKey, txHash[:])
	if err != nil {
		return nil, fmt.Errorf("failed to sign XDR: %w", err)
	}

	// Step 3: Encode r and s as a signature (concatenate them)
	signature := append(r.Bytes(), s.Bytes()...)

	return signature, nil
}

// CreateDecoratedSignature creates a decorated signature using the public key and the provided signature
func (pk *Passkey) CreateDecoratedSignature(signature []byte) (xdr.DecoratedSignature, error) {
	var decoratedSig xdr.DecoratedSignature

	// Use the public key to create the hint (last 4 bytes of the key)
	pubKeyBytes := elliptic.Marshal(elliptic.P256(), pk.PublicKey.X, pk.PublicKey.Y)
	hint := pubKeyBytes[len(pubKeyBytes)-4:]
	// // Use the public key's X and Y values to create the hint
	// pubKeyBytes := append(pk.PublicKey.X.Bytes(), pk.PublicKey.Y.Bytes()...)
	// if len(pubKeyBytes) < 4 {
	// 	return decoratedSig, fmt.Errorf("public key bytes length is less than 4")
	// }
	// hint := pubKeyBytes[len(pubKeyBytes)-4:]

	// Set the hint and signature in the decorated signature
	decoratedSig.Hint = xdr.SignatureHint{}
	copy(decoratedSig.Hint[:], hint)
	decoratedSig.Signature = xdr.Signature(signature)

	return decoratedSig, nil
}
