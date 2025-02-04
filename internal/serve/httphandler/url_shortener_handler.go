package httphandler

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
)

type URLShortenerHandler struct {
	Models *data.Models
}

func (u URLShortenerHandler) HandleRedirect(w http.ResponseWriter, r *http.Request) {
	shortCode := chi.URLParam(r, "code")
	if shortCode == "" {
		httperror.BadRequest("missing short code", nil, nil).Render(w)
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

	if incrErr := u.Models.URLShortener.IncrementHits(ctx, shortCode); incrErr != nil {
		log.Ctx(ctx).Errorf("Failed to increment hits for %s: %v", shortCode, incrErr)
	}

	http.Redirect(w, r, originalURL, http.StatusMovedPermanently)
}
