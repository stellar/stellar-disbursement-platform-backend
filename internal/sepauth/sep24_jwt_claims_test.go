package sepauth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/assert"
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

func Test_ParseAccountAndMemo(t *testing.T) {
	testCases := []struct {
		name            string
		subject         string
		expectedAccount string
		expectedMemo    string
	}{
		{
			name:            "account only",
			subject:         "GBVFTZL5HIPT4PFQVTZVIWR77V7LWYCXU4CLYWWHHOEXB64XPG5LDMTU",
			expectedAccount: "GBVFTZL5HIPT4PFQVTZVIWR77V7LWYCXU4CLYWWHHOEXB64XPG5LDMTU",
			expectedMemo:    "",
		},
		{
			name:            "account with memo",
			subject:         "GBVFTZL5HIPT4PFQVTZVIWR77V7LWYCXU4CLYWWHHOEXB64XPG5LDMTU:12345",
			expectedAccount: "GBVFTZL5HIPT4PFQVTZVIWR77V7LWYCXU4CLYWWHHOEXB64XPG5LDMTU",
			expectedMemo:    "12345",
		},
		{
			name:            "empty string",
			subject:         "",
			expectedAccount: "",
			expectedMemo:    "",
		},
		{
			name:            "trailing colon",
			subject:         "GBVFTZL5HIPT4PFQVTZVIWR77V7LWYCXU4CLYWWHHOEXB64XPG5LDMTU:",
			expectedAccount: "GBVFTZL5HIPT4PFQVTZVIWR77V7LWYCXU4CLYWWHHOEXB64XPG5LDMTU",
			expectedMemo:    "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			account, memo := ParseAccountAndMemo(tc.subject)
			assert.Equal(t, tc.expectedAccount, account)
			assert.Equal(t, tc.expectedMemo, memo)
		})
	}
}

func Test_FormatSubject(t *testing.T) {
	testCases := []struct {
		name     string
		account  string
		memo     string
		expected string
	}{
		{
			name:     "account only",
			account:  "GBVFTZL5HIPT4PFQVTZVIWR77V7LWYCXU4CLYWWHHOEXB64XPG5LDMTU",
			memo:     "",
			expected: "GBVFTZL5HIPT4PFQVTZVIWR77V7LWYCXU4CLYWWHHOEXB64XPG5LDMTU",
		},
		{
			name:     "account with memo",
			account:  "GBVFTZL5HIPT4PFQVTZVIWR77V7LWYCXU4CLYWWHHOEXB64XPG5LDMTU",
			memo:     "12345",
			expected: "GBVFTZL5HIPT4PFQVTZVIWR77V7LWYCXU4CLYWWHHOEXB64XPG5LDMTU:12345",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := FormatSubject(tc.account, tc.memo)
			assert.Equal(t, tc.expected, result)
		})
	}
}
