package utils

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type ecdsaKeypair struct {
	privateKeyStr string
	publicKeyStr  string
}

var (
	keypair1 = ecdsaKeypair{
		publicKeyStr: `-----BEGIN PUBLIC KEY-----
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAER88h7AiQyVDysRTxKvBB6CaiO/kS
cvGyimApUE/12gFhNTRf37SE19CSCllKxstnVFOpLLWB7Qu5OJ0Wvcz3hg==
-----END PUBLIC KEY-----`,
		privateKeyStr: `-----BEGIN PRIVATE KEY-----
MIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQgIqI1MzMZIw2pQDLx
Jn0+FcNT/hNjwtn2TW43710JKZqhRANCAARHzyHsCJDJUPKxFPEq8EHoJqI7+RJy
8bKKYClQT/XaAWE1NF/ftITX0JIKWUrGy2dUU6kstYHtC7k4nRa9zPeG
-----END PRIVATE KEY-----`,
	}
	keypair2 = ecdsaKeypair{
		publicKeyStr: `-----BEGIN PUBLIC KEY-----
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAERJtGEWVxHTOghAFU9XyANbF10aXK
zT3U72jUfBk38fceemINJERxdLbBs2O1foeFd8HyJ6Zn7tLvZWGNvVN+cA==
-----END PUBLIC KEY-----`,
		privateKeyStr: `-----BEGIN PRIVATE KEY-----
MIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQgw8lMqTKWEdxusLOW
J16L7THmguSKZq1PPS1SRravKpOhRANCAAREm0YRZXEdM6CEAVT1fIA1sXXRpcrN
PdTvaNR8GTfx9x56Yg0kRHF0tsGzY7V+h4V3wfInpmfu0u9lYY29U35w
-----END PRIVATE KEY-----`,
	}
)

func Test_ParseECDSAPublicKey(t *testing.T) {
	// publicKeyObj is the public key object that corresponds to the keypair1.publicKeyStr
	bigIntX := new(big.Int)
	bigIntX.SetString("32480183712899956666963574445105818726761898573293978186307012095310684346881", 10)
	bigIntY := new(big.Int)
	bigIntY.SetString("43968350682573962747988640660801043718476300246351425025163140929681875597190", 10)
	publicKeyObj := &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     bigIntX,
		Y:     bigIntY,
	}

	testCases := []struct {
		name            string
		value           string
		wantResult      *ecdsa.PublicKey
		wantErrContains string
	}{
		{
			name:            "returns an error if the value is not a PEM string",
			value:           "not-a-pem-string",
			wantErrContains: "failed to decode PEM block containing public key",
		},
		{
			name:            "returns an error if the value is not a x509 string",
			value:           "-----BEGIN MY STRING-----\nYWJjZA==\n-----END MY STRING-----",
			wantErrContains: "failed to parse x509 PKIX public key",
		},
		{
			name:            "returns an error if the value is not a ECDSA public key",
			value:           "-----BEGIN PUBLIC KEY-----\nMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAyNPqmozv8a2PnXHIkV+F\nmWMFy2YhOFzX12yzjjWkJ3rI9QSEomz4Unkwc6oYrnKEDYlnAgCiCqL2zPr5qNkX\nk5MPU87/wLgEqp7uAk0GkJZfrhJIYZ5AuG9+o69BNeQDEi7F3YdMJj9bvs2Ou1FN\n1zG/8HV969rJ/63fzWsqlNon1j4H5mJ0YbmVh/QLcYPmv7feFZGEj4OSZ4u+eJsw\nat5NPyhMgo6uB/goNS3fEY29UNvXoSIN3hnK3WSxQ79Rjn4V4so7ehxzCVPjnm/G\nFFTgY0hGBobmnxbjI08hEZmYKosjan4YqydGETjKR3UlhBx9y/eqqgL+opNJ8vJs\n2QIDAQAB\n-----END PUBLIC KEY-----",
			wantErrContains: "public key is not of type ECDSA",
		},
		{
			name:       "🎉 Successfully handles a valid ECDSA public key",
			value:      keypair1.publicKeyStr,
			wantResult: publicKeyObj,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotPublicKey, err := ParseECDSAPublicKey(tc.value)
			if tc.wantErrContains == "" {
				assert.NotNil(t, gotPublicKey)
				assert.Equal(t, tc.wantResult, gotPublicKey)
				assert.NoError(t, err)
			} else {
				assert.Nil(t, gotPublicKey)
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrContains)
			}
		})
	}
}

