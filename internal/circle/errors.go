package circle

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// APIError represents the error response from Circle APIs
type APIError struct {
	Code    int              `json:"code"`
	Message string           `json:"message"`
	Errors  []APIErrorDetail `json:"errors,omitempty"`
}

// APIErrorDetail represents the detailed error information
type APIErrorDetail struct {
	Error        string                 `json:"error"`
	Message      string                 `json:"message"`
	Location     string                 `json:"location"`
	InvalidValue interface{}            `json:"invalidValue,omitempty"`
	Constraints  map[string]interface{} `json:"constraints,omitempty"`
}

// Error implements the error interface for APIError
func (e APIError) Error() string {
	return fmt.Sprintf("APIError: Code=%d, Message=%s, Errors=%v", e.Code, e.Message, e.Errors)
}

// parseAPIError parses the error response from Circle APIs
func parseAPIError(resp *http.Response) error {
	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("unauthorized to access the resource")
	}

	if resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("forbidden to access the resource")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading error response body: %w", err)
	}
	defer resp.Body.Close()

	var apiErr APIError
	if err = json.Unmarshal(body, &apiErr); err != nil {
		return fmt.Errorf("unmarshalling error response body: %w", err)
	}

	return apiErr
}
