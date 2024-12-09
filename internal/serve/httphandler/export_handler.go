package httphandler

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gocarina/gocsv"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
)

type ExportHandler struct {
	Models *data.Models
}

func (e ExportHandler) ExportDisbursements(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	validator := validators.NewDisbursementQueryValidator()
	queryParams := validator.ParseParametersFromRequest(r)

	if validator.HasErrors() {
		httperror.BadRequest("Request invalid", nil, validator.Errors).Render(rw)
		return
	}

	queryParams.Filters = validator.ValidateAndGetDisbursementFilters(queryParams.Filters)
	if validator.HasErrors() {
		httperror.BadRequest("Request invalid", nil, validator.Errors).Render(rw)
		return
	}

	disbursements, err := e.Models.Disbursements.GetAll(ctx, e.Models.DBConnectionPool, queryParams, data.QueryTypeSelectAll)
	if err != nil {
		httperror.InternalError(ctx, "Failed to get disbursements", err, nil).Render(rw)
		return
	}

	fileName := fmt.Sprintf("disbursements_%s.csv", time.Now().Format("2006-01-02-15-04-05"))
	rw.Header().Set("Content-Type", "text/csv")
	rw.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", fileName))

	if err := gocsv.Marshal(disbursements, rw); err != nil {
		httperror.InternalError(ctx, "Failed to write CSV", err, nil).Render(rw)
		return
	}
}
