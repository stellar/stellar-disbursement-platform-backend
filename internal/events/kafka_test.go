package events

import "testing"

// testAccessKeyPEM is testing Private Key PEM. DO NOT USE THIS IN PRODUCTION!
// Please, refer to: https://github.com/golang/go/blob/master/src/crypto/tls/tls_test.go#L54-L63
const testAccessKeyPEM = `-----BEGIN PRIVATE KEY-----
MIIBOwIBAAJBANLJhPHhITqQbPklG3ibCVxwGMRfp/v4XqhfdQHdcVfHap6NQ5Wo
k/4xIA+ui35/MmNartNuC+BdZ1tMuVCPFZcCAwEAAQJAEJ2N+zsR0Xn8/Q6twa4G
6OB1M1WO+k+ztnX/1SvNeWu8D6GImtupLTYgjZcHufykj09jiHmjHx8u8ZZB/o1N
MQIhAPW+eyZo7ay3lMz1V01WVjNKK9QSn1MJlb06h/LuYv9FAiEA25WPedKgVyCW
SmUwbPw8fnTcpqDWE3yTO3vKcebqMSsCIBF3UmVue8YU3jybC3NxuXq3wNm34R8T
xVLHwDXh/6NJAiEAl2oHGGLz64BuAfjKrqwz7qMYr9HCLIe/YsoWq/olzScCIQDi
D2lWusoe2/nEqfDVVWGWlyJ7yOmqaVm/iNUN9B2N2g==
-----END PRIVATE KEY-----
`

// testAccessCertificatePEM is testing Certificate that matches with the test Private Key. DO NOT USE THIS IN PRODUCTION!
// Please, refer to: https://github.com/golang/go/blob/master/src/crypto/tls/tls_test.go#L27-L39
const testAccessCertificatePEM = `-----BEGIN CERTIFICATE-----
MIIB0zCCAX2gAwIBAgIJAI/M7BYjwB+uMA0GCSqGSIb3DQEBBQUAMEUxCzAJBgNV
BAYTAkFVMRMwEQYDVQQIDApTb21lLVN0YXRlMSEwHwYDVQQKDBhJbnRlcm5ldCBX
aWRnaXRzIFB0eSBMdGQwHhcNMTIwOTEyMjE1MjAyWhcNMTUwOTEyMjE1MjAyWjBF
MQswCQYDVQQGEwJBVTETMBEGA1UECAwKU29tZS1TdGF0ZTEhMB8GA1UECgwYSW50
ZXJuZXQgV2lkZ2l0cyBQdHkgTHRkMFwwDQYJKoZIhvcNAQEBBQADSwAwSAJBANLJ
hPHhITqQbPklG3ibCVxwGMRfp/v4XqhfdQHdcVfHap6NQ5Wok/4xIA+ui35/MmNa
rtNuC+BdZ1tMuVCPFZcCAwEAAaNQME4wHQYDVR0OBBYEFJvKs8RfJaXTH08W+SGv
zQyKn0H8MB8GA1UdIwQYMBaAFJvKs8RfJaXTH08W+SGvzQyKn0H8MAwGA1UdEwQF
MAMBAf8wDQYJKoZIhvcNAQEFBQADQQBJlffJHybjDGxRMqaRmDhX0+6v02TUKZsW
r5QuVbpQhH6u+0UgcW0jp9QwpxoPTLTWGXEWBBBurxFwiCBhkQ+V
-----END CERTIFICATE-----
`

func Test_ParseKafkaSecurityProtocol(t *testing.T) {
	tests := []struct {
		name        string
		protocol    string
		want        KafkaSecurityProtocol
		expectError bool
	}{
		{
			name:        "PLAINTEXT protocol",
			protocol:    "PLAINTEXT",
			want:        KafkaProtocolPlaintext,
			expectError: false,
		},
		{
			name:        "SASL_PLAINTEXT protocol",
			protocol:    "SASL_PLAINTEXT",
			want:        KafkaProtocolSASLPlaintext,
			expectError: false,
		},
		{
			name:        "SASL_SSL protocol",
			protocol:    "SASL_SSL",
			want:        KafkaProtocolSASLSSL,
			expectError: false,
		},
		{
			name:        "SSL protocol",
			protocol:    "SSL",
			want:        KafkaProtocolSSL,
			expectError: false,
		},
		{
			name:        "Invalid protocol",
			protocol:    "INVALID",
			want:        "",
			expectError: true,
		},
		{
			name:        "Case insensitivity",
			protocol:    "plaintext",
			want:        KafkaProtocolPlaintext,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseKafkaSecurityProtocol(tt.protocol)
			if (err != nil) != tt.expectError {
				t.Errorf("ParseKafkaSecurityProtocol() error = %v, expectError %v", err, tt.expectError)
				return
			}
			if got != tt.want {
				t.Errorf("ParseKafkaSecurityProtocol() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_KafkaConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  KafkaConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "Valid config with PLAINTEXT",
			config: KafkaConfig{
				Brokers:          []string{"localhost:9092"},
				SecurityProtocol: KafkaProtocolPlaintext,
			},
			wantErr: false,
		},
		{
			name: "Valid config with SASL_PLAINTEXT",
			config: KafkaConfig{
				Brokers:          []string{"localhost:9092"},
				SecurityProtocol: KafkaProtocolSASLPlaintext,
				SASLUsername:     "user",
				SASLPassword:     "pass",
			},
			wantErr: false,
		},
		{
			name: "Empty brokers",
			config: KafkaConfig{
				SecurityProtocol: KafkaProtocolPlaintext,
			},
			wantErr: true,
			errMsg:  "brokers cannot be empty",
		},
		{
			name: "Empty security protocol",
			config: KafkaConfig{
				Brokers: []string{"localhost:9092"},
			},
			wantErr: true,
			errMsg:  "security protocol cannot be empty",
		},
		{
			name: "SASL_PLAINTEXT without credentials",
			config: KafkaConfig{
				Brokers:          []string{"localhost:9092"},
				SecurityProtocol: KafkaProtocolSASLPlaintext,
			},
			wantErr: true,
			errMsg:  "SASL credentials must be provided for SASL_PLAINTEXT and SASL_SSL protocols",
		},
		{
			name: "Valid config with SASL_SSL",
			config: KafkaConfig{
				Brokers:          []string{"localhost:9092"},
				SecurityProtocol: KafkaProtocolSASLSSL,
				SASLUsername:     "user",
				SASLPassword:     "pass",
			},
			wantErr: false,
		},
		{
			name: "Valid config with SSL",
			config: KafkaConfig{
				Brokers:              []string{"localhost:9092"},
				SecurityProtocol:     KafkaProtocolSSL,
				SSLAccessKey:         testAccessKeyPEM,
				SSLAccessCertificate: testAccessCertificatePEM,
			},
			wantErr: false,
		},
		{
			name: "SSL without the Access Key/Certificate",
			config: KafkaConfig{
				Brokers:              []string{"localhost:9092"},
				SecurityProtocol:     KafkaProtocolSSL,
				SSLAccessKey:         "",
				SSLAccessCertificate: "",
			},
			wantErr: true,
			errMsg:  "validating Kafka SSL Access Key/Certificate: tls: failed to find any PEM data in certificate input",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("KafkaConfig.Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err != nil && tt.errMsg != "" && err.Error() != tt.errMsg {
				t.Errorf("KafkaConfig.Validate() error = %v, wantErrMsg %v", err.Error(), tt.errMsg)
			}
		})
	}
}
