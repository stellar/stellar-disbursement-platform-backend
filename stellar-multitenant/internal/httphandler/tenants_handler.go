package httphandler

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/stellar/go/support/http/httpdecode"
	"github.com/stellar/go/support/log"
	"github.com/stellar/go/support/render/httpjson"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/internal/provisioning"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/internal/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

type TenantsHandler struct {
	Manager               *tenant.Manager
	Models                *data.Models
	ProvisioningManager   *provisioning.Manager
	NetworkType           utils.NetworkType
	AdminDBConnectionPool db.DBConnectionPool
	SingleTenantMode      bool
}

func (t TenantsHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	tnts, err := t.Manager.GetAllTenants(ctx, &tenant.QueryParams{
		Filters: map[tenant.FilterKey]interface{}{
			tenant.FilterKeyOutStatus: []tenant.TenantStatus{tenant.DeactivatedTenantStatus},
		},
		SortBy:    data.SortFieldName,
		SortOrder: data.SortOrderASC,
	})
	if err != nil {
		httperror.InternalError(ctx, "Cannot get tenants", err, nil).Render(w)
		return
	}

	httpjson.RenderStatus(w, http.StatusOK, tnts, httpjson.JSON)
}

func (t TenantsHandler) GetByIDOrName(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	arg := chi.URLParam(r, "arg")

	tnt, err := t.Manager.GetTenantByIDOrName(ctx, arg, &tenant.QueryParams{
		Filters: map[tenant.FilterKey]interface{}{
			tenant.FilterKeyOutStatus: []tenant.TenantStatus{tenant.DeactivatedTenantStatus},
		},
	})
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

	tnt, err := h.ProvisioningManager.ProvisionNewTenant(
		ctx, reqBody.Name, reqBody.OwnerFirstName,
		reqBody.OwnerLastName, reqBody.OwnerEmail, reqBody.OrganizationName,
		reqBody.SDPUIBaseURL, string(h.NetworkType),
	)
	if err != nil {
		if errors.Is(err, tenant.ErrDuplicatedTenantName) {
			httperror.BadRequest("Tenant name already exists", err, nil).Render(rw)
			return
		}
		httperror.InternalError(ctx, "Could not provision a new tenant", err, nil).Render(rw)
		return
	}

	tnt, err = h.Manager.UpdateTenantConfig(ctx, &tenant.TenantUpdate{
		ID:      tnt.ID,
		BaseURL: &reqBody.BaseURL,
	})
	if err != nil {
		httperror.InternalError(ctx, "Could not update tenant config", err, nil).Render(rw)
		return
	}

	httpjson.RenderStatus(rw, http.StatusCreated, tnt, httpjson.JSON)
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

	if reqBody.Status != nil {
		tnt, err := t.Manager.GetTenantByID(ctx, tenantID, nil)
		if err != nil {
			httperror.InternalError(ctx, "cannot retrieve tenant by id", err, nil).Render(w)
		}

		reqStatus := *reqBody.Status
		// if attempting to deactivate tenant, need to check for a few conditions such as
		// 1. whether tenant is already deactivated
		// 2. whether there are any payments not in a terminal state
		if reqStatus == tenant.DeactivatedTenantStatus {
			if tnt.Status == tenant.DeactivatedTenantStatus {
				httpjson.RenderStatus(w, http.StatusNotModified, tnt, httpjson.JSON)
				return
			}

			indeterminatePaymentsCount, err := t.Models.Payment.Count(ctx, &data.QueryParams{
				Filters: map[data.FilterKey]interface{}{
					data.FilterKeyStatus: []data.PaymentStatus{
						data.DraftPaymentStatus,
						data.ReadyPaymentStatus,
						data.PendingPaymentStatus,
						data.PausedPaymentStatus,
					},
				},
			}, t.Models.DBConnectionPool)
			if err != nil {
				httperror.InternalError(ctx, "cannot retrieve payments for tenant", err, nil).Render(w)
				return
			}

			if indeterminatePaymentsCount != 0 {
				httperror.BadRequest("cannot deactivate tenant", errors.New(""), nil).Render(w)
				return
			}
		} else if reqStatus == tenant.ActivatedTenantStatus {
			if tnt.Status == tenant.ActivatedTenantStatus {
				httpjson.RenderStatus(w, http.StatusNotModified, tnt, httpjson.JSON)
				return
			}

			if tnt.Status != tenant.DeactivatedTenantStatus {
				httperror.BadRequest("cannot activate tenant that is not deactivated", nil, nil).Render(w)
				return
			}
		} else {
			httperror.BadRequest("cannot perform update on tenant to status in request", nil, nil).Render(w)
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
