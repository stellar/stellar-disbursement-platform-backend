package httphandler

import (
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"strings"

	"github.com/stellar/go-stellar-sdk/support/render/httpjson"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
)

type SEP45Handler struct {
	SEP45Service services.SEP45Service
}

// GetChallenge handles GET /sep45/auth requests for SEP-45 authentication.
func (h SEP45Handler) GetChallenge(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	clientDomain := r.URL.Query().Get("client_domain")
	var clientDomainPtr *string
	if strings.TrimSpace(clientDomain) != "" {
		clientDomainPtr = &clientDomain
	}

	req := services.SEP45ChallengeRequest{
		Account:      r.URL.Query().Get("account"),
		HomeDomain:   r.URL.Query().Get("home_domain"),
		ClientDomain: clientDomainPtr,
	}

	if err := req.Validate(); err != nil {
		httperror.BadRequest(err.Error(), nil, nil).Render(w)
		return
	}

	challenge, err := h.SEP45Service.CreateChallenge(ctx, req)
	if err != nil {
		if errors.Is(err, services.ErrSEP45Validation) {
			httperror.BadRequest(err.Error(), err, nil).Render(w)
		} else {
			httperror.InternalError(ctx, "Failed to create challenge", err, nil).Render(w)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	httpjson.Render(w, challenge, httpjson.JSON)
}

// PostChallenge handles POST /sep45/auth requests for SEP-45 authentication validation.
func (h SEP45Handler) PostChallenge(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req services.SEP45ValidationRequest

	contentType := r.Header.Get("Content-Type")
	var mediaType string
	if contentType != "" {
		if parsed, _, err := mime.ParseMediaType(contentType); err == nil {
			mediaType = parsed
		} else {
			mediaType = contentType
		}
	}

	switch mediaType {
	case "application/x-www-form-urlencoded":
		if err := r.ParseForm(); err != nil {
			httperror.BadRequest("invalid form data", err, nil).Render(w)
			return
		}
		req.AuthorizationEntries = r.FormValue("authorization_entries")
	case "application/json":
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httperror.BadRequest("invalid request body", err, nil).Render(w)
			return
		}
	default:
		httperror.BadRequest("unsupported content type. Expected application/x-www-form-urlencoded or application/json", nil, nil).Render(w)
		return
	}

	if err := req.Validate(); err != nil {
		httperror.BadRequest(err.Error(), nil, nil).Render(w)
		return
	}

	response, err := h.SEP45Service.ValidateChallenge(ctx, req)
	if err != nil {
		if errors.Is(err, services.ErrSEP45Validation) {
			httperror.BadRequest("challenge validation failed", err, nil).Render(w)
		} else {
			httperror.InternalError(ctx, "challenge validation failed", err, nil).Render(w)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	httpjson.Render(w, response, httpjson.JSON)
}
