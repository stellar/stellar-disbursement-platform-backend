package httphandler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/stellar/go/keypair"
	"github.com/stellar/go/support/render/httpjson"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/circle"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	sdpUtils "github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

// CircleConfigHandler implements a handler to configure the Circle API access.
type CircleConfigHandler struct {
	Encrypter                   sdpUtils.PrivateKeyEncrypter
	EncryptionPassphrase        string
	CircleClientConfigModel     *circle.ClientConfigModel
	DistributionAccountResolver signing.DistributionAccountResolver
}

type PatchCircleConfigRequest struct {
	WalletID *string `json:"wallet_id"`
	APIKey   *string `json:"api_key"`
}

// validate validates the request.
func (r PatchCircleConfigRequest) validate() error {
	if r.WalletID == nil && r.APIKey == nil {
		return fmt.Errorf("wallet_id or api_key must be provided")
	}
	return nil
}

// Patch is a handler to configure the Circle API access.
func (h CircleConfigHandler) Patch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	distAccount, err := h.DistributionAccountResolver.DistributionAccountFromContext(ctx)
	if err != nil {
		httperror.InternalError(ctx, "Cannot retrieve distribution account", err, nil).Render(w)
		return
	}

	if !distAccount.IsCircle() {
		errResponseMsg := fmt.Sprintf("This endpoint is only available for tenants using %v", schema.CirclePlatform)
		httperror.BadRequest(errResponseMsg, nil, nil).Render(w)
		return
	}

	var patchRequest PatchCircleConfigRequest
	err = json.NewDecoder(r.Body).Decode(&patchRequest)
	if err != nil {
		httperror.BadRequest("Request body is not valid", err, nil).Render(w)
		return
	}

	if err = patchRequest.validate(); err != nil {
		extras := map[string]interface{}{"validation_error": err.Error()}
		httperror.BadRequest("Request body is not valid", err, extras).Render(w)
		return
	}

	var clientConfigUpdate circle.ClientConfigUpdate
	if patchRequest.APIKey != nil {
		kp, kpErr := keypair.ParseFull(h.EncryptionPassphrase)
		if kpErr != nil {
			httperror.InternalError(ctx, "Cannot parse the encryption keypair", kpErr, nil).Render(w)
			return
		}

		encryptedAPIKey, encryptErr := h.Encrypter.Encrypt(*patchRequest.APIKey, kp.Seed())
		if encryptErr != nil {
			httperror.InternalError(ctx, "Cannot encrypt the API key", encryptErr, nil).Render(w)
			return
		}
		clientConfigUpdate.EncryptedAPIKey = &encryptedAPIKey
		encrypterPublicKey := kp.Address()
		clientConfigUpdate.EncrypterPublicKey = &encrypterPublicKey
	}

	if patchRequest.WalletID != nil {
		clientConfigUpdate.WalletID = patchRequest.WalletID
	}

	err = h.CircleClientConfigModel.Upsert(ctx, clientConfigUpdate)
	if err != nil {
		httperror.InternalError(ctx, "Cannot insert the Circle configuration", err, nil).Render(w)
		return
	}

	httpjson.RenderStatus(w, http.StatusOK, map[string]string{"message": "Circle configuration updated"}, httpjson.JSON)
}
