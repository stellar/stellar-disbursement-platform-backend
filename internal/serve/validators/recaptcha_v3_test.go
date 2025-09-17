package validators

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type mockHTTPClient struct {
	response *http.Response
	err      error
}

func (m *mockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return m.response, m.err
}

func TestGoogleReCAPTCHAV3Validator_IsTokenValid(t *testing.T) {
	tests := []struct {
		name           string
		token          string
		mockResponse   string
		mockStatusCode int
		mockError      error
		minScore       float64
		wantValid      bool
		wantErr        bool
	}{
		{
			name:           "valid token with high score",
			token:          "valid-token",
			mockResponse:   `{"success": true, "score": 0.9, "action": "submit", "challenge_ts": "2023-01-01T00:00:00Z", "hostname": "localhost"}`,
			mockStatusCode: 200,
			minScore:       0.5,
			wantValid:      true,
			wantErr:        false,
		},
		{
			name:           "valid token with low score",
			token:          "low-score-token",
			mockResponse:   `{"success": true, "score": 0.3, "action": "submit", "challenge_ts": "2023-01-01T00:00:00Z", "hostname": "localhost"}`,
			mockStatusCode: 200,
			minScore:       0.5,
			wantValid:      false,
			wantErr:        true,
		},
		{
			name:           "invalid token",
			token:          "invalid-token",
			mockResponse:   `{"success": false, "error-codes": ["invalid-input-response"]}`,
			mockStatusCode: 200,
			minScore:       0.5,
			wantValid:      false,
			wantErr:        true,
		},
		{
			name:           "timeout error",
			token:          "timeout-token",
			mockResponse:   `{"success": false, "error-codes": ["timeout-or-duplicate"]}`,
			mockStatusCode: 200,
			minScore:       0.5,
			wantValid:      false,
			wantErr:        false,
		},
		{
			name:           "network error",
			token:          "network-error-token",
			mockResponse:   "",
			mockStatusCode: 0,
			mockError:      errors.New("network error"),
			minScore:       0.5,
			wantValid:      false,
			wantErr:        true,
		},
		{
			name:           "invalid JSON response",
			token:          "invalid-json-token",
			mockResponse:   `invalid json`,
			mockStatusCode: 200,
			minScore:       0.5,
			wantValid:      false,
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mockClient *mockHTTPClient
			if tt.mockError != nil {
				mockClient = &mockHTTPClient{err: tt.mockError}
			} else {
				mockClient = &mockHTTPClient{
					response: &http.Response{
						StatusCode: tt.mockStatusCode,
						Body:       io.NopCloser(strings.NewReader(tt.mockResponse)),
					},
				}
			}

			validator := NewGoogleReCAPTCHAV3Validator("test-secret", tt.minScore, mockClient)

			valid, err := validator.IsTokenValid(context.Background(), tt.token)

			if tt.wantErr {
				if err == nil {
					t.Errorf("IsTokenValid() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("IsTokenValid() unexpected error: %v", err)
				return
			}

			if valid != tt.wantValid {
				t.Errorf("IsTokenValid() = %v, want %v", valid, tt.wantValid)
			}
		})
	}
}

func TestNewGoogleReCAPTCHAV3Validator(t *testing.T) {
	tests := []struct {
		name     string
		minScore float64
		want     float64
	}{
		{
			name:     "valid min score",
			minScore: 0.7,
			want:     0.7,
		},
		{
			name:     "zero min score - should use default",
			minScore: 0,
			want:     defaultMinScore,
		},
		{
			name:     "negative min score - should use default",
			minScore: -0.1,
			want:     defaultMinScore,
		},
		{
			name:     "score above 1.0 - should use default",
			minScore: 1.5,
			want:     defaultMinScore,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewGoogleReCAPTCHAV3Validator("test-secret", tt.minScore, &mockHTTPClient{})

			if validator.MinScore != tt.want {
				t.Errorf("NewGoogleReCAPTCHAV3Validator() MinScore = %v, want %v", validator.MinScore, tt.want)
			}
		})
	}
}
