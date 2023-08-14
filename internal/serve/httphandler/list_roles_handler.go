package httphandler

import (
	"net/http"

	"github.com/stellar/go/support/render/httpjson"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
)

type ListRolesHandler struct{}

// GetRoles retrieves all the users roles available
func (h ListRolesHandler) GetRoles(rw http.ResponseWriter, req *http.Request) {
	roles := map[string][]data.UserRole{"roles": data.GetAllRoles()}
	httpjson.Render(rw, roles, httpjson.JSON)
}
