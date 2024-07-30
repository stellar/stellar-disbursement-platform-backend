package httphandler

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/support/http/httpdecode"
	"github.com/stellar/go/support/log"
	"github.com/stellar/go/support/render/httpjson"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	coreSvc "github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/internal/provisioning"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/internal/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

type TenantsHandler struct {
	Manager                     tenant.ManagerInterface
	Models                      *data.Models
	DistributionAccountService  coreSvc.DistributionAccountServiceInterface
	HorizonClient               horizonclient.ClientInterface
	MessengerClient             message.MessengerClient
	DistributionAccountResolver signing.DistributionAccountResolver
	ProvisioningManager         *provisioning.Manager
	NetworkType                 utils.NetworkType
	AdminDBConnectionPool       db.DBConnectionPool
	CrashTrackerClient          crashtracker.CrashTrackerClient
	SingleTenantMode            bool
	BaseURL                     string
	SDPUIBaseURL                string
}

const MaxNativeAssetBalanceForDeletion = 100

func (t TenantsHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	tnts, err := t.Manager.GetAllTenants(ctx, &tenant.QueryParams{})
	if err != nil {
		httperror.InternalError(ctx, "Cannot get tenants", err, nil).Render(w)
		return
	}

	httpjson.RenderStatus(w, http.StatusOK, tnts, httpjson.JSON)
}

func (t TenantsHandler) GetByIDOrName(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	arg := chi.URLParam(r, "arg")

	tnt, err := t.Manager.GetTenantByIDOrName(ctx, arg)
	if err != nil {
		if errors.Is(tenant.ErrTenantDoesNotExist, err) {
			errorMsg := fmt.Sprintf("tenant %s does not exist", arg)
			httperror.NotFound(errorMsg, err, nil).Render(w)
			return
		}
		httperror.InternalError(ctx, "Cannot get tenant by ID or name", err, nil).Render(w)
		return
	}

	httpjson.RenderStatus(w, http.StatusOK, tnt, httpjson.JSON)
}

func (h TenantsHandler) Post(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	var reqBody *validators.TenantRequest
	if err := httpdecode.DecodeJSON(req, &reqBody); err != nil {
		log.Ctx(ctx).Errorf("decoding request body: %v", err)
		httperror.BadRequest("", err, nil).Render(rw)
		return
	}

	validator := validators.NewTenantValidator()
	reqBody = validator.ValidateCreateTenantRequest(reqBody)
	if validator.HasErrors() {
		httperror.BadRequest("invalid request body", nil, validator.Errors).Render(rw)
		return
	}

	// generate SDP UI URL first if necessary since we need to pass it to the provisioning manager when
	// sending the invitation message
	tntSDPUIBaseURL, err := h.generateTenantURL(reqBody.SDPUIBaseURL, h.SDPUIBaseURL, reqBody.Name)
	if err != nil {
		httperror.InternalError(ctx, fmt.Sprintf("Could not generate SDP UI URL: %v", err), err, nil).Render(rw)
		return
	}

	tntBaseURL, err := h.generateTenantURL(reqBody.BaseURL, h.BaseURL, reqBody.Name)
	if err != nil {
		httperror.InternalError(ctx, fmt.Sprintf("Could not generate URL: %v", err), err, nil).Render(rw)
		return
	}

	tnt, err := h.ProvisioningManager.ProvisionNewTenant(ctx, provisioning.ProvisionTenant{
		Name:                    reqBody.Name,
		UserFirstName:           reqBody.OwnerFirstName,
		UserLastName:            reqBody.OwnerLastName,
		UserEmail:               reqBody.OwnerEmail,
		OrgName:                 reqBody.OrganizationName,
		NetworkType:             string(h.NetworkType),
		UiBaseURL:               tntSDPUIBaseURL,
		BaseURL:                 tntBaseURL,
		DistributionAccountType: schema.AccountType(reqBody.DistributionAccountType),
	})
	if err != nil {
		if errors.Is(err, tenant.ErrDuplicatedTenantName) {
			httperror.BadRequest("Tenant name already exists", err, nil).Render(rw)
			return
		}
		httperror.InternalError(ctx, fmt.Sprintf("Could not provision a new tenant: %v", err), err, nil).Render(rw)
		return
	}

	log.Ctx(ctx).Infof("Tenant %s created successfully.", tnt.Name)

	if err = h.sendInvitationMessage(ctx, tnt.Name, coreSvc.SendInvitationMessageOptions{
		FirstName: reqBody.OwnerFirstName,
		Email:     reqBody.OwnerEmail,
		Role:      data.OwnerUserRole.String(),
		UIBaseURL: tntSDPUIBaseURL,
	}); err != nil {
		h.CrashTrackerClient.LogAndReportErrors(ctx, err, "Cannot send invitation message")
	}

	httpjson.RenderStatus(rw, http.StatusCreated, tnt, httpjson.JSON)
}

