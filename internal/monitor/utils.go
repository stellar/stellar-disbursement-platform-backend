package monitor

import (
	"fmt"
	"net/http"
)

const (
	noHTTPStatus  = "0"
	successStatus = "success"
	errorStatus   = "error"
	unknownMethod = "unknown"
)

func ParseHTTPResponseStatus(resp *http.Response, reqErr error) (status, statusCode string) {
	if reqErr != nil {
		return errorStatus, noHTTPStatus
	}
	return successStatus, fmt.Sprint(resp.StatusCode)
}

// sanitizeHTTPMethod bounds the cardinality of the `method` metric label.
// Standard methods pass through unchanged, the rest collapse to a single constant.
func sanitizeHTTPMethod(method string) string {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut,
		http.MethodPatch, http.MethodDelete, http.MethodConnect,
		http.MethodOptions, http.MethodTrace:
		return method
	default:
		return unknownMethod
	}
}
