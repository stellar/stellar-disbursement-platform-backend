package httphandler

import (
	"net/http"

	"github.com/stellar/go/support/render/httpjson"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
)

type CountriesHandler struct {
	Models *data.Models
}

// GetCountries returns a list of countries
func (c CountriesHandler) GetCountries(w http.ResponseWriter, r *http.Request) {
	countries, err := c.Models.Countries.GetAll(r.Context())
	if err != nil {
		ctx := r.Context()
		httperror.InternalError(ctx, "Cannot retrieve countries", err, nil).Render(w)
		return
	}
	httpjson.Render(w, countries, httpjson.JSON)
}
