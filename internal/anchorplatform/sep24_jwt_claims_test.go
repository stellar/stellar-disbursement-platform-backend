package anchorplatform

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/require"
)

func Test_SEP24JWTClaims_getters(t *testing.T) {
	expiresAt := jwt.NewNumericDate(time.Now().Add(time.Minute * 5))
	claims := SEP24JWTClaims{
		ClientDomainClaim: "test.com",
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        "test-transaction-id",
			Subject:   "GB54GWWWOSHATX5ALKHBBL2IQBZ2E7TBFO7F7VXKPIW6XANYDK4Y3RRC:123456",
			ExpiresAt: expiresAt,
		},
	}

	require.Equal(t, "test-transaction-id", claims.TransactionID())
	require.Equal(t, "GB54GWWWOSHATX5ALKHBBL2IQBZ2E7TBFO7F7VXKPIW6XANYDK4Y3RRC", claims.SEP10StellarAccount())
	require.Equal(t, "123456", claims.SEP10StellarMemo())
	require.Equal(t, "test.com", claims.ClientDomain())
	require.Equal(t, expiresAt.Time, *claims.ExpiresAt())
}

func Test_SEP24JWTClaims_valid(t *testing.T) {
	// empty claims
	claims := SEP24JWTClaims{}
	err := claims.Valid()
	require.EqualError(t, err, "expires_at is required")

	// expired claims
	now := time.Now()
	fiveMinAgo := now.Add(time.Minute * -5)
	claims.RegisteredClaims.ExpiresAt = jwt.NewNumericDate(fiveMinAgo)
	err = claims.Valid()
	require.Contains(t, err.Error(), "validating registered claims: token is expired by 5m0")

	// missing transaction ID
	fiveMinFromNow := now.Add(time.Minute * 5)
	claims.RegisteredClaims.ExpiresAt = jwt.NewNumericDate(fiveMinFromNow)
	err = claims.Valid()
	require.EqualError(t, err, "transaction_id is required")

	// missing subject
	claims.ID = "test-transaction-id"
	err = claims.Valid()
	require.EqualError(t, err, "stellar account is invalid: strkey is 0 bytes long; minimum valid length is 5")

	// invalid subject
	claims.Subject = "invalid"
	err = claims.Valid()
	require.EqualError(t, err, "stellar account is invalid: base32 decode failed: illegal base32 data at input byte 7")

	// invalid client domain
	claims.Subject = "GB54GWWWOSHATX5ALKHBBL2IQBZ2E7TBFO7F7VXKPIW6XANYDK4Y3RRC:123456"
	claims.ClientDomainClaim = "localhost:8000"
	err = claims.Valid()
	require.EqualError(t, err, `client_domain is invalid: "localhost:8000" is not a valid DNS name`)

	// valid claims 🎉
	claims.ClientDomainClaim = "test.com"
	err = claims.Valid()
	require.NoError(t, err)
}
