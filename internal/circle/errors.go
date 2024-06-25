package circle

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"

	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

// APIError represents the error response from Circle APIs.
type APIError struct {
	Code    int              `json:"code"`
	Message string           `json:"message"`
	Errors  []APIErrorDetail `json:"errors,omitempty"`
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
	return fmt.Sprintf("APIError: Code=%d, Message=%s, Errors=%v", e.Code, e.Message, e.Errors)
}

type TenantStatusUpdater struct {
	tntManager tenant.ManagerInterface
}

var invalidAPIKeyStatusCodes = []int{http.StatusUnauthorized, http.StatusForbidden}

// parseAPIError parses the error response from Circle APIs.
// https://developers.circle.com/circle-mint/docs/circle-apis-api-errors.
func (u TenantStatusUpdater) parseAPIError(ctx context.Context, resp *http.Response) (*APIError, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading error response body: %w", err)
	}
	defer resp.Body.Close()

	if slices.Contains(invalidAPIKeyStatusCodes, resp.StatusCode) {
		tnt, getCtxTntErr := tenant.GetTenantFromContext(ctx)
		if getCtxTntErr != nil {
			return nil, fmt.Errorf("getting tenant from context: %w", getCtxTntErr)
		}

		deactivateTntErr := u.tntManager.DeactivateTenantDistributionAccount(ctx, tnt.ID)
		if deactivateTntErr != nil {
			return nil, fmt.Errorf("deactivating tenant distribution account: %w", deactivateTntErr)
		}
	}

	var apiErr APIError
	if err = json.Unmarshal(body, &apiErr); err != nil {
		return nil, fmt.Errorf("unmarshalling error response body: %w", err)
	}

	return &apiErr, nil
}
