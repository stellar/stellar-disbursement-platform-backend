package httphandler

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/stellar/go/support/render/httpjson"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/statistics"
)

type StatisticsHandler struct {
	DBConnectionPool db.DBConnectionPool
}

func (s StatisticsHandler) GetStatistics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	stats, err := statistics.CalculateStatistics(ctx, s.DBConnectionPool)
	if err != nil {
		httperror.InternalError(ctx, "Cannot calculate statistics", err, nil).Render(w)
		return
	}

	httpjson.RenderStatus(w, http.StatusOK, stats, httpjson.JSON)
}

func (s StatisticsHandler) GetStatisticsByDisbursement(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	disbursementID := chi.URLParam(r, "id")

	stats, err := statistics.CalculateStatisticsByDisbursement(ctx, s.DBConnectionPool, disbursementID)
	if err != nil {
		if errors.Is(statistics.ErrResourcesNotFound, err) {
			errorMsg := fmt.Sprintf("a disbursement with the id %s does not exist", disbursementID)
			httperror.NotFound(errorMsg, err, nil).Render(w)
			return
		} else {
			httperror.InternalError(ctx, "Cannot calculate statistics", err, nil).Render(w)
			return
		}
	}

	httpjson.RenderStatus(w, http.StatusOK, stats, httpjson.JSON)
}
