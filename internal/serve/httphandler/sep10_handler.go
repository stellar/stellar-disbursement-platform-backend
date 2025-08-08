package httphandler

import (
	"encoding/json"
	"mime"
	"net/http"

	"github.com/stellar/go/support/render/httpjson"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
)

type SEP10Handler struct {
	SEP10Service services.SEP10Service
}

// GetChallenge handles GET /auth requests for SEP-10 authentication.
func (h SEP10Handler) GetChallenge(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	req := services.ChallengeRequest{
		Account:      r.URL.Query().Get("account"),
		Memo:         r.URL.Query().Get("memo"),
		HomeDomain:   r.URL.Query().Get("home_domain"),
		ClientDomain: r.URL.Query().Get("client_domain"),
	}

	if err := req.Validate(); err != nil {
		httperror.BadRequest(err.Error(), nil, nil).Render(w)
		return
	}

	challenge, err := h.SEP10Service.CreateChallenge(ctx, req)
	if err != nil {
		httperror.InternalError(ctx, "Failed to create challenge", err, nil).Render(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	httpjson.Render(w, challenge, httpjson.JSON)
}

// PostChallenge handles POST /auth requests for SEP-10 authentication.
func (h SEP10Handler) PostChallenge(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req services.ValidationRequest

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
		req.Transaction = r.FormValue("transaction")
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

	response, err := h.SEP10Service.ValidateChallenge(ctx, req)
	if err != nil {
		httperror.BadRequest("challenge validation failed", err, nil).Render(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	httpjson.Render(w, response, httpjson.JSON)
}
