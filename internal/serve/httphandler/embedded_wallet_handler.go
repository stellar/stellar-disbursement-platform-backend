package httphandler

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/stellar/go/support/http/httpdecode"
	"github.com/stellar/go/support/render/httpjson"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

type EmbeddedWalletHandler struct {
	EmbeddedWalletService services.EmbeddedWalletServiceInterface
}

type CreateAccountRequest struct {
	CredentialID string `json:"credential_id"`
	PublicKey    string `json:"public_key"`
	ClaimToken   string `json:"claim_token"`
}

func (r CreateAccountRequest) Validate() *httperror.HTTPError {
	validator := validators.NewValidator()
	validator.Check(len(r.CredentialID) != 0, "credential_id", "credential_id should not be empty")
	validator.Check(len(r.PublicKey) != 0, "public_key", "public_key should not be empty")
	validator.Check(len(r.ClaimToken) != 0, "claim_token", "claim_token should not be empty")
	if validator.HasErrors() {
		return httperror.BadRequest("", nil, validator.Errors)
	}
	return nil
}

type AccountStatusResponse struct {
	ID      string              `json:"id"`
	Status  data.CreationStatus `json:"status"`
	Message string              `json:"message"`
}

func (h EmbeddedWalletHandler) CreateAccount(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	var reqBody CreateAccountRequest
	if err := httpdecode.DecodeJSON(req, &reqBody); err != nil {
		httperror.BadRequest("", err, nil).Render(rw)
	}

	if err := reqBody.Validate(); err != nil {
		err.Render(rw)
		return
	}

	currentTenant, err := tenant.GetTenantFromContext(ctx)
	if err != nil || currentTenant == nil {
		httperror.InternalError(ctx, "Failed to load tenant from context", err, nil).Render(rw)
		return
	}

	requestID := uuid.NewString()

	err = h.EmbeddedWalletService.QueueAccountCreation(req.Context(), currentTenant.ID, requestID, reqBody.CredentialID, reqBody.PublicKey)
	if err != nil {
		httperror.InternalError(ctx, "Cannot create account contract", err, nil).Render(rw)
		return
	}

	resp := AccountStatusResponse{
		ID:      requestID,
		Status:  data.PendingCreationStatus,
		Message: "Account creation is in progress",
	}
	httpjson.RenderStatus(rw, http.StatusOK, resp, httpjson.JSON)
}
