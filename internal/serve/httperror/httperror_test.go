package httperror

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/stellar/go/support/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewHTTPError(t *testing.T) {
	err := NewHTTPError(http.StatusBadRequest, "Bad request", nil, map[string]interface{}{
		"foo": "bar",
	})

	assert.Equal(t, http.StatusBadRequest, err.StatusCode)
	assert.Equal(t, "Bad request", err.Message)
	assert.Len(t, err.Extras, 1)
	assert.Equal(t, map[string]interface{}{"foo": "bar"}, err.Extras)
}

func TestNewHTTPError_returnOriginalErrIfNoNewInfoWasAdded(t *testing.T) {
	err := NewHTTPError(http.StatusBadRequest, "Bad request", nil, map[string]interface{}{
		"foo": "bar",
	})

	// if no new info was added, return original error
	newErr := NewHTTPError(http.StatusBadRequest, "", err, nil)
	assert.Equal(t, err, newErr)

	// return new error if the message changed
	newErr = NewHTTPError(http.StatusBadRequest, "Foo Bar Bad Request", err, nil)
	assert.NotEqual(t, err, newErr)

	// return new error if the status code changed
	newErr = NewHTTPError(http.StatusNotFound, "", err, nil)
	assert.NotEqual(t, err, newErr)

	// return new error if the extras have changed
	newErr = NewHTTPError(http.StatusBadRequest, "", err, map[string]interface{}{
		"foo2": "bar2",
	})
	assert.NotEqual(t, err, newErr)
}

func TestNotFound(t *testing.T) {
	originalErr := errors.New("original error")

	err := NotFound("", originalErr, map[string]interface{}{"foo": "not found"})
	assert.Equal(t, http.StatusNotFound, err.StatusCode)
	assert.Equal(t, "Resource not found.", err.Message)
	assert.Equal(t, originalErr, err.Err)
	assert.Equal(t, map[string]interface{}{"foo": "not found"}, err.Extras)

	err = NotFound("Foo Bar NotFound", nil, nil)
	assert.Equal(t, http.StatusNotFound, err.StatusCode)
	assert.Equal(t, "Foo Bar NotFound", err.Message)
	assert.Nil(t, err.Err)
	assert.Nil(t, err.Extras)
}

func TestBadRequest(t *testing.T) {
	originalErr := errors.New("original error")

	err := BadRequest("", originalErr, map[string]interface{}{"foo": "bad request"})
	assert.Equal(t, http.StatusBadRequest, err.StatusCode)
	assert.Equal(t, "The request was invalid in some way.", err.Message)
	assert.Equal(t, originalErr, err.Err)
	assert.Equal(t, map[string]interface{}{"foo": "bad request"}, err.Extras)

	err = BadRequest("Foo Bar BadRequest", nil, nil)
	assert.Equal(t, http.StatusBadRequest, err.StatusCode)
	assert.Equal(t, "Foo Bar BadRequest", err.Message)
	assert.Nil(t, err.Err)
	assert.Nil(t, err.Extras)
}

func TestUnauthorized(t *testing.T) {
	originalErr := errors.New("original error")

	err := Unauthorized("", originalErr, map[string]interface{}{"foo": "invalid token"})
	assert.Equal(t, http.StatusUnauthorized, err.StatusCode)
	assert.Equal(t, "Not authorized.", err.Message)
	assert.Equal(t, originalErr, err.Err)
	assert.Equal(t, map[string]interface{}{"foo": "invalid token"}, err.Extras)

	err = Unauthorized("Not authorized.", nil, nil)
	assert.Equal(t, http.StatusUnauthorized, err.StatusCode)
	assert.Equal(t, "Not authorized.", err.Message)
	assert.Nil(t, err.Err)
	assert.Nil(t, err.Extras)
}

func TestForbidden(t *testing.T) {
	originalErr := errors.New("original error")

	err := Forbidden("", originalErr, map[string]interface{}{"foo": "forbidden"})
	assert.Equal(t, http.StatusForbidden, err.StatusCode)
	assert.Equal(t, "You don't have permission to perform this action.", err.Message)
	assert.Equal(t, originalErr, err.Err)
	assert.Equal(t, map[string]interface{}{"foo": "forbidden"}, err.Extras)

	err = Forbidden("Foo Bar Forbidden", nil, nil)
	assert.Equal(t, http.StatusForbidden, err.StatusCode)
	assert.Equal(t, "Foo Bar Forbidden", err.Message)
	assert.Nil(t, err.Err)
	assert.Nil(t, err.Extras)
}

