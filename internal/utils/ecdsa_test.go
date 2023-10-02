package utils

import (
	"crypto/elliptic"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type ecdsaKeypair struct {
	privateKeyStr string
	publicKeyStr  string
}

var (
	ecdsaKeypair1 = ecdsaKeypair{
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
	ecdsaKeypair2 = ecdsaKeypair{
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
	ec256Keypair = ecdsaKeypair{
		publicKeyStr: `-----BEGIN PUBLIC KEY-----
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEfyKl2tU5lwaQ0l0VJ5vdyW6PoJDb
YNf2uGmNq2Mw64xBqwNfI2iHQFFRUKfvJXdejeCNXZKvkP8XPSzcu0FjOw==
-----END PUBLIC KEY-----`,
		privateKeyStr: `-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIGgkQuWgch6O9Ryw9qsShdHAeIvvJy9X8s/tbiMlbIRqoAoGCCqGSM49
AwEHoUQDQgAEfyKl2tU5lwaQ0l0VJ5vdyW6PoJDbYNf2uGmNq2Mw64xBqwNfI2iH
QFFRUKfvJXdejeCNXZKvkP8XPSzcu0FjOw==
-----END EC PRIVATE KEY-----`,
	}
	ec386Keypair = ecdsaKeypair{
		publicKeyStr: `-----BEGIN PUBLIC KEY-----
MHYwEAYHKoZIzj0CAQYFK4EEACIDYgAETM39j3wuLdKA/FjWvbep5HaRKNI25YZb
AcXGJuvULcaEhM1heR1C8dqEKFiaqJBBwNH0TIiEAEulMhDg/xj8RGwc8OZC5laQ
daNkmorQXHrMkFKIrLX2XaVUsoGazfUB
-----END PUBLIC KEY-----
`,
		privateKeyStr: `-----BEGIN EC PRIVATE KEY-----
MIGkAgEBBDArMin+1alz7nicQ9LGUJgTU/+2v1OQE0G24h+0/V8Sk45sPvRwaxyI
fzZ2qk5WVDagBwYFK4EEACKhZANiAARMzf2PfC4t0oD8WNa9t6nkdpEo0jblhlsB
xcYm69QtxoSEzWF5HULx2oQoWJqokEHA0fRMiIQAS6UyEOD/GPxEbBzw5kLmVpB1
o2SaitBcesyQUoistfZdpVSygZrN9QE=
-----END EC PRIVATE KEY-----`,
	}
)

func Test_ParseStrongECPublicKey(t *testing.T) {
	testCases := []struct {
		name            string
		value           string
		wantCurve       elliptic.Curve
		wantErrContains string
	}{
		{
			name:            "returns an error if the value is not a PEM string",
			value:           "not-a-pem-string",
			wantErrContains: fmt.Sprintf("failed to decode PEM block containing public key: %v", ErrInvalidECPublicKey),
		},
		{
			name:            "returns an error if the value cannot be parsed as a ecdsa.PublicKey",
			value:           "-----BEGIN PUBLIC KEY-----\nYWJjZA==\n-----END PUBLIC KEY-----",
			wantErrContains: fmt.Sprintf("failed to parse EC public key: %v", ErrInvalidECPublicKey),
		},
		{
			name:            "returns an error if the curve is not a valid elliptic curve public key",
			value:           "-----BEGIN PUBLIC KEY-----\nMFwwDQYJKoZIhvcNAQEBBQADSwAwSAJBAMG3KF4Uzd5l/5U6KPYYZA62lrZurmnh\nQ+UptPHvIUgVkQEJwbH+08WXuBiGu1XT00iBtlBSkoZHnB7c04AWFVUCAwEAAQ==\n-----END PUBLIC KEY-----",
			wantErrContains: fmt.Sprintf("not a valid elliptic curve public key: %v", ErrInvalidECPublicKey),
		},
		{
			name:            "returns an error if the curve is weaker than EC256",
			value:           "-----BEGIN PUBLIC KEY-----\nME4wEAYHKoZIzj0CAQYFK4EEACEDOgAEW95JIkzEq9Q9wy2ctSNq2+zj+D0lsepN\n8Ov18JVDuoL1D/1EelRdfdvR70Ss0kfM9frCaXPc7dI=\n-----END PUBLIC KEY-----",
			wantErrContains: fmt.Sprintf("public key curve is not at least as strong as prime256v1: %v", ErrInvalidECPublicKey),
		},
		{
			name:      "🎉 Successfully handles a valid EC256 public key",
			value:     "-----BEGIN PUBLIC KEY-----\nMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEfyKl2tU5lwaQ0l0VJ5vdyW6PoJDb\nYNf2uGmNq2Mw64xBqwNfI2iHQFFRUKfvJXdejeCNXZKvkP8XPSzcu0FjOw==\n-----END PUBLIC KEY-----",
			wantCurve: elliptic.P256(),
		},
		{
			name:      "🎉 Successfully handles a valid EC384 public key",
			value:     "-----BEGIN PUBLIC KEY-----\nMHYwEAYHKoZIzj0CAQYFK4EEACIDYgAETM39j3wuLdKA/FjWvbep5HaRKNI25YZb\nAcXGJuvULcaEhM1heR1C8dqEKFiaqJBBwNH0TIiEAEulMhDg/xj8RGwc8OZC5laQ\ndaNkmorQXHrMkFKIrLX2XaVUsoGazfUB\n-----END PUBLIC KEY-----",
			wantCurve: elliptic.P384(),
		},
		{
			name:      "🎉 Successfully handles a valid EC256 private key in ECDSA format",
			value:     "-----BEGIN PUBLIC KEY-----\nMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAER88h7AiQyVDysRTxKvBB6CaiO/kS\ncvGyimApUE/12gFhNTRf37SE19CSCllKxstnVFOpLLWB7Qu5OJ0Wvcz3hg==\n-----END PUBLIC KEY-----",
			wantCurve: elliptic.P256(),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotPrivateKey, err := ParseStrongECPublicKey(tc.value)
			if tc.wantErrContains == "" {
				require.NoError(t, err)
				require.NotNil(t, gotPrivateKey)
				assert.Equal(t, tc.wantCurve, gotPrivateKey.Curve)
			} else {
				require.Nil(t, gotPrivateKey)
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrContains)
			}
		})
	}
}

func Test_ParseStrongECPrivateKey(t *testing.T) {
	testCases := []struct {
		name            string
		value           string
		wantCurve       elliptic.Curve
		wantErrContains string
	}{
		{
			name:            "returns an error if the value is not a PEM string",
			value:           "not-a-pem-string",
			wantErrContains: fmt.Sprintf("failed to decode PEM block containing private key: %v", ErrInvalidECPrivateKey),
		},
		{
			name:            "returns an error if the value cannot be parsed as a ecdsa.PrivateKey",
			value:           "-----BEGIN EC PRIVATE KEY-----\nMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAyNPqmozv8a2PnXHIkV+F\nmWMFy2YhOFzX12yzjjWkJ3rI9QSEomz4Unkwc6oYrnKEDYlnAgCiCqL2zPr5qNkX\nk5MPU87/wLgEqp7uAk0GkJZfrhJIYZ5AuG9+o69BNeQDEi7F3YdMJj9bvs2Ou1FN\n1zG/8HV969rJ/63fzWsqlNon1j4H5mJ0YbmVh/QLcYPmv7feFZGEj4OSZ4u+eJsw\nat5NPyhMgo6uB/goNS3fEY29UNvXoSIN3hnK3WSxQ79Rjn4V4so7ehxzCVPjnm/G\nFFTgY0hGBobmnxbjI08hEZmYKosjan4YqydGETjKR3UlhBx9y/eqqgL+opNJ8vJs\n2QIDAQAB\n-----END EC PRIVATE KEY-----",
			wantErrContains: fmt.Sprintf("failed to parse EC private key: %v", ErrInvalidECPrivateKey),
		},
		{
			name:            "returns an error if the curve is not a valid elliptic curve private key",
			value:           "-----BEGIN PRIVATE KEY-----\nMIIBUwIBADANBgkqhkiG9w0BAQEFAASCAT0wggE5AgEAAkEAwbcoXhTN3mX/lToo\n9hhkDraWtm6uaeFD5Sm08e8hSBWRAQnBsf7TxZe4GIa7VdPTSIG2UFKShkecHtzT\ngBYVVQIDAQABAkBV9452kgz6kZFnDDR5YkGlNeqUc3H7kviqjmO6qkC+1+zOTxhj\nR3QLOKws7YIPGLzpM4In+cM0XpcEh2EZbn+BAiEA9D5t8X1giqzc6m6JTu5PK6Ue\nsBT0e/XenQ8XiOf29bkCIQDLCh4hhBXA4xnfCvliJ0YE41tRwiVW5/4k9Bxq8Y3q\nfQIgWma1CNoQHqPmzLqHBfj8wrnGBwRqjWsur1FDs7+vz7kCIDajTmBujvwNIRUo\netuy/eCq3hQuTqYIYBfJqSwOPMZxAiBODaEz+TXYJz+nWAHpxtouN8F9cm1hzcRS\nTIyhCXGxSw==\n-----END PRIVATE KEY-----",
			wantErrContains: fmt.Sprintf("not a valid elliptic curve private key: %v", ErrInvalidECPrivateKey),
		},
		{
			name:            "returns an error if the curve is weaker than EC256",
			value:           "-----BEGIN EC PRIVATE KEY-----\nMGgCAQEEHKSQdMBibZ7iVb1gcINiGubmrEn/UhDp6oFfYIWgBwYFK4EEACGhPAM6\nAARb3kkiTMSr1D3DLZy1I2rb7OP4PSWx6k3w6/XwlUO6gvUP/UR6VF1929HvRKzS\nR8z1+sJpc9zt0g==\n-----END EC PRIVATE KEY-----",
			wantErrContains: fmt.Sprintf("private key curve is not at least as strong as prime256v1: %v", ErrInvalidECPrivateKey),
		},
		{
			name:      "🎉 Successfully handles a valid EC256 private key",
			value:     "-----BEGIN EC PRIVATE KEY-----\nMHcCAQEEIGgkQuWgch6O9Ryw9qsShdHAeIvvJy9X8s/tbiMlbIRqoAoGCCqGSM49\nAwEHoUQDQgAEfyKl2tU5lwaQ0l0VJ5vdyW6PoJDbYNf2uGmNq2Mw64xBqwNfI2iH\nQFFRUKfvJXdejeCNXZKvkP8XPSzcu0FjOw==\n-----END EC PRIVATE KEY-----",
			wantCurve: elliptic.P256(),
		},
		{
			name:      "🎉 Successfully handles a valid EC384 private key",
			value:     "-----BEGIN EC PRIVATE KEY-----\nMIGkAgEBBDArMin+1alz7nicQ9LGUJgTU/+2v1OQE0G24h+0/V8Sk45sPvRwaxyI\nfzZ2qk5WVDagBwYFK4EEACKhZANiAARMzf2PfC4t0oD8WNa9t6nkdpEo0jblhlsB\nxcYm69QtxoSEzWF5HULx2oQoWJqokEHA0fRMiIQAS6UyEOD/GPxEbBzw5kLmVpB1\no2SaitBcesyQUoistfZdpVSygZrN9QE=\n-----END EC PRIVATE KEY-----",
			wantCurve: elliptic.P384(),
		},
		{
			name:      "🎉 Successfully handles a valid EC256 private key in ECDSA format",
			value:     "-----BEGIN PRIVATE KEY-----\nMIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQgIqI1MzMZIw2pQDLx\nJn0+FcNT/hNjwtn2TW43710JKZqhRANCAARHzyHsCJDJUPKxFPEq8EHoJqI7+RJy\n8bKKYClQT/XaAWE1NF/ftITX0JIKWUrGy2dUU6kstYHtC7k4nRa9zPeG\n-----END PRIVATE KEY-----",
			wantCurve: elliptic.P256(),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotPrivateKey, err := ParseStrongECPrivateKey(tc.value)
			if tc.wantErrContains == "" {
				require.NoError(t, err)
				require.NotNil(t, gotPrivateKey)
				assert.Equal(t, tc.wantCurve, gotPrivateKey.Curve)
			} else {
				require.Nil(t, gotPrivateKey)
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrContains)
			}
		})
	}
}

