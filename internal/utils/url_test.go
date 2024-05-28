package utils

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stellar/go/keypair"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_SignURL(t *testing.T) {
	// rawURL := 	https://vibrantapp.com/sdp-dev?domain=ap-stellar-disbursement-platform-backend-dev.stellar.org&name=Stellar%20Test&asset=USDC-GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5
	// PublicKey:  	GBFDUUZ5ZYC6RAPOQLM7IYXLFHYTMCYXBGM7NIC4EE2MWOSGIYCOSN5F
	// PrivateKey: 	SBUSPEKAZKLZSWHRSJ2HWDZUK6I3IVDUWA7JJZSGBLZ2WZIUJI7FPNB5
	// result: 		https://vibrantapp.com/sdp-dev?asset=USDC-GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5&domain=ap-stellar-disbursement-platform-backend-dev.stellar.org&name=Stellar+Test&signature=60bb8ed15df271131bb2d7c87fd5649a9a69bf655c5ffcff3816c766cfd98356381a7d4c03494c4bb9eb25e1167a399845aae73ec667990d840e9fc43af6e906

	testCases := []struct {
		name             string
		stellarSecretKey string
		rawURL           string
		wantSignedURL    string
		wantErrContains  string
	}{
		{
			name:            "returns an error if stellarSecretKey is empty",
			wantErrContains: "error parsing stellar private key: strkey is 0 bytes long; minimum valid length is 5",
		},
		{
			name:             "returns an error if stellarSecretKey is invalid",
			stellarSecretKey: "INVALID_SECRET_KEY",
			wantErrContains:  "error parsing stellar private key: base32 decode failed: illegal base32 data at input byte 7",
		},
		{
			name:             "returns an error if rawURL is empty",
			stellarSecretKey: "SBUSPEKAZKLZSWHRSJ2HWDZUK6I3IVDUWA7JJZSGBLZ2WZIUJI7FPNB5",
			wantErrContains:  `raw url "" should have both a scheme and a host`,
		},
		{
			name:             "returns an error if rawURL has a host without scheme",
			stellarSecretKey: "SBUSPEKAZKLZSWHRSJ2HWDZUK6I3IVDUWA7JJZSGBLZ2WZIUJI7FPNB5",
			rawURL:           "host-without-scheme",
			wantErrContains:  `raw url "host-without-scheme" should have both a scheme and a host`,
		},
		{
			name:             "returns an error if rawURL has a scheme without host",
			stellarSecretKey: "SBUSPEKAZKLZSWHRSJ2HWDZUK6I3IVDUWA7JJZSGBLZ2WZIUJI7FPNB5",
			rawURL:           "scheme-without-host://",
			wantErrContains:  `raw url "scheme-without-host://" should have both a scheme and a host`,
		},
		{
			name:             "🎉 successfully signs the desired url",
			stellarSecretKey: "SBUSPEKAZKLZSWHRSJ2HWDZUK6I3IVDUWA7JJZSGBLZ2WZIUJI7FPNB5",
			rawURL:           "https://vibrantapp.com/sdp-dev?domain=ap-stellar-disbursement-platform-backend-dev.stellar.org&name=Stellar%20Test&asset=USDC-GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
			wantSignedURL:    "https://vibrantapp.com/sdp-dev?asset=USDC-GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5&domain=ap-stellar-disbursement-platform-backend-dev.stellar.org&name=Stellar+Test&signature=fea6c5e805a29b903835bea2f6c60069113effdf1c5cb448d4948573c65557b1d667bcd176c24a94ed9d54a1829317c74f39319076511512a3e697b4b746ae0a",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotSignedURL, err := SignURL(tc.stellarSecretKey, tc.rawURL)
			if tc.wantErrContains != "" {
				assert.Empty(t, gotSignedURL)
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrContains)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.wantSignedURL, gotSignedURL)
			}
		})
	}
}

func Test_VerifySignedURL(t *testing.T) {
	// signedURL example from previous test
	signedURL := "https://vibrantapp.com/sdp-dev/aid?asset=USDC-GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5&domain=ap-stellar-disbursement-platform-backend-dev.stellar.org&name=Stellar+Test&signature=60bb8ed15df271131bb2d7c87fd5649a9a69bf655c5ffcff3816c766cfd98356381a7d4c03494c4bb9eb25e1167a399845aae73ec667990d840e9fc43af6e906"
	expectedPublicKey := "GBFDUUZ5ZYC6RAPOQLM7IYXLFHYTMCYXBGM7NIC4EE2MWOSGIYCOSN5F"

	// expectedPublicKey cannot be empty
	isValid, err := VerifySignedURL(signedURL, "")
	require.False(t, isValid)
	require.EqualError(t, err, "error parsing expected public key: strkey is 0 bytes long; minimum valid length is 5")

	// invalid expectedPublicKey
	isValid, err = VerifySignedURL(signedURL, "INVALID_PUBLIC_KEY")
	require.False(t, isValid)
	require.EqualError(t, err, "error parsing expected public key: base32 decode failed: illegal base32 data at input byte 7")

	// signedURL cannot be empty
	isValid, err = VerifySignedURL("", expectedPublicKey)
	require.False(t, isValid)
	require.EqualError(t, err, "signed url does not contain a signature")

	// invalid signedURL
	isValid, err = VerifySignedURL("invalid_signed_url", expectedPublicKey)
	require.False(t, isValid)
	require.EqualError(t, err, "signed url does not contain a signature")

	// valid signedURL and expectedPublicKey 🎉
	isValid, err = VerifySignedURL(signedURL, expectedPublicKey)
	require.NoError(t, err)
	require.True(t, isValid)

	// valid signedURL and expectedPublicKey but signature is invalid
	tamperedURL := strings.Replace(signedURL, "USDC", "USD", 1)
	isValid, err = VerifySignedURL(tamperedURL, expectedPublicKey)
	require.False(t, isValid)
	require.EqualError(t, err, "error verifying URL signature: signature verification failed")
}

