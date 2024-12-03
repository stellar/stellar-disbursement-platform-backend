package circle

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_RecipientRequest_validate(t *testing.T) {
	idempotencyKey := uuid.NewString()

	testCases := []struct {
		name            string
		abr             *RecipientRequest
		wantErrContains string
	}{
		{
			name:            "🔴IdempotencyKey is required",
			abr:             &RecipientRequest{},
			wantErrContains: "idempotency key must be provided",
		},
		{
			name: "🔴IdempotencyKey must be a valid uuid",
			abr: &RecipientRequest{
				IdempotencyKey: "invalid-idempotency-key",
			},
			wantErrContains: "idempotency key is not a valid UUID",
		},
		{
			name: "🔴Address is required",
			abr: &RecipientRequest{
				IdempotencyKey: idempotencyKey,
			},
			wantErrContains: "address must be provided",
		},
		{
			name: "🔴Address must be a valid Stellar public key",
			abr: &RecipientRequest{
				IdempotencyKey: idempotencyKey,
				Address:        "invalid",
			},
			wantErrContains: "address is not a valid Stellar public key",
		},
		{
			name: "🔴Chain must be XLM or left empty",
			abr: &RecipientRequest{
				IdempotencyKey: idempotencyKey,
				Address:        "GCESKSSHPZKB6IE67LFZRZBGSX2FTHP4LUOIOZ54BUQFHYCQGH3WGUNX",
				Chain:          "FOO",
			},
			wantErrContains: `invalid chain provided "FOO"`,
		},
		{
			name: "🔴Matadata is required",
			abr: &RecipientRequest{
				IdempotencyKey: idempotencyKey,
				Address:        "GCESKSSHPZKB6IE67LFZRZBGSX2FTHP4LUOIOZ54BUQFHYCQGH3WGUNX",
				Chain:          StellarChainCode,
			},
			wantErrContains: "metadata must be provided",
		},
		{
			name: "🔴Metadata.Nickname is required",
			abr: &RecipientRequest{
				IdempotencyKey: idempotencyKey,
				Address:        "GCESKSSHPZKB6IE67LFZRZBGSX2FTHP4LUOIOZ54BUQFHYCQGH3WGUNX",
				Chain:          StellarChainCode,
				Metadata: RecipientMetadata{
					BNS: "bns",
				},
			},
			wantErrContains: "metadata nickname must be provided",
		},
		{
			name: "🔴Metadata.Email must be a valid email",
			abr: &RecipientRequest{
				IdempotencyKey: idempotencyKey,
				Address:        "GCESKSSHPZKB6IE67LFZRZBGSX2FTHP4LUOIOZ54BUQFHYCQGH3WGUNX",
				Chain:          StellarChainCode,
				Metadata: RecipientMetadata{
					Nickname: "+14155556789",
					Email:    "invalid.email",
				},
			},
			wantErrContains: "metadata email is not a valid email",
		},
		{
			name: "🟢valid without email nor chain",
			abr: &RecipientRequest{
				IdempotencyKey: idempotencyKey,
				Address:        "GCESKSSHPZKB6IE67LFZRZBGSX2FTHP4LUOIOZ54BUQFHYCQGH3WGUNX",
				Metadata: RecipientMetadata{
					Nickname: "+14155556789",
				},
			},
		},
		{
			name: "🟢valid with email and chain",
			abr: &RecipientRequest{
				IdempotencyKey: idempotencyKey,
				Address:        "GCESKSSHPZKB6IE67LFZRZBGSX2FTHP4LUOIOZ54BUQFHYCQGH3WGUNX",
				Chain:          StellarChainCode,
				Metadata: RecipientMetadata{
					Nickname: "+14155556789",
					Email:    "test@test.com",
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.abr.validate()
			if tc.wantErrContains == "" {
				require.NoError(t, err)
				assert.Equal(t, StellarChainCode, tc.abr.Chain)
			} else {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErrContains)
			}
		})
	}
}