func Test_ValidateStrongECKeyPair(t *testing.T) {
	testCases := []struct {
		name            string
		publicKeyStr    string
		privateKeyStr   string
		wantErrContains string
	}{
		{
			name:            "returns an error if the public key is invalid",
			publicKeyStr:    "not-a-pem-string",
			privateKeyStr:   ecdsaKeypair1.privateKeyStr,
			wantErrContains: fmt.Sprintf("validating EC public key: failed to decode PEM block containing public key: %v", ErrInvalidECPublicKey),
		},
		{
			name:            "returns an error if the private key is invalid",
			publicKeyStr:    ecdsaKeypair1.publicKeyStr,
			privateKeyStr:   "-----BEGIN MY STRING-----\nYWJjZA==\n-----END MY STRING-----",
			wantErrContains: fmt.Sprintf("validating EC private key: failed to parse EC private key: %v", ErrInvalidECPrivateKey),
		},
		{
			name:            "returns an error if the keys are not a pair (1 & 2)",
			publicKeyStr:    ecdsaKeypair1.publicKeyStr,
			privateKeyStr:   ecdsaKeypair2.privateKeyStr,
			wantErrContains: "signature verification failed for the provided pair of keys",
		},
		{
			name:            "returns an error if the keys are not a pair (2 & 1)",
			publicKeyStr:    ecdsaKeypair2.publicKeyStr,
			privateKeyStr:   ecdsaKeypair1.privateKeyStr,
			wantErrContains: "signature verification failed for the provided pair of keys",
		},
		{
			name:          "🎉 Successfully validates a valid ECDSA key pair (1)",
			publicKeyStr:  ecdsaKeypair1.publicKeyStr,
			privateKeyStr: ecdsaKeypair1.privateKeyStr,
		},
		{
			name:          "🎉 Successfully validates a valid ECDSA key pair (2)",
			publicKeyStr:  ecdsaKeypair2.publicKeyStr,
			privateKeyStr: ecdsaKeypair2.privateKeyStr,
		},
		{
			name:          "🎉 Successfully validates a valid EC256 key pair",
			publicKeyStr:  ec256Keypair.publicKeyStr,
			privateKeyStr: ec256Keypair.privateKeyStr,
		},
		{
			name:          "🎉 Successfully validates a valid EC386 key pair",
			publicKeyStr:  ec386Keypair.publicKeyStr,
			privateKeyStr: ec386Keypair.privateKeyStr,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateStrongECKeyPair(tc.publicKeyStr, tc.privateKeyStr)
			if tc.wantErrContains == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.wantErrContains)
			}
		})
	}
}
