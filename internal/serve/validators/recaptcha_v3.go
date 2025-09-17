package validators

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
)

const (
	recaptchaV3VerifyURL = "https://www.google.com/recaptcha/api/siteverify"
	// Default minimum score for reCAPTCHA v3 (0.0 to 1.0, where 1.0 is very likely a good interaction)
	defaultMinScore = 0.5
)

type GoogleReCAPTCHAV3Validator struct {
	SiteSecretKey  string
	VerifyTokenURL string
	MinScore       float64
	HTTPClient     HTTPClient
}

type reCAPTCHAV3VerifyResponse struct {
	Success     bool     `json:"success"`
	Score       float64  `json:"score"`
	Action      string   `json:"action"`
	ChallengeTS string   `json:"challenge_ts"`
	Hostname    string   `json:"hostname"`
	ErrorCodes  []string `json:"error-codes"`
}

// IsTokenValid validates a reCAPTCHA v3 token and checks if the score meets the minimum threshold.
func (v *GoogleReCAPTCHAV3Validator) IsTokenValid(ctx context.Context, token string) (bool, error) {
	payload := fmt.Sprintf("secret=%s&response=%s", v.SiteSecretKey, token)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.VerifyTokenURL, strings.NewReader(payload))
	if err != nil {
		return false, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	resp, err := v.HTTPClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("error requesting verify reCAPTCHA v3 token: %w", err)
	}
	defer resp.Body.Close()

	respBodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("error reading body response: %w", err)
	}

	var respBody reCAPTCHAV3VerifyResponse
	if err := json.Unmarshal(respBodyBytes, &respBody); err != nil {
		return false, fmt.Errorf("error unmarshalling body response: %w", err)
	}

	if slices.Contains(respBody.ErrorCodes, timeoutOrDuplicateErrorCode) {
		return false, nil
	}

	if len(respBody.ErrorCodes) > 0 {
		return false, fmt.Errorf("error returned by verify reCAPTCHA v3 token: %v", respBody.ErrorCodes)
	}

	if !respBody.Success {
		return false, nil
	}

	if respBody.Score < v.MinScore {
		return false, fmt.Errorf("reCAPTCHA v3 score %.2f is below minimum threshold %.2f", respBody.Score, v.MinScore)
	}

	return true, nil
}

// NewGoogleReCAPTCHAV3Validator creates a new reCAPTCHA v3 validator.
func NewGoogleReCAPTCHAV3Validator(siteSecretKey string, minScore float64, httpClient HTTPClient) *GoogleReCAPTCHAV3Validator {
	if minScore <= 0 || minScore > 1.0 {
		minScore = defaultMinScore
	}

	return &GoogleReCAPTCHAV3Validator{
		SiteSecretKey:  siteSecretKey,
		VerifyTokenURL: recaptchaV3VerifyURL,
		MinScore:       minScore,
		HTTPClient:     httpClient,
	}
}