func Test_ParseECDSAPrivateKey(t *testing.T) {
	// privateKeyObj is the public key object that corresponds to the keypair1.privateKeyStr
	bigIntX := new(big.Int)
	bigIntX.SetString("32480183712899956666963574445105818726761898573293978186307012095310684346881", 10)
	bigIntY := new(big.Int)
	bigIntY.SetString("43968350682573962747988640660801043718476300246351425025163140929681875597190", 10)
	publicKeyObj := &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     bigIntX,
		Y:     bigIntY,
	}
	bigIntD := new(big.Int)
	bigIntD.SetString("15665233249220082997812441880036381661021061746430729869708887737553839008154", 10)
	privateKeyObj := &ecdsa.PrivateKey{
		PublicKey: *publicKeyObj,
		D:         bigIntD,
	}

	testCases := []struct {
		name            string
		value           string
		wantResult      *ecdsa.PrivateKey
		wantErrContains string
	}{
		{
			name:            "returns an error if the value is not a PEM string",
			value:           "not-a-pem-string",
			wantErrContains: "failed to decode PEM block containing private key",
		},
		{
			name:            "returns an error if the value is not a x509 string",
			value:           "-----BEGIN MY STRING-----\nYWJjZA==\n-----END MY STRING-----",
			wantErrContains: "failed to parse EC private key",
		},
		{
			name:            "returns an error if the value is not a ECDSA private key",
			value:           "-----BEGIN PUBLIC KEY-----\nMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAyNPqmozv8a2PnXHIkV+F\nmWMFy2YhOFzX12yzjjWkJ3rI9QSEomz4Unkwc6oYrnKEDYlnAgCiCqL2zPr5qNkX\nk5MPU87/wLgEqp7uAk0GkJZfrhJIYZ5AuG9+o69BNeQDEi7F3YdMJj9bvs2Ou1FN\n1zG/8HV969rJ/63fzWsqlNon1j4H5mJ0YbmVh/QLcYPmv7feFZGEj4OSZ4u+eJsw\nat5NPyhMgo6uB/goNS3fEY29UNvXoSIN3hnK3WSxQ79Rjn4V4so7ehxzCVPjnm/G\nFFTgY0hGBobmnxbjI08hEZmYKosjan4YqydGETjKR3UlhBx9y/eqqgL+opNJ8vJs\n2QIDAQAB\n-----END PUBLIC KEY-----",
			wantErrContains: "failed to parse EC private key",
		},
		{
			name:       "🎉 Successfully handles a valid ECDSA private key",
			wantResult: privateKeyObj,
			value:      keypair1.privateKeyStr,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotPrivateKey, err := ParseECDSAPrivateKey(tc.value)
			if tc.wantErrContains == "" {
				assert.Equal(t, tc.wantResult, gotPrivateKey)
				assert.NoError(t, err)
			} else {
				assert.Nil(t, gotPrivateKey)
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrContains)
			}
		})
	}
}

func Test_ValidateECDSAKeys(t *testing.T) {
	testCases := []struct {
		name            string
		publicKeyStr    string
		privateKeyStr   string
		wantErrContains string
	}{
		{
			name:            "returns an error if the public key is not a PEM string",
			publicKeyStr:    "not-a-pem-string",
			privateKeyStr:   keypair1.privateKeyStr,
			wantErrContains: "validating ECDSA public key: failed to decode PEM block containing public key",
		},
		{
			name:            "returns an error if the public key is valid but the private key is not a x509 string",
			publicKeyStr:    keypair1.publicKeyStr,
			privateKeyStr:   "-----BEGIN MY STRING-----\nYWJjZA==\n-----END MY STRING-----",
			wantErrContains: "validating ECDSA private key: failed to parse EC private key",
		},
		{
			name:            "returns an error if the keys are not a pair (1 & 2)",
			publicKeyStr:    keypair1.publicKeyStr,
			privateKeyStr:   keypair2.privateKeyStr,
			wantErrContains: "signature verification failed",
		},
		{
			name:            "returns an error if the keys are not a pair (2 & 1)",
			publicKeyStr:    keypair2.publicKeyStr,
			privateKeyStr:   keypair1.privateKeyStr,
			wantErrContains: "signature verification failed",
		},
		{
			name:          "🎉 Successfully validates a valid ECDSA key pair (1)",
			publicKeyStr:  keypair1.publicKeyStr,
			privateKeyStr: keypair1.privateKeyStr,
		},
		{
			name:          "🎉 Successfully validates a valid ECDSA key pair (2)",
			publicKeyStr:  keypair2.publicKeyStr,
			privateKeyStr: keypair2.privateKeyStr,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateECDSAKeys(tc.publicKeyStr, tc.privateKeyStr)
			if tc.wantErrContains == "" {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrContains)
			}
		})
	}
}
