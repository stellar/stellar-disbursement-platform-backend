package validators

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/mock"

	httpclientMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpclient/mocks"
)

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
			httpClientMock := httpclientMocks.NewHTTPClientMock(t)

			if tt.mockError != nil {
				httpClientMock.On("Do", mock.AnythingOfType("*http.Request")).Return(nil, tt.mockError)
			} else {
				httpClientMock.On("Do", mock.AnythingOfType("*http.Request")).Return(&http.Response{
					StatusCode: tt.mockStatusCode,
					Body:       io.NopCloser(strings.NewReader(tt.mockResponse)),
				}, nil)
			}

			validator := NewGoogleReCAPTCHAV3Validator("test-secret", tt.minScore, httpClientMock)

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
			httpClientMock := httpclientMocks.NewHTTPClientMock(t)
			validator := NewGoogleReCAPTCHAV3Validator("test-secret", tt.minScore, httpClientMock)

			if validator.MinScore != tt.want {
				t.Errorf("NewGoogleReCAPTCHAV3Validator() MinScore = %v, want %v", validator.MinScore, tt.want)
			}
		})
	}
}
