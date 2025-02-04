package httphandler

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

type URLShortenerHandler struct {
	Models *data.Models
}

func (u URLShortenerHandler) HandleRedirect(w http.ResponseWriter, r *http.Request) {
	shortCode := strings.TrimSpace(chi.URLParam(r, "code"))
	if shortCode == "" {
		httperror.BadRequest("Missing short code", nil, nil).Render(w)
		return
	}

	ctx := r.Context()
	originalURL, err := u.Models.URLShortener.GetOriginalURL(ctx, shortCode)
	if err != nil {
		if errors.Is(err, data.ErrRecordNotFound) {
			httperror.NotFound("Short URL not found", err, nil).Render(w)
		} else {
			httperror.InternalError(ctx, "Error retrieving URL", err, nil).Render(w)
		}
		return
	}

	// Increment hits in a separate go routine to avoid blocking the request.
	go u.incrementHits(ctx, shortCode)

	http.Redirect(w, r, originalURL, http.StatusMovedPermanently)
}

func (u URLShortenerHandler) incrementHits(mainCtx context.Context, shortCode string) {
	currentTenant, err := tenant.GetTenantFromContext(mainCtx)
	if err != nil {
		log.Ctx(mainCtx).Errorf("Failed to get tenant from context: %v", err)
		return
	}

	incrementCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ctx := tenant.SaveTenantInContext(incrementCtx, currentTenant)

	if err = u.Models.URLShortener.IncrementHits(ctx, shortCode); err != nil {
		log.Ctx(ctx).Errorf("Failed to increment hits for %s: %v", shortCode, err)
	}
}
