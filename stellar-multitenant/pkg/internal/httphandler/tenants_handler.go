package httphandler

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/stellar/go/support/render/httpjson"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

type TenantsHandler struct {
	Manager *tenant.Manager
}

func (t TenantsHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	tnts, err := t.Manager.GetAllTenants(ctx)
	if err != nil {
		httperror.InternalError(ctx, "Cannot get tenants", err, nil).Render(w)
		return
	}

	httpjson.RenderStatus(w, http.StatusOK, tnts, httpjson.JSON)
}

func (t TenantsHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := chi.URLParam(r, "id")

	tnt, err := t.Manager.GetTenantByID(ctx, tenantID)
	if err != nil {
		if errors.Is(tenant.ErrTenantDoesNotExist, err) {
			errorMsg := fmt.Sprintf("a tenant with the id %s does not exist", tenantID)
			httperror.NotFound(errorMsg, err, nil).Render(w)
			return
		}
		httperror.InternalError(ctx, "Cannot get tenant by ID", err, nil).Render(w)
		return
	}

	httpjson.RenderStatus(w, http.StatusOK, tnt, httpjson.JSON)
}

func (t TenantsHandler) GetByName(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantName := chi.URLParam(r, "name")

	tnt, err := t.Manager.GetTenantByName(ctx, tenantName)
	if err != nil {
		if errors.Is(tenant.ErrTenantDoesNotExist, err) {
			errorMsg := fmt.Sprintf("a tenant with the name %s does not exist", tenantName)
			httperror.NotFound(errorMsg, err, nil).Render(w)
			return
		}
		httperror.InternalError(ctx, "Cannot get tenant by ID", err, nil).Render(w)
		return
	}

	httpjson.RenderStatus(w, http.StatusOK, tnt, httpjson.JSON)
}
