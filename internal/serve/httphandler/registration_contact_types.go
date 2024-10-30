package httphandler

import (
	"net/http"

	"github.com/stellar/go/support/render/httpjson"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
)

type RegistrationContactTypesHandler struct{}

func (c RegistrationContactTypesHandler) Get(w http.ResponseWriter, r *http.Request) {
	httpjson.Render(w, data.AllRegistrationContactTypes(), httpjson.JSON)
}
