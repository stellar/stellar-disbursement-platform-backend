package httphandler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
)

func Test_SEP45Handler_GetChallenge(t *testing.T) {
	t.Run("successful challenge creation", func(t *testing.T) {
		mockService := services.NewMockSEP45Service(t)
		handler := SEP45Handler{SEP45Service: mockService}

		expected := &services.SEP45ChallengeResponse{AuthorizationEntries: "abc", NetworkPassphrase: "Test"}
		clientDomain := "wallet.example.com"

		mockService.
			On("CreateChallenge", mock.Anything, services.SEP45ChallengeRequest{
				Account:      "CCYU2FUIMK23K34U3SWCN2O2JVI6JBGUGQUILYK7GRPCIDABVVTCS7R4",
				HomeDomain:   "example.com",
				ClientDomain: &clientDomain,
			}).
			Return(expected, nil)

		r := chi.NewRouter()
		r.Get("/sep45/auth", handler.GetChallenge)

		req := httptest.NewRequest(http.MethodGet, "/sep45/auth?account=CCYU2FUIMK23K34U3SWCN2O2JVI6JBGUGQUILYK7GRPCIDABVVTCS7R4&home_domain=example.com&client_domain="+clientDomain, nil)
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Header().Get("Content-Type"), "application/json")

		var resp services.SEP45ChallengeResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, expected.AuthorizationEntries, resp.AuthorizationEntries)
		assert.Equal(t, expected.NetworkPassphrase, resp.NetworkPassphrase)
	})

	t.Run("validation error", func(t *testing.T) {
		mockService := services.NewMockSEP45Service(t)
		handler := SEP45Handler{SEP45Service: mockService}

		r := chi.NewRouter()
		r.Get("/sep45/auth", handler.GetChallenge)

		req := httptest.NewRequest(http.MethodGet, "/sep45/auth?account=", nil)
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("service error", func(t *testing.T) {
		mockService := services.NewMockSEP45Service(t)
		handler := SEP45Handler{SEP45Service: mockService}

		clientDomain := "wallet.example.com"
		mockService.
			On("CreateChallenge", mock.Anything, services.SEP45ChallengeRequest{
				Account:      "CCYU2FUIMK23K34U3SWCN2O2JVI6JBGUGQUILYK7GRPCIDABVVTCS7R4",
				HomeDomain:   "example.com",
				ClientDomain: &clientDomain,
			}).
			Return((*services.SEP45ChallengeResponse)(nil), assert.AnError)

		r := chi.NewRouter()
		r.Get("/sep45/auth", handler.GetChallenge)

		req := httptest.NewRequest(http.MethodGet, "/sep45/auth?account=CCYU2FUIMK23K34U3SWCN2O2JVI6JBGUGQUILYK7GRPCIDABVVTCS7R4&home_domain=example.com&client_domain="+clientDomain, nil)
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func Test_SEP45Handler_PostChallenge(t *testing.T) {
	t.Run("valid json request", func(t *testing.T) {
		mockService := services.NewMockSEP45Service(t)
		handler := SEP45Handler{SEP45Service: mockService}

		mockService.
			On("ValidateChallenge", mock.Anything, services.SEP45ValidationRequest{AuthorizationEntries: "encoded"}).
			Return(&services.SEP45ValidationResponse{Token: "jwt-token"}, nil)

		r := chi.NewRouter()
		r.Post("/sep45/auth", handler.PostChallenge)

		body := bytes.NewBufferString(`{"authorization_entries":"encoded"}`)
		req := httptest.NewRequest(http.MethodPost, "/sep45/auth", body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp services.SEP45ValidationResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, "jwt-token", resp.Token)
	})

	t.Run("valid form request", func(t *testing.T) {
		mockService := services.NewMockSEP45Service(t)
		handler := SEP45Handler{SEP45Service: mockService}

		mockService.
			On("ValidateChallenge", mock.Anything, services.SEP45ValidationRequest{AuthorizationEntries: "encoded"}).
			Return(&services.SEP45ValidationResponse{Token: "jwt-token"}, nil)

		r := chi.NewRouter()
		r.Post("/sep45/auth", handler.PostChallenge)

		req := httptest.NewRequest(http.MethodPost, "/sep45/auth", bytes.NewBufferString("authorization_entries=encoded"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Header().Get("Content-Type"), "application/json")

		var resp services.SEP45ValidationResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, "jwt-token", resp.Token)
	})

	t.Run("unsupported content type", func(t *testing.T) {
		mockService := services.NewMockSEP45Service(t)
		handler := SEP45Handler{SEP45Service: mockService}

		r := chi.NewRouter()
		r.Post("/sep45/auth", handler.PostChallenge)

		req := httptest.NewRequest(http.MethodPost, "/sep45/auth", bytes.NewBufferString("authorization_entries=encoded"))
		req.Header.Set("Content-Type", "text/plain")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("missing authorization entries", func(t *testing.T) {
		mockService := services.NewMockSEP45Service(t)
		handler := SEP45Handler{SEP45Service: mockService}

		r := chi.NewRouter()
		r.Post("/sep45/auth", handler.PostChallenge)

		req := httptest.NewRequest(http.MethodPost, "/sep45/auth", bytes.NewBufferString("{}"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("validation failure", func(t *testing.T) {
		mockService := services.NewMockSEP45Service(t)
		handler := SEP45Handler{SEP45Service: mockService}

		mockService.
			On("ValidateChallenge", mock.Anything, services.SEP45ValidationRequest{AuthorizationEntries: "bad"}).
			Return((*services.SEP45ValidationResponse)(nil), fmt.Errorf("%w: bad auth", services.ErrSEP45Validation))

		r := chi.NewRouter()
		r.Post("/sep45/auth", handler.PostChallenge)

		req := httptest.NewRequest(http.MethodPost, "/sep45/auth", bytes.NewBufferString(`{"authorization_entries":"bad"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("internal error from service returns 500", func(t *testing.T) {
		mockService := services.NewMockSEP45Service(t)
		handler := SEP45Handler{SEP45Service: mockService}

		mockService.
			On("ValidateChallenge", mock.Anything, services.SEP45ValidationRequest{AuthorizationEntries: "boom"}).
			Return((*services.SEP45ValidationResponse)(nil), assert.AnError)

		r := chi.NewRouter()
		r.Post("/sep45/auth", handler.PostChallenge)

		req := httptest.NewRequest(http.MethodPost, "/sep45/auth", bytes.NewBufferString(`{"authorization_entries":"boom"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}
