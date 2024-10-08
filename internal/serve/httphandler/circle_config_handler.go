package httphandler

import (
	"context"
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
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

// CircleConfigHandler implements a handler to configure the Circle API access.
type CircleConfigHandler struct {
	NetworkType                 sdpUtils.NetworkType
	CircleFactory               circle.ClientFactory
	TenantManager               tenant.ManagerInterface
	Encrypter                   sdpUtils.PrivateKeyEncrypter
	EncryptionPassphrase        string
	CircleClientConfigModel     circle.ClientConfigModelInterface
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

	tnt, err := tenant.GetTenantFromContext(ctx)
	if err != nil {
		httperror.InternalError(ctx, "Cannot retrieve the tenant from the context", err, nil).Render(w)
		return
	}

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

	validationErr := h.validateConfigWithCircle(ctx, patchRequest)
	if validationErr != nil {
		validationErr.Render(w)
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

	// Update tenant status to active
	_, err = h.TenantManager.UpdateTenantConfig(ctx, &tenant.TenantUpdate{
		ID:                        tnt.ID,
		DistributionAccountStatus: schema.AccountStatusActive,
	})
	if err != nil {
		httperror.InternalError(ctx, "Could not update the tenant status to ACTIVE", err, nil).Render(w)
		return
	}

	httpjson.RenderStatus(w, http.StatusOK, map[string]string{"message": "Circle configuration updated"}, httpjson.JSON)
}

func (h CircleConfigHandler) validateConfigWithCircle(ctx context.Context, patchRequest PatchCircleConfigRequest) *httperror.HTTPError {
	if err := patchRequest.validate(); err != nil {
		return httperror.BadRequest("Request body is not valid", err, nil)
	}

	// Use the request values for walletID and apiKey, if they were provided.
	var walletID, apiKey string
	if patchRequest.APIKey != nil {
		apiKey = *patchRequest.APIKey
	}
	if patchRequest.WalletID != nil {
		walletID = *patchRequest.WalletID
	}

	// If walletID or apiKey are not provided, try to get them from the existing configuration.
	if walletID == "" || apiKey == "" {
		existingConfig, err := h.CircleClientConfigModel.Get(ctx)
		if err != nil {
			return httperror.InternalError(ctx, "Cannot retrieve the existing Circle configuration", err, nil)
		}

		if existingConfig == nil {
			return httperror.BadRequest("You must provide both the Circle walletID and Circle APIKey during the first configuration", nil, nil)
		}

		if walletID == "" && existingConfig.WalletID != nil { // walletID is not provided but exists in the DB
			walletID = *existingConfig.WalletID
		}

		if apiKey == "" && existingConfig.EncryptedAPIKey != nil { // apiKey is not provided but exists in the DB
			apiKey, err = h.Encrypter.Decrypt(*existingConfig.EncryptedAPIKey, h.EncryptionPassphrase)
			if err != nil {
				return httperror.InternalError(ctx, "Cannot decrypt the API key", err, nil)
			}
		}
	}

	circleClient := h.CircleFactory(h.NetworkType, apiKey, h.TenantManager)

	// validate incoming APIKey
	if patchRequest.APIKey != nil {
		ok, err := circleClient.Ping(ctx)
		if err != nil {
			return wrapCircleError(ctx, err)
		}

		if !ok {
			return httperror.BadRequest("Failed to ping, please make sure that the provided API Key is correct.", nil, nil)
		}
	}

	// validate incoming WalletID
	if patchRequest.WalletID != nil {
		accountConfig, err := circleClient.GetAccountConfiguration(ctx)
		if err != nil {
			return wrapCircleError(ctx, err)
		}
		if accountConfig.Payments.MasterWalletID != walletID {
			return httperror.BadRequest("The provided wallet ID does not match the master wallet ID from Circle", nil, nil)
		}
	}

	return nil
}