func (h TenantsHandler) generateTenantURL(providedURL *string, defaultURL string, tenantName string) (string, error) {
	if providedURL != nil {
		return *providedURL, nil
	}
	return utils.GenerateTenantURL(defaultURL, tenantName)
}

func (h TenantsHandler) sendInvitationMessage(
	ctx context.Context, tntName string, opts coreSvc.SendInvitationMessageOptions,
) error {
	tenantSchemaDSN, err := h.Manager.GetDSNForTenant(ctx, tntName)
	if err != nil {
		return fmt.Errorf("getting database DSN for tenant %s", tntName)
	}

	tenantSchemaConnectionPool, models, err := provisioning.GetTenantSchemaDBConnectionAndModels(tenantSchemaDSN)
	if err != nil {
		return fmt.Errorf("opening database connection on tenant schema and getting model: %w", err)
	}
	defer tenantSchemaConnectionPool.Close()

	if err = coreSvc.SendInvitationMessage(ctx, h.MessengerClient, models, opts); err != nil {
		return fmt.Errorf("creating and sending invitation message: %w", err)
	}

	return nil
}

func (t TenantsHandler) Patch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var reqBody *validators.UpdateTenantRequest
	if err := httpdecode.DecodeJSON(r, &reqBody); err != nil {
		err = fmt.Errorf("decoding request body: %w", err)
		log.Ctx(ctx).Error(err)
		httperror.BadRequest("", err, nil).Render(w)
		return
	}

	validator := validators.NewTenantValidator()
	reqBody = validator.ValidateUpdateTenantRequest(reqBody)
	if validator.HasErrors() {
		httperror.BadRequest("invalid request body", nil, validator.Errors).Render(w)
		return
	}

	tenantID := chi.URLParam(r, "id")

	// factor out to own method
	if reqBody.Status != nil {
		err := services.ValidateStatus(ctx, t.Manager, t.Models, tenantID, *reqBody.Status)
		if err != nil {
			if errors.Is(err, services.ErrCannotRetrieveTenantByID) {
				httperror.InternalError(ctx, services.ErrCannotRetrieveTenantByID.Error(), err, nil).Render(w)
			} else if errors.Is(err, services.ErrCannotRetrievePayments) {
				httperror.InternalError(ctx, services.ErrCannotRetrievePayments.Error(), err, nil).Render(w)
			} else {
				httperror.BadRequest(err.Error(), nil, nil).Render(w)
			}
			return
		}
	}

	tnt, err := t.Manager.UpdateTenantConfig(ctx, &tenant.TenantUpdate{
		ID:           tenantID,
		BaseURL:      reqBody.BaseURL,
		SDPUIBaseURL: reqBody.SDPUIBaseURL,
		Status:       reqBody.Status,
	})
	if err != nil {
		if errors.Is(tenant.ErrEmptyUpdateTenant, err) {
			errorMsg := fmt.Sprintf("updating tenant %s: %s", tenantID, err)
			httperror.BadRequest(errorMsg, err, nil).Render(w)
			return
		}
		if errors.Is(tenant.ErrTenantDoesNotExist, err) {
			errorMsg := fmt.Sprintf("updating tenant: tenant %s does not exist", tenantID)
			httperror.NotFound(errorMsg, err, nil).Render(w)
			return
		}
		err = fmt.Errorf("updating tenant: %w", err)
		httperror.InternalError(ctx, "", err, nil).Render(w)
		return
	}

	httpjson.RenderStatus(w, http.StatusOK, tnt, httpjson.JSON)
}

