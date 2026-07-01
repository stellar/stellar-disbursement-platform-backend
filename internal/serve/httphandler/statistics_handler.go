package httphandler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/go-chi/chi/v5"
	"github.com/stellar/go-stellar-sdk/support/render/httpjson"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/statistics"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

type StatisticsHandler struct {
	DBConnectionPool db.DBConnectionPool
	Models           *data.Models
	AuthManager      auth.AuthManager
}

// GetStatistics returns tenant statistics, wallet-aware per the read taxonomy:
//   - X-Wallet-Id narrows to one wallet (404 outside the caller's scope — no disclosure)
//   - non-Owners without the header get their membership set aggregated
//   - Owners without the header get the tenant-wide aggregate (the "all wallets" view)
func (s StatisticsHandler) GetStatistics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	scope, scopeErr := resolveWalletReadScope(ctx, s.AuthManager, s.Models)
	if scopeErr != nil {
		scopeErr.Render(w)
		return
	}

	walletIDs := scope // nil for Owners (tenant-wide), membership set otherwise
	if headerWalletID := r.Header.Get(XWalletIDHeader); headerWalletID != "" {
		if scope != nil && !slices.Contains(scope, headerWalletID) {
			// Per the read-leakage rules, existence is never disclosed.
			httperror.NotFound("distribution wallet not found", nil, nil).Render(w)
			return
		}
		walletIDs = []string{headerWalletID}
	}

	stats, err := statistics.CalculateStatistics(ctx, s.DBConnectionPool, walletIDs)
	if err != nil {
		httperror.InternalError(ctx, "Cannot calculate statistics", err, nil).Render(w)
		return
	}

	httpjson.RenderStatus(w, http.StatusOK, stats, httpjson.JSON)
}

func (s StatisticsHandler) GetStatisticsByDisbursement(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	disbursementID := chi.URLParam(r, "id")

	// Membership-filtered visibility: the disbursement's stats follow its wallet.
	if httpErr := s.ensureDisbursementVisible(ctx, disbursementID); httpErr != nil {
		httpErr.Render(w)
		return
	}

	stats, err := statistics.CalculateStatisticsByDisbursement(ctx, s.DBConnectionPool, disbursementID)
	if err != nil {
		if errors.Is(err, statistics.ErrResourcesNotFound) {
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

func (s StatisticsHandler) ensureDisbursementVisible(ctx context.Context, disbursementID string) *httperror.HTTPError {
	disbursement, err := s.Models.Disbursements.Get(ctx, s.DBConnectionPool, disbursementID)
	if err != nil {
		if errors.Is(err, data.ErrRecordNotFound) {
			return httperror.NotFound(fmt.Sprintf("a disbursement with the id %s does not exist", disbursementID), err, nil)
		}
		return httperror.InternalError(ctx, "Cannot load disbursement", err, nil)
	}

	scope, scopeErr := resolveWalletReadScope(ctx, s.AuthManager, s.Models)
	if scopeErr != nil {
		return scopeErr
	}
	if !walletInReadScope(scope, disbursement.SourceWalletID) {
		return httperror.NotFound(fmt.Sprintf("a disbursement with the id %s does not exist", disbursementID), nil, nil)
	}
	return nil
}
