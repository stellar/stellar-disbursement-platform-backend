package anchorplatform

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
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
	tokenStr, err := jwtManager.GenerateSEP24Token("", "", "test.com", "test-home-domain.com:3000", "test-transaction-id")
	require.EqualError(t, err, "validating SEP24 token claims: stellar account is invalid: strkey is 0 bytes long; minimum valid length is 5")
	require.Empty(t, tokenStr)

	// valid claims 🎉
	tokenStr, err = jwtManager.GenerateSEP24Token("GB54GWWWOSHATX5ALKHBBL2IQBZ2E7TBFO7F7VXKPIW6XANYDK4Y3RRC", "123456", "test.com", "test-home-domain.com:3000", "test-transaction-id")
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
	assert.Equal(t, "test-home-domain.com:3000", claims.HomeDomain())
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

func Test_JWTManager_GenerateAndParseSEP10Token(t *testing.T) {
	jwtManager, err := NewJWTManager("1234567890ab", 5000)
	require.NoError(t, err)

	now := time.Now()
	iat := now
	exp := now.Add(5 * time.Minute)

	testCases := []struct {
		name         string
		issuer       string
		subject      string
		jti          string
		clientDomain string
		homeDomain   string
		iat          time.Time
		exp          time.Time
		wantErr      bool
		errContains  string
	}{
		{
			name:         "valid SEP-10 token",
			issuer:       "https://example.com/auth",
			subject:      "GBVFTZL5HIPT4PFQVTZVIWR77V7LWYCXU4CLYWWHHOEXB64XPG5LDMTU",
			jti:          "challenge-123456",
			clientDomain: "wallet.example.com",
			homeDomain:   "example.com",
			iat:          iat,
			exp:          exp,
			wantErr:      false,
		},
		{
			name:         "SEP-10 token without optional fields",
			issuer:       "https://example.com/auth",
			subject:      "GBVFTZL5HIPT4PFQVTZVIWR77V7LWYCXU4CLYWWHHOEXB64XPG5LDMTU",
			jti:          "challenge-123456",
			clientDomain: "",
			homeDomain:   "",
			iat:          iat,
			exp:          exp,
			wantErr:      false,
		},
		{
			name:         "SEP-10 token with memo in subject",
			issuer:       "https://example.com/auth",
			subject:      "GBVFTZL5HIPT4PFQVTZVIWR77V7LWYCXU4CLYWWHHOEXB64XPG5LDMTU:12345",
			jti:          "challenge-123456",
			clientDomain: "wallet.example.com",
			homeDomain:   "example.com",
			iat:          iat,
			exp:          exp,
			wantErr:      false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tokenStr, err := jwtManager.GenerateSEP10Token(
				tc.issuer, tc.subject, tc.jti, tc.clientDomain, tc.homeDomain, tc.iat, tc.exp,
			)

			if tc.wantErr {
				require.Error(t, err)
				if tc.errContains != "" {
					assert.Contains(t, err.Error(), tc.errContains)
				}
				return
			}

			require.NoError(t, err)
			require.NotEmpty(t, tokenStr)

			claims, err := jwtManager.ParseSEP10TokenClaims(tokenStr)
			require.NoError(t, err)
			require.NotNil(t, claims)

			assert.Equal(t, tc.issuer, claims.Issuer)
			assert.Equal(t, tc.subject, claims.Subject)
			assert.Equal(t, tc.jti, claims.ID)
			assert.Equal(t, tc.clientDomain, claims.ClientDomain)
			assert.Equal(t, tc.homeDomain, claims.HomeDomain)
			assert.Equal(t, jwt.NewNumericDate(tc.iat).Unix(), claims.IssuedAt.Unix())
			assert.Equal(t, jwt.NewNumericDate(tc.exp).Unix(), claims.ExpiresAt.Unix())
		})
	}
}

func Test_JWTManager_ParseSEP10TokenClaims_InvalidTokens(t *testing.T) {
	jwtManager, err := NewJWTManager("1234567890ab", 5000)
	require.NoError(t, err)

	// Create a different JWT manager with different secret
	differentJWTManager, err := NewJWTManager("different12345", 5000)
	require.NoError(t, err)

	testCases := []struct {
		name        string
		token       string
		setupToken  func() string
		wantErr     bool
		errContains string
	}{
		{
			name:        "empty token",
			token:       "",
			wantErr:     true,
			errContains: "parsing SEP10 token",
		},
		{
			name:        "invalid token format",
			token:       "not.a.jwt",
			wantErr:     true,
			errContains: "parsing SEP10 token",
		},
		{
			name: "token signed with different secret",
			setupToken: func() string {
				token, _ := differentJWTManager.GenerateSEP10Token(
					"issuer", "subject", "jti", "", "",
					time.Now(), time.Now().Add(5*time.Minute),
				)
				return token
			},
			wantErr:     true,
			errContains: "parsing SEP10 token",
		},
		{
			name: "expired SEP-10 token",
			setupToken: func() string {
				token, _ := jwtManager.GenerateSEP10Token(
					"issuer", "subject", "jti", "", "",
					time.Now().Add(-10*time.Minute), time.Now().Add(-5*time.Minute),
				)
				return token
			},
			wantErr:     true,
			errContains: "parsing SEP10 token",
		},
		{
			name: "SEP-24 token parsed as SEP-10",
			setupToken: func() string {
				token, _ := jwtManager.GenerateSEP24Token(
					"GBVFTZL5HIPT4PFQVTZVIWR77V7LWYCXU4CLYWWHHOEXB64XPG5LDMTU",
					"", "client.com", "home.com", "tx-123",
				)
				return token
			},
			wantErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tokenStr := tc.token
			if tc.setupToken != nil {
				tokenStr = tc.setupToken()
			}

			claims, err := jwtManager.ParseSEP10TokenClaims(tokenStr)

			if tc.wantErr {
				require.Error(t, err)
				if tc.errContains != "" {
					assert.Contains(t, err.Error(), tc.errContains)
				}
				assert.Nil(t, claims)
			} else {
				require.NoError(t, err)
				require.NotNil(t, claims)
			}
		})
	}
}
