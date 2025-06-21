package httphandler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/stellar/go/support/render/httpjson"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/bridge"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	ctxHelper "github.com/stellar/stellar-disbursement-platform-backend/internal/serve/auth"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

// BridgeIntegrationHandler handles Bridge integration endpoints.
type BridgeIntegrationHandler struct {
	BridgeService               bridge.ServiceInterface
	AuthManager                 auth.AuthManager
	Models                      *data.Models
	DistributionAccountResolver signing.DistributionAccountResolver
}

// Get handles GET /bridge-integration
func (h BridgeIntegrationHandler) Get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if h.BridgeService == nil {
		response := bridge.BridgeIntegrationInfo{
			Status: data.BridgeIntegrationStatusNotEnabled,
		}
		httpjson.RenderStatus(w, http.StatusOK, response, httpjson.JSON)
		return
	}

	bridgeInfo, err := h.BridgeService.GetBridgeIntegration(ctx)
	if err != nil {
		httperror.InternalError(ctx, "Failed to get Bridge integration status", err, nil).Render(w)
		return
	}

	httpjson.RenderStatus(w, http.StatusOK, bridgeInfo, httpjson.JSON)
}

// OptInRequest represents the request to opt into Bridge integration
type OptInRequest struct {
	Status   data.BridgeIntegrationStatus `json:"status"`
	Email    string                       `json:"email,omitempty"`
	FullName string                       `json:"full_name,omitempty"`
}

// Validate validates the opt-in request
func (r OptInRequest) Validate() error {
	if r.Status != data.BridgeIntegrationStatusOptedIn {
		return fmt.Errorf("status must be OPTED_IN")
	}
	if r.Email != "" {
		if err := utils.ValidateEmail(r.Email); err != nil {
			return fmt.Errorf("invalid email: %w", err)
		}
	}
	if r.FullName != "" && strings.TrimSpace(r.FullName) == "" {
		return fmt.Errorf("full_name cannot be empty or whitespace only")
	}
	return nil
}

// Patch handles PATCH /bridge-integration
func (h BridgeIntegrationHandler) Patch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Check if Bridge service is enabled
	if h.BridgeService == nil {
		httperror.BadRequest("Bridge integration is not enabled", nil, nil).Render(w)
		return
	}

	// Parse request
	var optInRequest OptInRequest
	err := json.NewDecoder(r.Body).Decode(&optInRequest)
	if err != nil {
		httperror.BadRequest("Invalid request body", err, nil).Render(w)
		return
	}

	if err = optInRequest.Validate(); err != nil {
		extras := map[string]interface{}{"validation_error": err.Error()}
		httperror.BadRequest("Invalid request", err, extras).Render(w)
		return
	}

	// Get user from context
	user, err := ctxHelper.GetUserFromContext(ctx, h.AuthManager)
	if err != nil {
		httperror.InternalError(ctx, "Cannot retrieve user from context", err, nil).Render(w)
		return
	}

	// Use provided email/fullName or default to user info
	email := optInRequest.Email
	if email == "" {
		email = user.Email
	}
	if err = utils.ValidateEmail(email); err != nil {
		httperror.BadRequest("Invalid email format", err, nil).Render(w)
		return
	}

	fullName := optInRequest.FullName
	if fullName == "" {
		firstName := strings.TrimSpace(user.FirstName)
		lastName := strings.TrimSpace(user.LastName)
		fullName = fmt.Sprintf("%s %s", firstName, lastName)
	}

	// Opt into Bridge integration
	bridgeInfo, err := h.BridgeService.OptInToBridge(ctx, user.ID, fullName, email)
	if err != nil {
		var bridgeError bridge.BridgeErrorResponse
		switch {
		case errors.Is(err, bridge.ErrBridgeAlreadyOptedIn):
			httperror.BadRequest("Your organization has already opted into Bridge integration", nil, nil).Render(w)
			return
		case errors.As(err, &bridgeError):
			extras := bridgeErrorToExtras(bridgeError)
			httperror.BadRequest("Opt-in to Bridge integration failed", err, extras).Render(w)
			return
		}
		// For other errors, treat as internal server error
		httperror.InternalError(ctx, "Failed to opt into Bridge integration", err, nil).Render(w)
		return
	}

	httpjson.RenderStatus(w, http.StatusOK, bridgeInfo, httpjson.JSON)
}

