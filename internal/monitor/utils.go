package monitor

import (
	"fmt"
	"net/http"
)

const (
	noHTTPStatus  = "0"
	successStatus = "success"
	errorStatus   = "error"
)

func ParseHTTPResponseStatus(resp *http.Response, reqErr error) (status, statusCode string) {
	if reqErr != nil {
		return errorStatus, noHTTPStatus
	}
	return successStatus, fmt.Sprint(resp.StatusCode)
}
