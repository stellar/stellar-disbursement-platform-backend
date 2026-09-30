package monitor

import (
	"fmt"
	"net/http"
	"strings"
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

// SanitizeHTTPMethod bounds the cardinality of the `method` metric label.
// This collapses any unrecognized method to a single constant
func SanitizeHTTPMethod(method string) string {
	switch strings.ToUpper(method) {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut,
		http.MethodPatch, http.MethodDelete, http.MethodConnect,
		http.MethodOptions, http.MethodTrace:
		return strings.ToUpper(method)
	default:
		return unknownMethod
	}
}
