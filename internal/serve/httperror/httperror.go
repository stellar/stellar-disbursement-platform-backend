package httperror

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/stellar/go/support/log"
	"github.com/stellar/go/support/render/httpjson"
)

type HTTPError struct {
	StatusCode int    `json:"-"`
	Message    string `json:"error"`
	// Extras contains extra information about the error.
	Extras map[string]interface{} `json:"extras,omitempty"`
	// Err is an optional field that can be used to wrap the original error to pass it forward.
	Err error `json:"-"`
}

// ReportFunc is a function type used to report unexpected errors.
type ReportErrorFunc func(ctx context.Context, err error, msg string)

// ReportError is a struct type used to report unexpected errors.
type ReportError struct {
	reportErrorFunc ReportErrorFunc
}

// defaultReportFunc initiliaze defaultReportFunc with a default function.
var defaultReportErrorFunc = ReportError{
	reportErrorFunc: func(ctx context.Context, err error, msg string) {
		if msg != "" {
			err = fmt.Errorf("%s: %w", msg, err)
		}
		log.Ctx(ctx).WithStack(err).Errorf("%+v", err)
	},
}

// SetDefaultReportErrorFunc sets a new defaultReportErrorFunc to report unexpected errors.
func SetDefaultReportErrorFunc(fn ReportErrorFunc) {
	defaultReportErrorFunc.reportErrorFunc = fn
}

func (h *HTTPError) Error() string {
	return h.Message
}

func (e *HTTPError) Unwrap() error {
	return e.Err
}

func (e *HTTPError) Render(w http.ResponseWriter) {
	httpjson.RenderStatus(w, e.StatusCode, e, httpjson.JSON)
}

func NewHTTPError(statusCode int, msg string, originalErr error, extras map[string]interface{}) *HTTPError {
	if msg == "" && originalErr != nil && len(extras) == 0 {
		var hErr *HTTPError
		if errors.As(originalErr, &hErr) && (hErr.StatusCode == statusCode) {
			return hErr
		}
	}

	return &HTTPError{
		StatusCode: statusCode,
		Message:    msg,
		Extras:     extras,
		Err:        originalErr,
	}
}

func NotFound(msg string, originalErr error, extras map[string]interface{}) *HTTPError {
	if msg == "" {
		msg = "Resource not found."
	}
	return NewHTTPError(http.StatusNotFound, msg, originalErr, extras)
}

func Conflict(msg string, originalErr error, extras map[string]interface{}) *HTTPError {
	if msg == "" {
		msg = "The resource already exists."
	}
	return NewHTTPError(http.StatusConflict, msg, originalErr, extras)
}

func BadRequest(msg string, originalErr error, extras map[string]interface{}) *HTTPError {
	if msg == "" {
		msg = "The request was invalid in some way."
	}
	return NewHTTPError(http.StatusBadRequest, msg, originalErr, extras)
}

func NotImplemented(msg string, originalErr error, extras map[string]interface{}) *HTTPError {
	if msg == "" {
		msg = "This feature is not implemented yet."
	}
	return NewHTTPError(http.StatusNotImplemented, msg, originalErr, extras)
}

func Unauthorized(msg string, originalErr error, extras map[string]interface{}) *HTTPError {
	if msg == "" {
		msg = "Not authorized."
	}
	return NewHTTPError(http.StatusUnauthorized, msg, originalErr, extras)
}

func Forbidden(msg string, originalErr error, extras map[string]interface{}) *HTTPError {
	if msg == "" {
		msg = "You don't have permission to perform this action."
	}
	return NewHTTPError(http.StatusForbidden, msg, originalErr, extras)
}

func InternalError(ctx context.Context, msg string, originalErr error, extras map[string]interface{}) *HTTPError {
	if msg == "" {
		msg = "An internal error occurred while processing this request."
	}
	defaultReportErrorFunc.reportErrorFunc(ctx, originalErr, msg)
	return NewHTTPError(http.StatusInternalServerError, msg, originalErr, extras)
}

func UnprocessableEntity(msg string, originalErr error, extras map[string]interface{}) *HTTPError {
	if msg == "" {
		msg = "Unprocessable entity."
	}
	return NewHTTPError(http.StatusUnprocessableEntity, msg, originalErr, extras)
}
