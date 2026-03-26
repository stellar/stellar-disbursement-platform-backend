package httphandler

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/stellar/go-stellar-sdk/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
)

// MaxRPCRequestBodySize is the maximum allowed size for RPC proxy request bodies (512 KB).
const MaxRPCRequestBodySize = 512 * 1024

// RPCProxyHandler proxies JSON-RPC requests to the underlying Stellar RPC instance, allowing embedded
// wallets and the SDP frontends to interact with the Stellar network.
type RPCProxyHandler struct {
	RPCUrl             string
	RPCAuthHeaderKey   string
	RPCAuthHeaderValue string
}

// ServeHTTP proxies RPC requests to the underlying RPC instance.
func (h RPCProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if h.RPCUrl == "" {
		httperror.InternalError(ctx, "RPC URL not configured", nil, nil).Render(w)
		return
	}

	target, err := url.Parse(h.RPCUrl)
	if err != nil {
		httperror.InternalError(ctx, "Invalid RPC URL", err, nil).Render(w)
		return
	}

	if r.Method != http.MethodPost {
		httperror.BadRequest("Only POST requests are allowed", nil, nil).Render(w)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, MaxRPCRequestBodySize)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		httperror.BadRequest("Failed to read request body", err, map[string]interface{}{
			"details": "request body too large or unreadable",
		}).Render(w)
		return
	}
	defer func(body io.ReadCloser) {
		if err := body.Close(); err != nil {
			log.Ctx(ctx).Warnf("Failed to close request body: %v", err)
		}
	}(r.Body)

	if len(body) == 0 {
		httperror.BadRequest("Request body cannot be empty", nil, nil).Render(w)
		return
	}

	r.Body = io.NopCloser(bytes.NewBuffer(body))

	proxy := httputil.NewSingleHostReverseProxy(target)

	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = target.Host
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.URL.Path = target.Path

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		if h.RPCAuthHeaderKey != "" && h.RPCAuthHeaderValue != "" {
			req.Header.Set(h.RPCAuthHeaderKey, h.RPCAuthHeaderValue)
		}
	}

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Ctx(ctx).Errorf("RPC proxy error: %v", err)
		httperror.InternalError(ctx, "Failed to proxy request to RPC", err, nil).Render(w)
	}

	// Remove CORS headers added by the RPC server to avoid conflicts with SDP's CORS settings
	proxy.ModifyResponse = func(resp *http.Response) error {
		resp.Header.Del("Access-Control-Allow-Origin")
		resp.Header.Del("Access-Control-Allow-Methods")
		resp.Header.Del("Access-Control-Allow-Headers")
		resp.Header.Del("Access-Control-Expose-Headers")
		resp.Header.Del("Access-Control-Allow-Credentials")
		return nil
	}

	proxy.ServeHTTP(w, r)
}
