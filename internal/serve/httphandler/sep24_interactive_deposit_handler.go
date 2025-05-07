package httphandler

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/anchorplatform"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

type SEP24InteractiveDepositHandler struct {
	App      fs.FS
	BasePath string
}

// ServeApp services the SEP-24 interactive deposit app.
func (h SEP24InteractiveDepositHandler) ServeApp(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	routeCtx := chi.RouteContext(ctx)

	// Extract the path relative to the base path
	// e.g. `/wallet-registration/start` -> `/start`
	pathPrefix := strings.TrimSuffix(routeCtx.RoutePattern(), "/*")
	path := strings.TrimPrefix(r.URL.Path, pathPrefix)

	// Clean up path for processing
	if path == "" {
		path = "/"
	}

	// Don't allow directory listings.
	if path != "/" && path[len(path)-1] == '/' {
		http.NotFound(w, r)
		return
	}

	// Create a sub-filesystem from the app's base path to serve the app.
	subFS, err := fs.Sub(h.App, h.BasePath)
	if err != nil {
		httperror.InternalError(ctx, "Could not render Registration Page", err, nil).Render(w)
		return
	}

	// If it's a static asset, serve it directly.
	if utils.IsStaticAsset(path) {
		fileServer := http.StripPrefix(pathPrefix, http.FileServer(http.FS(subFS)))
		fileServer.ServeHTTP(w, r)
		return
	}

	// For all other paths, serve the SPA.
	serveReactApp(ctx, r.URL, w, subFS)
}

// serveReactApp serves the React SPA by delivering the index.html file.
func serveReactApp(ctx context.Context, reqURL *url.URL, w http.ResponseWriter, fileSystem fs.FS) {
	// Authentication and authorization
	sep24Claims := anchorplatform.GetSEP24Claims(ctx)
	if sep24Claims == nil {
		err := fmt.Errorf("no SEP-24 claims found in the request context")
		log.Ctx(ctx).Error(err)
		httperror.Unauthorized("", err, nil).Render(w)
		return
	}

	if token := reqURL.Query().Get("token"); token == "" {
		err := fmt.Errorf("no token was provided in the request")
		log.Ctx(ctx).Error(err)
		httperror.Unauthorized("", err, nil).Render(w)
		return
	}

	if err := sep24Claims.Valid(); err != nil {
		err = fmt.Errorf("SEP-24 claims are invalid: %w", err)
		log.Ctx(ctx).Error(err)
		httperror.Unauthorized("", err, nil).Render(w)
		return
	}

	// Render the index.html file
	content, err := fs.ReadFile(fileSystem, "index.html")
	if err != nil {
		httperror.InternalError(ctx, "Could not render Registration Page", err, nil).Render(w)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err = w.Write(content); err != nil {
		// Too late to return an error to the client, so just log it.
		log.Ctx(ctx).Errorf("Error writing response body for SEP-24 registration page: %v", err)
	}
}
