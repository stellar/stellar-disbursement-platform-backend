package events

import "testing"

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
			name: "Unsupported SASL_SSL",
			config: KafkaConfig{
				Brokers:          []string{"localhost:9092"},
				SecurityProtocol: KafkaProtocolSASLSSL,
				SASLUsername:     "user",
				SASLPassword:     "pass",
			},
			wantErr: true,
			errMsg:  "security protocols SASL_SSL and SSL are not yet supported",
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
