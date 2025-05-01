package httphandler

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stellar/go/support/http/httpdecode"
	"github.com/stellar/go/support/log"
	"github.com/stellar/go/support/render/httpjson"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/middleware"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
)

type APIKeyHandler struct {
	Models *data.Models
}

type CreateAPIKeyRequest struct {
	Name        string                  `json:"name"`
	Permissions []data.APIKeyPermission `json:"permissions"`
	ExpiryDate  *time.Time              `json:"expiry_date,omitempty"`
	AllowedIPs  any                     `json:"allowed_ips,omitempty"` // Can be a string or array of strings
}

func (h APIKeyHandler) CreateAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req CreateAPIKeyRequest
	if err := httpdecode.DecodeJSON(r, &req); err != nil {
		httperror.BadRequest("Invalid request body", err, nil).Render(w)
		return
	}

	v := validators.NewValidator()

	v.Check(req.Name != "", "name", "name is required")
	v.Check(len(req.Permissions) > 0, "permissions", "at least one permission is required")

	if err := data.ValidatePermissions(req.Permissions); err != nil {
		v.AddError("permissions", err.Error())
	}

	allowedIPs, err := parseAllowedIPs(req.AllowedIPs)
	if err != nil {
		v.AddError("allowed_ips", err.Error())
	} else if validationErr := data.ValidateAllowedIPs(allowedIPs); validationErr != nil {
		v.AddError("allowed_ips", validationErr.Error())
	}

	if req.ExpiryDate != nil && req.ExpiryDate.Before(time.Now()) {
		v.AddError("expiry_date", "expiry date must be in the future")
	}

	if v.HasErrors() {
		httperror.BadRequest("Request validation failed", nil, v.Errors).Render(w)
		return
	}

	userID, ok := ctx.Value(middleware.UserIDContextKey).(string)
	if !ok {
		log.Ctx(ctx).Error("User ID not found in context")
		httperror.InternalError(ctx, "User identification error", nil, nil).Render(w)
		return
	}

	apiKey, err := h.Models.APIKeys.Insert(
		ctx,
		req.Name,
		req.Permissions,
		allowedIPs,
		req.ExpiryDate,
		userID,
	)
	if err != nil {
		log.Ctx(ctx).Errorf("Error creating API key: %s", err)
		httperror.InternalError(ctx, "Failed to create API key", err, nil).Render(w)
		return
	}

	httpjson.RenderStatus(w, http.StatusCreated, apiKey, httpjson.JSON)
}

func (h APIKeyHandler) GetApiKeyByID(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	keyID := chi.URLParam(r, "id")

	userID, ok := ctx.Value(middleware.UserIDContextKey).(string)
	if !ok {
		log.Ctx(ctx).Error("User ID not found in context")
		httperror.InternalError(ctx, "User identification error", nil, nil).Render(w)
		return
	}

	key, err := h.Models.APIKeys.GetByID(ctx, keyID, userID)
	if err != nil {
		if errors.Is(err, data.ErrNotFound) {
			httperror.NotFound("API key not found", nil, nil).Render(w)
		} else {
			log.Ctx(ctx).Errorf("Error fetching API key: %s", err)
			httperror.InternalError(ctx, "Failed to retrieve API key", err, nil).Render(w)
		}
		return
	}

	httpjson.RenderStatus(w, http.StatusOK, key, httpjson.JSON)
}

func (h APIKeyHandler) GetAllApiKeys(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userID, ok := ctx.Value(middleware.UserIDContextKey).(string)
	if !ok {
		log.Ctx(ctx).Error("User ID not found in context")
		httperror.InternalError(ctx, "User identification error", nil, nil).Render(w)
		return
	}

	apiKeys, err := h.Models.APIKeys.GetAll(ctx, userID)
	if err != nil {
		log.Ctx(ctx).Errorf("Error retrieving API keys: %s", err)
		httperror.InternalError(ctx, "Failed to retrieve API keys", err, nil).Render(w)
		return
	}

	httpjson.RenderStatus(w, http.StatusOK, apiKeys, httpjson.JSON)
}

func (h APIKeyHandler) DeleteApiKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	keyID := chi.URLParam(r, "id")

	userID, ok := ctx.Value(middleware.UserIDContextKey).(string)
	if !ok {
		httperror.InternalError(ctx, "User identification error", nil, nil).Render(w)
		return
	}

	if err := h.Models.APIKeys.Delete(ctx, keyID, userID); err != nil {
		if errors.Is(err, data.ErrNotFound) {
			httperror.NotFound("API key not found", nil, nil).Render(w)
		} else {
			httperror.InternalError(ctx, "Failed to delete API key", err, nil).Render(w)
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// parseAllowedIPs converts the allowed_ips field from the request into a string slice.
func parseAllowedIPs(input any) ([]string, error) {
	if input == nil {
		return []string{}, nil
	}

	if strArray, ok := input.([]string); ok {
		return strArray, nil
	}

	if arr, ok := input.([]any); ok {
		strArray := make([]string, 0, len(arr))
		for i, item := range arr {
			str, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("item at position %d must be a string", i)
			}
			strArray = append(strArray, str)
		}
		return strArray, nil
	}

	if str, ok := input.(string); ok {
		return []string{str}, nil
	}

	return nil, fmt.Errorf("must be a string or array of strings")
}