func (t TenantsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := chi.URLParam(r, "id")

	tnt, err := t.Manager.GetTenant(ctx, &tenant.QueryParams{
		Filters: map[tenant.FilterKey]interface{}{tenant.FilterKeyID: tenantID},
	})
	if err != nil {
		if errors.Is(tenant.ErrTenantDoesNotExist, err) {
			errorMsg := fmt.Sprintf("tenant %s does not exist", tenantID)
			httperror.NotFound(errorMsg, err, nil).Render(w)
			return
		}

		httperror.InternalError(ctx, "Cannot get tenant by ID", err, nil).Render(w)
		return
	}

	if tnt.DeletedAt != nil {
		log.Ctx(ctx).Warnf("Tenant %s is already deleted", tenantID)
		httpjson.RenderStatus(w, http.StatusNotModified, tnt, httpjson.JSON)
		return
	}

	if tnt.Status != tenant.DeactivatedTenantStatus {
		httperror.BadRequest("Tenant must be deactivated to be eligible for deletion", nil, nil).Render(w)
		return
	}

	if tnt.DistributionAccountAddress != nil && t.DistributionAccountResolver.HostDistributionAccount().Address != *tnt.DistributionAccountAddress {
		tntDistributionAcc, getTntDistAccErr := t.DistributionAccountResolver.DistributionAccount(ctx, tnt.ID)
		if getTntDistAccErr != nil {
			httperror.InternalError(ctx, "Cannot get tenant distribution account", getTntDistAccErr, nil).Render(w)
			return
		}

		distAccBalances, getBalErr := t.DistributionAccountService.GetBalances(ctx, &tntDistributionAcc)
		if getBalErr != nil {
			httperror.InternalError(ctx, "Cannot get tenant distribution account balances", getBalErr, nil).Render(w)
			return
		}

		for asset, assetBalance := range distAccBalances {
			if asset.IsNative() {
				if assetBalance > MaxNativeAssetBalanceForDeletion {
					errMsg := fmt.Sprintf("Tenant distribution account must have a balance of less than %d XLM to be eligible for deletion", MaxNativeAssetBalanceForDeletion)
					httperror.BadRequest(errMsg, nil, nil).Render(w)
					return
				}
			} else {
				if assetBalance != 0 {
					errMsg := fmt.Sprintf("Tenant distribution account must have a zero balance to be eligible for deletion. Current balance for (%s, %s)=%f", asset.Code, asset.Issuer, assetBalance)
					httperror.BadRequest(errMsg, nil, nil).Render(w)
					return
				}
			}
		}
	}

	tnt, err = t.Manager.SoftDeleteTenantByID(ctx, tenantID)
	if err != nil {
		errMsg := fmt.Sprintf("Cannot delete tenant %s", tenantID)
		httperror.InternalError(ctx, errMsg, err, nil).Render(w)
		return
	}

	httpjson.RenderStatus(w, http.StatusOK, tnt, httpjson.JSON)
}

func (t TenantsHandler) SetDefault(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	if !t.SingleTenantMode {
		log.Ctx(ctx).Warnf("An attempt of set a default tenant was made but SINGLE_TENANT_MODE is set to `false`")
		httperror.Forbidden("Single Tenant Mode feature is disabled. Please, enable it before setting a tenant as default.", nil, nil).Render(rw)
		return
	}

	var reqBody validators.DefaultTenantRequest
	if err := httpdecode.DecodeJSON(req, &reqBody); err != nil {
		err = fmt.Errorf("decoding request body: %w", err)
		log.Ctx(ctx).Error(err)
		httperror.BadRequest("", err, nil).Render(rw)
		return
	}

	if err := reqBody.Validate(); err != nil {
		httperror.BadRequest("Invalid request body", nil, map[string]interface{}{"id": err.Error()}).Render(rw)
		return
	}

	defaultTnt, err := db.RunInTransactionWithResult(ctx, t.AdminDBConnectionPool, nil, func(dbTx db.DBTransaction) (*tenant.Tenant, error) {
		tnt, err := t.Manager.SetDefault(ctx, dbTx, reqBody.ID)
		if err != nil {
			return nil, fmt.Errorf("setting tenant id %s as default: %w", reqBody.ID, err)
		}

		return tnt, nil
	})
	if err != nil {
		if errors.Is(err, tenant.ErrTenantDoesNotExist) {
			httperror.NotFound(fmt.Sprintf("tenant id %s does not exist", reqBody.ID), err, nil).Render(rw)
		} else {
			httperror.InternalError(ctx, "", err, nil).Render(rw)
		}
		return
	}

	httpjson.Render(rw, defaultTnt, httpjson.JSON)
}
