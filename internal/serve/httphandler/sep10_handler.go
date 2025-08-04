package httphandler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/stellar/go/strkey"
	"github.com/stellar/go/support/log"
	"github.com/stellar/go/support/render/httpjson"
	"github.com/stellar/go/txnbuild"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

type SEP10Handler struct {
	SEP10Service services.SEP10Service
}

// parseContentType parses the Content-Type header and returns the base MIME type
func parseContentType(contentType string) string {
	if contentType == "" {
		return ""
	}

	parts := strings.Split(contentType, ";")
	return strings.TrimSpace(parts[0])
}

// validateChallengeRequest validates all challenge request parameters
func (h SEP10Handler) validateChallengeRequest(req services.ChallengeRequest) error {
	if req.Account == "" {
		return fmt.Errorf("account is required")
	}

	if !strkey.IsValidEd25519PublicKey(req.Account) {
		return fmt.Errorf("invalid account format - must be a valid Ed25519 public key")
	}

	if req.Memo != "" {
		memo, err := schema.NewMemo(schema.MemoTypeID, req.Memo)
		if err != nil {
			return fmt.Errorf("invalid memo must be a positive integer")
		}
		if _, ok := memo.(txnbuild.MemoID); !ok {
			return fmt.Errorf("invalid memo type")
		}
	}

	return nil
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

	if err := h.validateChallengeRequest(req); err != nil {
		httperror.BadRequest(err.Error(), nil, nil).Render(w)
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

	contentType := parseContentType(r.Header.Get("Content-Type"))

	switch contentType {
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
