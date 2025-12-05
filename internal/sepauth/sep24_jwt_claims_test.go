package sepauth

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
		HomeDomainClaim:   "tenant.test.com:8080",
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        "test-transaction-id",
			Subject:   "GB54GWWWOSHATX5ALKHBBL2IQBZ2E7TBFO7F7VXKPIW6XANYDK4Y3RRC:123456",
			ExpiresAt: expiresAt,
		},
	}

	require.Equal(t, "test-transaction-id", claims.TransactionID())
	require.Equal(t, "GB54GWWWOSHATX5ALKHBBL2IQBZ2E7TBFO7F7VXKPIW6XANYDK4Y3RRC", claims.Account())
	require.Equal(t, "123456", claims.Memo())
	require.Equal(t, "test.com", claims.ClientDomain())
	require.Equal(t, "tenant.test.com:8080", claims.HomeDomain())
	require.Equal(t, expiresAt.Time, *claims.ExpiresAt())
}

func Test_SEP24JWTClaims_valid(t *testing.T) {
	now := time.Now()
	fiveMinAgo := now.Add(-5 * time.Minute)
	fiveMinFromNow := now.Add(5 * time.Minute)

	tests := []struct {
		name    string
		mutate  func(c *SEP24JWTClaims)
		wantErr string
	}{
		{
			name:    "missing expires_at",
			mutate:  func(c *SEP24JWTClaims) {},
			wantErr: "expires_at is required",
		},
		{
			name: "expired token",
			mutate: func(c *SEP24JWTClaims) {
				c.RegisteredClaims.ExpiresAt = jwt.NewNumericDate(fiveMinAgo)
			},
			wantErr: "token is expired",
		},
		{
			name: "missing transaction id",
			mutate: func(c *SEP24JWTClaims) {
				c.RegisteredClaims.ExpiresAt = jwt.NewNumericDate(fiveMinFromNow)
			},
			wantErr: "transaction_id is required",
		},
		{
			name: "missing subject",
			mutate: func(c *SEP24JWTClaims) {
				c.RegisteredClaims.ExpiresAt = jwt.NewNumericDate(fiveMinFromNow)
				c.ID = "test-transaction-id"
			},
			wantErr: "stellar account is invalid",
		},
		{
			name: "invalid subject",
			mutate: func(c *SEP24JWTClaims) {
				c.RegisteredClaims.ExpiresAt = jwt.NewNumericDate(fiveMinFromNow)
				c.ID = "test-transaction-id"
				c.Subject = "invalid"
			},
			wantErr: "stellar account is invalid",
		},
		{
			name: "invalid client domain",
			mutate: func(c *SEP24JWTClaims) {
				c.RegisteredClaims.ExpiresAt = jwt.NewNumericDate(fiveMinFromNow)
				c.ID = "test-transaction-id"
				c.Subject = "GB54GWWWOSHATX5ALKHBBL2IQBZ2E7TBFO7F7VXKPIW6XANYDK4Y3RRC:123456"
				c.ClientDomainClaim = "localhost:8000"
			},
			wantErr: `client_domain is invalid`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var claims SEP24JWTClaims
			tt.mutate(&claims)
			err := claims.Valid()
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.wantErr)
		})
	}

	t.Run("valid pubkey subject", func(t *testing.T) {
		claims := SEP24JWTClaims{
			ClientDomainClaim: "test.com",
			HomeDomainClaim:   "tenant.test.com",
			RegisteredClaims: jwt.RegisteredClaims{
				ID:        "test-transaction-id",
				Subject:   "GB54GWWWOSHATX5ALKHBBL2IQBZ2E7TBFO7F7VXKPIW6XANYDK4Y3RRC:123456",
				ExpiresAt: jwt.NewNumericDate(fiveMinFromNow),
			},
		}
		require.NoError(t, claims.Valid())
	})

	t.Run("valid contract subject", func(t *testing.T) {
		claims := SEP24JWTClaims{
			ClientDomainClaim: "test.com",
			HomeDomainClaim:   "tenant.test.com",
			RegisteredClaims: jwt.RegisteredClaims{
				ID:        "test-transaction-id",
				Subject:   "CCYU2FUIMK23K34U3SWCN2O2JVI6JBGUGQUILYK7GRPCIDABVVTCS7R4",
				ExpiresAt: jwt.NewNumericDate(fiveMinFromNow),
			},
		}
		require.NoError(t, claims.Valid())
	})
}
