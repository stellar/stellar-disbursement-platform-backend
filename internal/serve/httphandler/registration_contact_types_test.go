package httphandler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_RegistrationContactTypesHandler_Get(t *testing.T) {
	h := RegistrationContactTypesHandler{}

	rr := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "/receiver-contact-types", nil)
	require.NoError(t, err)
	http.HandlerFunc(h.Get).ServeHTTP(rr, req)
	resp := rr.Result()
	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	expectedJSON := `[
		"EMAIL",
		"PHONE_NUMBER",
		"EMAIL_AND_WALLET_ADDRESS",
		"PHONE_NUMBER_AND_WALLET_ADDRESS"
	]`
	assert.JSONEq(t, expectedJSON, string(respBody))
}
