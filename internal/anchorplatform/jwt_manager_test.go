package anchorplatform

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewJWTManager(t *testing.T) {
	jwtManager, err := NewJWTManager("", 0)
	require.Nil(t, jwtManager)
	require.EqualError(t, err, "secret is required to have at least 12 characteres")

	jwtManager, err = NewJWTManager("1234567890ab", 0)
	require.Nil(t, jwtManager)
	require.EqualError(t, err, "expiration miliseconds is required to be at least 5000")

	jwtManager, err = NewJWTManager("1234567890ab", 5000)
	require.NotNil(t, jwtManager)
	require.NoError(t, err)
	wantManager := &JWTManager{
		secret:                []byte("1234567890ab"),
		expirationMiliseconds: 5000,
	}
	require.Equal(t, wantManager, jwtManager)
}

func Test_JWTManager_GenerateAndParseSEP24Token(t *testing.T) {
	jwtManager, err := NewJWTManager("1234567890ab", 5000)
	require.NoError(t, err)

	// invalid claims
	tokenStr, err := jwtManager.GenerateSEP24Token("", "", "test.com", "test-transaction-id")
	require.EqualError(t, err, "validating SEP24 token claims: stellar account is invalid: strkey is 0 bytes long; minimum valid length is 5")
	require.Empty(t, tokenStr)

	// valid claims 🎉
	tokenStr, err = jwtManager.GenerateSEP24Token("GB54GWWWOSHATX5ALKHBBL2IQBZ2E7TBFO7F7VXKPIW6XANYDK4Y3RRC", "123456", "test.com", "test-transaction-id")
	require.NoError(t, err)
	require.NotEmpty(t, tokenStr)
	now := time.Now()

	// parse claims
	claims, err := jwtManager.ParseSEP24TokenClaims(tokenStr)
	require.NoError(t, err)
	assert.Nil(t, claims.Valid())
	assert.Equal(t, "test-transaction-id", claims.TransactionID())
	assert.Equal(t, "GB54GWWWOSHATX5ALKHBBL2IQBZ2E7TBFO7F7VXKPIW6XANYDK4Y3RRC", claims.SEP10StellarAccount())
	assert.Equal(t, "123456", claims.SEP10StellarMemo())
	assert.Equal(t, "test.com", claims.ClientDomain())
	assert.True(t, claims.ExpiresAt().After(now.Add(time.Duration(4000*time.Millisecond))))
	assert.True(t, claims.ExpiresAt().Before(now.Add(time.Duration(5000*time.Millisecond))))
}

func Test_JWTManager_GenerateAndParseDefaultToken(t *testing.T) {
	jwtManager, err := NewJWTManager("1234567890ab", 5000)
	require.NoError(t, err)

	// valid claims 🎉
	tokenStr, err := jwtManager.GenerateDefaultToken("test-transaction-id")
	require.NoError(t, err)
	require.NotEmpty(t, tokenStr)
	now := time.Now()

	// parse claims
	claims, err := jwtManager.ParseDefaultTokenClaims(tokenStr)
	require.NoError(t, err)
	assert.Nil(t, claims.Valid())
	assert.Equal(t, "test-transaction-id", claims.ID)
	assert.Equal(t, "stellar-disbursement-platform-backend", claims.Subject)
	assert.True(t, claims.ExpiresAt.After(now.Add(time.Duration(4000*time.Millisecond))))
	assert.True(t, claims.ExpiresAt.Before(now.Add(time.Duration(5000*time.Millisecond))))
}