func TestInternalError(t *testing.T) {
	originalErr := errors.New("original error")
	ctx := context.Background()

	t.Run("internal error with default message", func(t *testing.T) {
		// set the logger to a buffer so we can check the error message
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)

		err := InternalError(ctx, "", originalErr, map[string]interface{}{"foo": "bad server error"})
		assert.Equal(t, http.StatusInternalServerError, err.StatusCode)
		assert.Equal(t, "An internal error occurred while processing this request.", err.Message)
		assert.Equal(t, originalErr, err.Err)
		assert.Equal(t, map[string]interface{}{"foo": "bad server error"}, err.Extras)

		// validate logs
		require.Contains(t, buf.String(), "An internal error occurred while processing this request.: original error")
	})

	t.Run("internal error with custom message", func(t *testing.T) {
		// set the logger to a buffer so we can check the error message
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)

		err := InternalError(ctx, "Foo Bar InternalError", originalErr, nil)
		assert.Equal(t, http.StatusInternalServerError, err.StatusCode)
		assert.Equal(t, "Foo Bar InternalError", err.Message)
		assert.Equal(t, originalErr, err.Err)
		assert.Nil(t, err.Extras)

		// validate logs
		require.Contains(t, buf.String(), "Foo Bar InternalError: original error")
	})

	t.Run("internal error without error", func(t *testing.T) {
		// set the logger to a buffer so we can check the error message
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)

		err := InternalError(ctx, "", nil, nil)
		assert.Equal(t, http.StatusInternalServerError, err.StatusCode)
		assert.Equal(t, "An internal error occurred while processing this request.", err.Message)
		assert.Nil(t, err.Err)
		assert.Nil(t, err.Extras)

		// validate logs
		require.Contains(t, buf.String(), "An internal error occurred while processing this request.:")
	})

	t.Run("internal error with custom ReportErrorFunc", func(t *testing.T) {
		// set the logger to a buffer so we can check the error message
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)

		reportErrorFunc := func(ctx context.Context, err error, msg string) {
			log.Error("reported with custom ReportFunc")
		}

		SetDefaultReportErrorFunc(reportErrorFunc)

		err := InternalError(ctx, "", originalErr, nil)
		assert.Equal(t, http.StatusInternalServerError, err.StatusCode)
		assert.Equal(t, "An internal error occurred while processing this request.", err.Message)
		assert.Equal(t, originalErr, err.Err)
		assert.Nil(t, err.Extras)

		// validate logs
		require.Contains(t, buf.String(), "reported with custom ReportFunc")
	})
}

func TestUnprocessableEntity(t *testing.T) {
	originalErr := errors.New("original error")

	err := UnprocessableEntity("", originalErr, map[string]interface{}{"foo": "invalid token"})
	assert.Equal(t, http.StatusUnprocessableEntity, err.StatusCode)
	assert.Equal(t, "Unprocessable entity.", err.Message)
	assert.Equal(t, originalErr, err.Err)
	assert.Equal(t, map[string]interface{}{"foo": "invalid token"}, err.Extras)

	err = UnprocessableEntity("Could not process your request.", nil, nil)
	assert.Equal(t, http.StatusUnprocessableEntity, err.StatusCode)
	assert.Equal(t, "Could not process your request.", err.Message)
	assert.Nil(t, err.Err)
	assert.Nil(t, err.Extras)
}

func TestNewHTTPError_json(t *testing.T) {
	httpErr := NewHTTPError(http.StatusAccepted, "Bad request", nil, map[string]interface{}{
		"foo": "bar",
	})

	gotJson, err := json.Marshal(httpErr)
	require.NoError(t, err)

	wantJson := `{
		"error": "Bad request",
		"extras": {
			"foo": "bar"
		}
	}`
	require.JSONEq(t, wantJson, string(gotJson))
}

type testError struct {
	Msg string
}

func (te *testError) Error() string {
	return te.Msg
}

func TestError_unwrap(t *testing.T) {
	wrappedError := testError{"wrapped error"}
	httpErr := NewHTTPError(http.StatusForbidden, "Bad request", &wrappedError, map[string]interface{}{
		"foo": "bar",
	})
	require.Equal(t, &wrappedError, httpErr.Unwrap())

	require.True(t, errors.Is(httpErr, &wrappedError))

	var e *testError
	require.True(t, errors.As(httpErr, &e))
	require.Equal(t, &wrappedError, e)
}
