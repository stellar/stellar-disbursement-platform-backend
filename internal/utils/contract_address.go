package utils

import (
	"encoding/hex"
	"fmt"

	"github.com/stellar/go/hash"
	"github.com/stellar/go/strkey"
	"github.com/stellar/go/xdr"
)

// CalculateContractAddress calculates a Stellar smart contract address from a distribution account and salt.
//
// Contract addresses can be deterministically derived from the deployer account and an optional salt.
// This function takes a distribution account (deployer), salt in hex format, and network passphrase
// to calculate the contract address using XDR encoding.
//
// Read more: https://developers.stellar.org/docs/build/smart-contracts/example-contracts/deployer#how-it-works
func CalculateContractAddress(distributionAccount, saltHex, networkPassphrase string) (string, error) {
	saltBytes, err := hex.DecodeString(saltHex)
	if err != nil {
		return "", fmt.Errorf("invalid hex salt: %w", err)
	}
	if len(saltBytes) != 32 {
		return "", fmt.Errorf("salt must be 32 bytes, got %d", len(saltBytes))
	}
	var salt xdr.Uint256
	copy(salt[:], saltBytes)

	rawAddress, err := strkey.Decode(strkey.VersionByteAccountID, distributionAccount)
	if err != nil {
		return "", fmt.Errorf("invalid distribution account: %w", err)
	}
	var uint256Val xdr.Uint256
	copy(uint256Val[:], rawAddress)

	contractIdPreimage := xdr.ContractIdPreimage{
		Type: xdr.ContractIdPreimageTypeContractIdPreimageFromAddress,
		FromAddress: &xdr.ContractIdPreimageFromAddress{
			Address: xdr.ScAddress{
				Type: xdr.ScAddressTypeScAddressTypeAccount,
				AccountId: &xdr.AccountId{
					Type:    xdr.PublicKeyTypePublicKeyTypeEd25519,
					Ed25519: &uint256Val,
				},
			},
			Salt: salt,
		},
	}

	networkHash := hash.Hash([]byte(networkPassphrase))
	hashIdPreimage := xdr.HashIdPreimage{
		Type: xdr.EnvelopeTypeEnvelopeTypeContractId,
		ContractId: &xdr.HashIdPreimageContractId{
			NetworkId:          xdr.Hash(networkHash),
			ContractIdPreimage: contractIdPreimage,
		},
	}

	preimageXDR, err := hashIdPreimage.MarshalBinary()
	if err != nil {
		return "", fmt.Errorf("marshaling preimage: %w", err)
	}

	contractIdHash := hash.Hash(preimageXDR)
	return strkey.Encode(strkey.VersionByteContract, contractIdHash[:])
}

// CalculateContractAddressFromReceiver calculates a contract address using receiver contact information as salt.
//
// This function generates a salt from the receiver's contact information (email or phone number)
// and then calculates the contract address. Email takes precedence over phone number if both are provided.
func CalculateContractAddressFromReceiver(receiverEmail, receiverPhone, distributionAccount, networkPassphrase string) (string, error) {
	var receiverContact, contactType string

	if receiverEmail != "" {
		receiverContact = receiverEmail
		contactType = string(ContactTypeEmail)
	} else if receiverPhone != "" {
		receiverContact = receiverPhone
		contactType = string(ContactTypePhoneNumber)
	} else {
		return "", fmt.Errorf("receiver has no email or phone number")
	}

	saltHex, err := GenerateSalt(receiverContact, contactType)
	if err != nil {
		return "", err
	}

	return CalculateContractAddress(distributionAccount, saltHex, networkPassphrase)
}
