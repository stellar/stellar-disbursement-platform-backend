package schema

import (
	"encoding/hex"
	"fmt"
	"strconv"

	"github.com/stellar/go/txnbuild"
)

type Memo struct {
	Value string
	Type  MemoType
}

type MemoType string

const (
	MemoTypeText   MemoType = "text"
	MemoTypeID     MemoType = "id"
	MemoTypeHash   MemoType = "hash"
	MemoTypeReturn MemoType = "return"
)

// NewMemo creates a new Memo from a MemoType and a string value.
func NewMemo(memoType MemoType, memoValue string) (txnbuild.Memo, error) {
	switch memoType {
	case "":
		return nil, nil

	case MemoTypeText:
		// Memo Text (up to 28 bytes)
		if len(memoValue) > 28 {
			return nil, fmt.Errorf("text memo must be 28 bytes or less")
		}
		return txnbuild.MemoText(memoValue), nil

	case MemoTypeID:
		// Memo ID (uint64)
		id, err := strconv.ParseUint(memoValue, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid Memo ID value, must be a uint64: %w", err)
		}
		return txnbuild.MemoID(id), nil

	case MemoTypeHash:
		// Memo Hash (32-byte hash)
		if len(memoValue) != 64 {
			return nil, fmt.Errorf("hash memo must be 64 hex characters (32 bytes)")
		}
		hashBytes, err := hexStringToBytes(memoValue)
		if err != nil {
			return nil, fmt.Errorf("invalid hash format: %w", err)
		}
		return txnbuild.MemoHash(hashBytes), nil

	case MemoTypeReturn:
		// Memo Return (32-byte hash)
		if len(memoValue) != 64 {
			return nil, fmt.Errorf("return memo must be 64 hex characters (32 bytes)")
		}
		hashBytes, err := hexStringToBytes(memoValue)
		if err != nil {
			return nil, fmt.Errorf("invalid return hash format: %w", err)
		}
		return txnbuild.MemoReturn(hashBytes), nil

	default:
		return nil, fmt.Errorf("unknown memo type: %s", memoType)
	}
}

// hexStringToBytes is a utility function to convert a hex string to a byte slice.
func hexStringToBytes(hexStr string) ([]byte, error) {
	bytes, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, fmt.Errorf("decoding hex string %s: %w", hexStr, err)
	}
	return bytes, nil
}
