package circle

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// APIError represents the error response from Circle APIs.
type APIError struct {
	// Code is the Circle API error code.
	Code    int              `json:"code"`
	Message string           `json:"message"`
	Errors  []APIErrorDetail `json:"errors,omitempty"`
	// StatusCode is the HTTP status code.
	StatusCode int `json:"status_code,omitempty"`
}

// APIErrorDetail represents the detailed error information.
type APIErrorDetail struct {
	Error        string                 `json:"error"`
	Message      string                 `json:"message"`
	Location     string                 `json:"location"`
	InvalidValue interface{}            `json:"invalidValue,omitempty"`
	Constraints  map[string]interface{} `json:"constraints,omitempty"`
}

// Error implements the error interface for APIError.
func (e APIError) Error() string {
	return fmt.Sprintf("APIError: Code=%d, Message=%s, Errors=%v, StatusCode=%d", e.Code, e.Message, e.Errors, e.StatusCode)
}

// parseAPIError parses the error response from Circle APIs.
// https://developers.circle.com/circle-mint/docs/circle-apis-api-errors.
func parseAPIError(resp *http.Response) (*APIError, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading error response body: %w", err)
	}
	defer resp.Body.Close()

	var apiErr APIError
	if err = json.Unmarshal(body, &apiErr); err != nil {
		return nil, fmt.Errorf("unmarshalling error response body: %w", err)
	}
	apiErr.StatusCode = resp.StatusCode

	return &apiErr, nil
}
