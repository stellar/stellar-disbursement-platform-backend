package httphandler

import (
	"net/http"

	"github.com/stellar/go/support/http/httpdecode"
	"github.com/stellar/go/support/render/httpjson"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

type EmbeddedWalletHandler struct {
	Models      *data.Models
	AuthManager auth.AuthManager
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

type GetAccountStatusRequest struct {
	ContractID string `json:"contract_id"`
}

func (r GetAccountStatusRequest) Validate() *httperror.HTTPError {
	validator := validators.NewValidator()
	validator.Check(len(r.ContractID) != 0, "contract_id", "contract_id should not be empty")
	if validator.HasErrors() {
		return httperror.BadRequest("", nil, validator.Errors)
	}
	return nil
}

type AccountStatusResponse struct {
	ContractID string `json:"contract_id"`
	Status     string `json:"status"`
	Message    string `json:"message"`
}

func (h EmbeddedWalletHandler) CreateAccount(rw http.ResponseWriter, req *http.Request) {
	var reqBody CreateAccountRequest
	if err := httpdecode.DecodeJSON(req, &reqBody); err != nil {
		httperror.BadRequest("", err, nil).Render(rw)
	}

	if err := reqBody.Validate(); err != nil {
		err.Render(rw)
		return
	}

	resp := AccountStatusResponse{}
	httpjson.RenderStatus(rw, http.StatusOK, resp, httpjson.JSON)
}

func (h EmbeddedWalletHandler) GetAccountStatus(rw http.ResponseWriter, req *http.Request) {
	var reqBody GetAccountStatusRequest
	if err := httpdecode.DecodeJSON(req, &reqBody); err != nil {
		httperror.BadRequest("", err, nil).Render(rw)
	}

	if err := reqBody.Validate(); err != nil {
		err.Render(rw)
		return
	}

	resp := AccountStatusResponse{}
	httpjson.RenderStatus(rw, http.StatusOK, resp, httpjson.JSON)
}
