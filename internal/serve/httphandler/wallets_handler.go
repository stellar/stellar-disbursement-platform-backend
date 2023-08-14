package httphandler

import (
	"net/http"

	"github.com/stellar/go/support/render/httpjson"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
)

type WalletsHandler struct {
	Models *data.Models
}

// GetWallets returns a list of wallets
func (c WalletsHandler) GetWallets(w http.ResponseWriter, r *http.Request) {
	countries, err := c.Models.Wallets.GetAll(r.Context())
	if err != nil {
		httperror.InternalError(r.Context(), "Cannot retrieve list of wallets", err, nil).Render(w)
		return
	}
	httpjson.Render(w, countries, httpjson.JSON)
}
