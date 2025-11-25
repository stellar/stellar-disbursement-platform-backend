package schema

import (
	"fmt"
	"testing"

	"github.com/stellar/go-stellar-sdk/txnbuild"
	"github.com/stretchr/testify/require"
)

func Test_hexStringToBytes(t *testing.T) {
	hexStr := "12f37f82eb6708daa0ac372a1a67a0f33efa6a9cd213ed430517e45fefb51577"
	wantBytes := []byte{0x12, 0xf3, 0x7f, 0x82, 0xeb, 0x67, 0x08, 0xda, 0xa0, 0xac, 0x37, 0x2a, 0x1a, 0x67, 0xa0, 0xf3, 0x3e, 0xfa, 0x6a, 0x9c, 0xd2, 0x13, 0xed, 0x43, 0x05, 0x17, 0xe4, 0x5f, 0xef, 0xb5, 0x15, 0x77}

	got, err := hexStringToBytes(hexStr)
	require.NoError(t, err)
	require.Equal(t, wantBytes, got)
}

func Test_NewMemo(t *testing.T) {
	testCases := []struct {
		memoType        MemoType
		memoValue       string
		wantMemo        txnbuild.Memo
		wantErrContains string
	}{
		{
			memoType:  "",
			memoValue: "",
			wantMemo:  nil,
		},
		{
			memoType:        MemoTypeText,
			memoValue:       "This is a very long text that should exceed the 28-byte limit",
			wantErrContains: "text memo must be 28 bytes or less",
		},
		{
			memoType:        MemoTypeText,
			memoValue:       "HelloWorld!",
			wantMemo:        txnbuild.MemoText("HelloWorld!"),
			wantErrContains: "",
		},
		{
			memoType:        MemoTypeID,
			memoValue:       "not-a-valid-uint64",
			wantErrContains: "invalid Memo ID value, must be a uint64",
		},
		{
			memoType:        MemoTypeID,
			memoValue:       "1234567890",
			wantMemo:        txnbuild.MemoID(1234567890),
			wantErrContains: "",
		},
		{
			memoType:        MemoTypeHash,
			memoValue:       "12f37f82eb6708daa0ac372a1a67a0f33efa6a9cd213ed430517e45fefb5157712f37f82eb6708daa0ac372a1a67a0f33efa6a9cd213ed430517e45fefb51577",
			wantErrContains: "hash memo must be 64 hex characters (32 bytes)",
		},
		{
			memoType:        MemoTypeHash,
			memoValue:       "12f37f82eb6708daa0ac372a1a67a0f33efa6a9cd213ed430517e45fefb51577",
			wantMemo:        txnbuild.MemoHash([]byte{0x12, 0xf3, 0x7f, 0x82, 0xeb, 0x67, 0x08, 0xda, 0xa0, 0xac, 0x37, 0x2a, 0x1a, 0x67, 0xa0, 0xf3, 0x3e, 0xfa, 0x6a, 0x9c, 0xd2, 0x13, 0xed, 0x43, 0x05, 0x17, 0xe4, 0x5f, 0xef, 0xb5, 0x15, 0x77}),
			wantErrContains: "",
		},
		{
			memoType:        MemoTypeReturn,
			memoValue:       "12f37f82eb6708daa0ac372a1a67a0f33efa6a9cd213ed430517e45fefb5157712f37f82eb6708daa0ac372a1a67a0f33efa6a9cd213ed430517e45fefb51577",
			wantErrContains: "return memo must be 64 hex characters (32 bytes)",
		},
		{
			memoType:        MemoTypeReturn,
			memoValue:       "12f37f82eb6708daa0ac372a1a67a0f33efa6a9cd213ed430517e45fefb51577",
			wantMemo:        txnbuild.MemoReturn([]byte{0x12, 0xf3, 0x7f, 0x82, 0xeb, 0x67, 0x08, 0xda, 0xa0, 0xac, 0x37, 0x2a, 0x1a, 0x67, 0xa0, 0xf3, 0x3e, 0xfa, 0x6a, 0x9c, 0xd2, 0x13, 0xed, 0x43, 0x05, 0x17, 0xe4, 0x5f, 0xef, 0xb5, 0x15, 0x77}),
			wantErrContains: "",
		},
	}

	for _, tc := range testCases {
		emojiPrefix := "🟢"
		if tc.wantErrContains != "" {
			emojiPrefix = "🔴"
		}
		t.Run(fmt.Sprintf("%s%s(%s)", emojiPrefix, tc.memoType, tc.memoValue), func(t *testing.T) {
			gotMemo, err := NewMemo(tc.memoType, tc.memoValue)
			if tc.wantErrContains == "" {
				require.NoError(t, err)
				require.Equal(t, tc.wantMemo, gotMemo)
			} else {
				require.ErrorContains(t, err, tc.wantErrContains)
				require.Nil(t, gotMemo)
			}
		})
	}
}

func Test_ParseMemo(t *testing.T) {
	testCases := []struct {
		memoValue       string
		wantMemo        txnbuild.Memo
		wantType        MemoType
		wantErrContains string
	}{
		{
			memoValue: "0",
			wantMemo:  txnbuild.MemoID(0),
			wantType:  MemoTypeID,
		},
		{
			memoValue: "HelloWorld!",
			wantMemo:  txnbuild.MemoText("HelloWorld!"),
			wantType:  MemoTypeText,
		},
		{
			memoValue: "12f37f82eb6708daa0ac372a1a67a0f33efa6a9cd213ed430517e45fefb51577",
			wantMemo:  txnbuild.MemoHash([]byte{0x12, 0xf3, 0x7f, 0x82, 0xeb, 0x67, 0x08, 0xda, 0xa0, 0xac, 0x37, 0x2a, 0x1a, 0x67, 0xa0, 0xf3, 0x3e, 0xfa, 0x6a, 0x9c, 0xd2, 0x13, 0xed, 0x43, 0x05, 0x17, 0xe4, 0x5f, 0xef, 0xb5, 0x15, 0x77}),
			wantType:  MemoTypeHash,
		},
		{
			memoValue: "",
		},
		{
			memoValue:       "this-string-is-not-a-valid-memo-because-it's-not-uint-and-too-long-for-a-text-and-not-a-valid-hex",
			wantErrContains: `could not parse value "this-string-is-not-a-valid-memo-because-it's-not-uint-and-too-long-for-a-text-and-not-a-valid-hex" as any valid memo type`,
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("ParseMemo(%q)", tc.memoValue), func(t *testing.T) {
			gotMemo, gotType, err := ParseMemo(tc.memoValue)
			if tc.wantErrContains == "" {
				require.NoError(t, err)
				require.Equal(t, tc.wantMemo, gotMemo)
				require.Equal(t, tc.wantType, gotType)
			} else {
				require.ErrorContains(t, err, tc.wantErrContains)
				require.Nil(t, gotMemo)
				require.Equal(t, tc.wantType, gotType)
			}
		})
	}
}
