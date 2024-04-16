package httphandler

import (
	"net/http"

	"github.com/stellar/go/support/render/httpjson"
)

type HealthHandler struct {
	GitCommit string
	Version   string
}

func (h HealthHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	httpjson.RenderStatus(rw, http.StatusOK, map[string]string{
		"status":     "pass",
		"release_id": h.GitCommit,
		"version":    h.Version,
	}, httpjson.JSON)
}
