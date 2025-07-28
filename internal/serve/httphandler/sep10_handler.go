package httphandler

import (
	"encoding/json"
	"net/http"

	"github.com/stellar/go/strkey"
	"github.com/stellar/go/support/log"
	"github.com/stellar/go/support/render/httpjson"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
)

type SEP10Handler struct {
	SEP10Service services.SEP10Service
}

// GetChallenge handles GET /auth requests for SEP-10 authentication
func (h SEP10Handler) GetChallenge(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	req := services.ChallengeRequest{
		Account:      r.URL.Query().Get("account"),
		Memo:         r.URL.Query().Get("memo"),
		HomeDomain:   r.URL.Query().Get("home_domain"),
		ClientDomain: r.URL.Query().Get("client_domain"),
	}

	if req.Account == "" {
		httperror.BadRequest("account is required", nil, nil).Render(w)
		return
	}

	if !strkey.IsValidEd25519PublicKey(req.Account) {
		httperror.BadRequest("invalid account format", nil, nil).Render(w)
		return
	}

	challenge, err := h.SEP10Service.CreateChallenge(ctx, req)
	if err != nil {
		log.Ctx(ctx).Errorf("creating SEP-10 challenge: %v", err)
		httperror.InternalError(ctx, "Failed to create challenge", err, nil).Render(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	httpjson.Render(w, challenge, httpjson.JSON)
}

// PostChallenge handles POST /auth requests for SEP-10 authentication
func (h SEP10Handler) PostChallenge(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req services.ValidationRequest

	contentType := r.Header.Get("Content-Type")
	if contentType == "application/x-www-form-urlencoded" || contentType == "" {
		if err := r.ParseForm(); err != nil {
			httperror.BadRequest("invalid form data", err, nil).Render(w)
			return
		}
		req.Transaction = r.FormValue("transaction")
	} else {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httperror.BadRequest("invalid request body", err, nil).Render(w)
			return
		}
	}

	if req.Transaction == "" {
		httperror.BadRequest("transaction is required", nil, nil).Render(w)
		return
	}

	response, err := h.SEP10Service.ValidateChallenge(ctx, req)
	if err != nil {
		log.Ctx(ctx).Errorf("validating SEP-10 challenge: %v", err)
		httperror.BadRequest("challenge validation failed", err, nil).Render(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	httpjson.Render(w, response, httpjson.JSON)
}
