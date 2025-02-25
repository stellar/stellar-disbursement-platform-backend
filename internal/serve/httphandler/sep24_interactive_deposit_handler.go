package httphandler

import (
	"context"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
)

type SEP24InteractiveDepositHandler struct {
	App      fs.FS
	BasePath string
}

// ServeApp services the SEP-24 interactive deposit app.
func (h SEP24InteractiveDepositHandler) ServeApp(w http.ResponseWriter, r *http.Request) {
	// Extract the path relative to the base path
	// e.g. `/wallet-registration-fe/start` -> `/start`
	ctx := r.Context()
	rctx := chi.RouteContext(ctx)
	pathPrefix := strings.TrimSuffix(rctx.RoutePattern(), "/*")
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
	if isStaticAsset(path) {
		fileServer := http.StripPrefix(pathPrefix, http.FileServer(http.FS(subFS)))
		fileServer.ServeHTTP(w, r)
		return
	}

	// For all other paths, serve the SPA.
	serveReactApp(ctx, w, subFS)
}

// isStaticAsset determines if a path refers to a static asset.
func isStaticAsset(path string) bool {
	// Remove leading slash for processing
	path = strings.TrimPrefix(path, "/")

	// Check if path contains a directory separator
	if strings.Contains(path, "/") {
		// If it's in a subdirectory, it's a static asset
		return true
	}

	// If it's directly under the root and has a file extension, it's a static asset
	// Empty extensions (like "file.") are not considered valid
	return filepath.Ext(path) != ""
}

// serveReactApp serves the React SPA by delivering the index.html file.
func serveReactApp(ctx context.Context, w http.ResponseWriter, fileSystem fs.FS) {
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