func Test_SignURL_VerifySignedURL(t *testing.T) {
	kp, err := keypair.Random()
	require.NoError(t, err)

	// valid rawURL and stellarSecretKey 🎉
	validURL := "https://vibrantapp.com/sdp-dev/aid?domain=ap-stellar-disbursement-platform-backend-dev.stellar.org&name=Stellar%20Test&asset=USDC-GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5"
	gotSignedURL, err := SignURL(kp.Seed(), validURL)
	require.NoError(t, err)
	require.NotEmpty(t, gotSignedURL)

	// valid signedURL and expectedPublicKey 🎉
	isValid, err := VerifySignedURL(gotSignedURL, kp.Address())
	require.NoError(t, err)
	require.True(t, isValid)
}

func Test_GetURLWithScheme(t *testing.T) {
	testCases := []struct {
		name, rawURL, expectedURL string
		expectedErr               error
	}{
		{
			name:        "returns the same URL when it has URL scheme",
			rawURL:      "http://bluecorp.org.local",
			expectedURL: "http://bluecorp.org.local",
			expectedErr: nil,
		},
		{
			name:        "sets the URL scheme successfully",
			rawURL:      "bluecorp.org.local",
			expectedURL: "http://bluecorp.org.local",
			expectedErr: nil,
		},
		{
			name:        "sets the URL scheme successfully when it has port",
			rawURL:      "bluecorp.org.local:3000",
			expectedURL: "http://bluecorp.org.local:3000",
			expectedErr: nil,
		},
		{
			name:        "returns the same URL when it has URL scheme and port",
			rawURL:      "http://bluecorp.org.local:3000",
			expectedURL: "http://bluecorp.org.local:3000",
			expectedErr: nil,
		},
		{
			name:        "returns error when missing protocol scheme",
			rawURL:      "://bluecorp.org.local:3000",
			expectedURL: "",
			expectedErr: fmt.Errorf(`parsing url: parse "://bluecorp.org.local:3000": missing protocol scheme`),
		},
		{
			name:        "returns correct URL when it starts with //",
			rawURL:      "//bluecorp.org.local:3000",
			expectedURL: "http://bluecorp.org.local:3000",
			expectedErr: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotURL, err := GetURLWithScheme(tc.rawURL)
			if tc.expectedErr != nil {
				assert.EqualError(t, err, tc.expectedErr.Error())
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.expectedURL, gotURL)
		})
	}
}

func Test_GenerateTenantURL(t *testing.T) {
	tenantID := "abc"
	testCases := []struct {
		name, baseURL, tenantID, expectedURL string
		expectedErr                          error
	}{
		{
			name:        "returns the correct tenant URL",
			baseURL:     "http://bluecorp.org.local",
			tenantID:    tenantID,
			expectedURL: fmt.Sprintf("http://%s.bluecorp.org.local", tenantID),
			expectedErr: nil,
		},
		{
			name:        "returns the correct tenant URL - varying protocol",
			baseURL:     "https://bluecorp.org.local",
			tenantID:    tenantID,
			expectedURL: fmt.Sprintf("https://%s.bluecorp.org.local", tenantID),
			expectedErr: nil,
		},
		{
			name:        "returns the correct tenant URL when it has port",
			baseURL:     "http://bluecorp.org.local:3000",
			tenantID:    tenantID,
			expectedURL: fmt.Sprintf("http://%s.bluecorp.org.local:3000", tenantID),
			expectedErr: nil,
		},
		{
			name:        "returns the correct tenant URL when it has a path",
			baseURL:     "http://bluecorp.org.local/sdp",
			tenantID:    tenantID,
			expectedURL: fmt.Sprintf("http://%s.bluecorp.org.local/sdp", tenantID),
			expectedErr: nil,
		},
		{
			name:        "returns error when tenant ID is empty",
			baseURL:     "http://bluecorp.org.local/sdp",
			expectedURL: "",
			expectedErr: fmt.Errorf("tenantID is empty"),
		},
		{
			name:        "returns error when invalid base URL - no protocol and URL separator",
			baseURL:     "bluecorp.org.local:3000",
			tenantID:    tenantID,
			expectedURL: "",
			expectedErr: fmt.Errorf("base URL must have at least two domain parts bluecorp.org.local:3000"),
		},
		{
			name:        "returns error when invalid base URL - no protocol",
			baseURL:     "://bluecorp.org.local:3000",
			tenantID:    tenantID,
			expectedURL: "",
			expectedErr: fmt.Errorf("invalid base URL ://bluecorp.org.local:3000: parse \"://bluecorp.org.local:3000\": missing protocol scheme"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotURL, err := GenerateTenantURL(tc.baseURL, tc.tenantID)
			if tc.expectedErr != nil {
				assert.EqualError(t, err, tc.expectedErr.Error())
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.expectedURL, gotURL)
		})
	}
}