// VirtualAccountRequest represents the request to create a virtual account
type VirtualAccountRequest struct {
	Memo string `json:"memo,omitempty"`
}

// Validate validates the virtual account request
func (r VirtualAccountRequest) Validate() error {
	if strings.TrimSpace(r.Memo) == "" {
		return fmt.Errorf("memo cannot be empty")
	}
	if _, memoType, err := schema.ParseMemo(r.Memo); err != nil {
		return fmt.Errorf("invalid memo format for memo type %s: %w", memoType, err)
	}
	return nil
}

// PostVirtualAccount handles POST /bridge-integration/virtual-account
func (h BridgeIntegrationHandler) PostVirtualAccount(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Check if Bridge service is enabled
	if h.BridgeService == nil {
		httperror.BadRequest("Bridge integration is not enabled", nil, nil).Render(w)
		return
	}

	// Parse request
	var vaRequest VirtualAccountRequest
	err := json.NewDecoder(r.Body).Decode(&vaRequest)
	if err != nil {
		httperror.BadRequest("Invalid request body", err, nil).Render(w)
		return
	}

	if err = vaRequest.Validate(); err != nil {
		extras := map[string]interface{}{"validation_error": err.Error()}
		httperror.BadRequest("Invalid request", err, extras).Render(w)
		return
	}

	// Get distribution account address from the tenant
	distributionAccount, err := h.DistributionAccountResolver.DistributionAccountFromContext(ctx)
	if err != nil {
		httperror.InternalError(ctx, "Failed to get distribution account", err, nil).Render(w)
		return
	}

	// Get user from context
	user, err := ctxHelper.GetUserFromContext(ctx, h.AuthManager)
	if err != nil {
		httperror.InternalError(ctx, "Cannot retrieve user from context", err, nil).Render(w)
		return
	}

	// Create virtual account
	bridgeInfo, err := h.BridgeService.CreateVirtualAccount(ctx, user.ID, vaRequest.Memo, distributionAccount.Address)
	if err != nil {
		// Check for specific Bridge service errors first
		var bridgeError bridge.BridgeErrorResponse
		switch {
		case errors.As(err, &bridgeError):
			extras := bridgeErrorToExtras(bridgeError)
			httperror.BadRequest("Virtual account creation failed", err, extras).Render(w)
			return
		case errors.Is(err, bridge.ErrBridgeNotOptedIn):
			httperror.BadRequest("Organization must opt into Bridge integration before creating a virtual account", nil, nil).Render(w)
			return
		case errors.Is(err, bridge.ErrBridgeVirtualAccountAlreadyExists):
			httperror.BadRequest("Virtual account already exists for this organization", nil, nil).Render(w)
			return
		case errors.Is(err, bridge.ErrBridgeKYCNotApproved):
			httperror.BadRequest("KYC verification must be approved before creating a virtual account", nil, nil).Render(w)
			return
		case errors.Is(err, bridge.ErrBridgeTOSNotAccepted):
			httperror.BadRequest("Terms of service must be accepted before creating a virtual account", nil, nil).Render(w)
			return
		case errors.Is(err, bridge.ErrBridgeKYCRejected):
			httperror.BadRequest("Cannot create virtual account because KYC verification was rejected", nil, nil).Render(w)
			return
		}

		// For other errors, treat as internal server error
		httperror.InternalError(ctx, "Failed to create virtual account", err, nil).Render(w)
		return
	}

	httpjson.RenderStatus(w, http.StatusOK, bridgeInfo, httpjson.JSON)
}

func bridgeErrorToExtras(bridgeError bridge.BridgeErrorResponse) map[string]interface{} {
	extras := map[string]interface{}{
		"bridge_error_code": bridgeError.Code,
		"bridge_error_type": bridgeError.Type,
	}
	if bridgeError.Details != "" {
		extras["bridge_error_details"] = bridgeError.Details
	}
	if bridgeError.Source.Location != "" {
		extras["bridge_error_source_location"] = bridgeError.Source.Location
	}
	if len(bridgeError.Source.Key) > 0 {
		extras["bridge_error_source_key"] = bridgeError.Source.Key
	}
	return extras
}
