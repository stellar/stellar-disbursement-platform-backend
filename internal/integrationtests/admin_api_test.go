package integrationtests

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	httpclientMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpclient/mocks"
)

func Test_AdminAPIIntegrationTests_CreateTenant(t *testing.T) {
	httpClientMock := httpclientMocks.HTTPClientMock{}

	aa := AdminAPIIntegrationTests{
		HTTPClient:      &httpClientMock,
		AdminAPIBaseURL: "http://localhost:8083",
		AccountID:       "accountId",
		APIKey:          "apiKey",
	}

	ctx := context.Background()

	t.Run("error calling httpClient.Do", func(t *testing.T) {
		httpClientMock.
			On("Do", mock.AnythingOfType("*http.Request")).
			Return(nil, fmt.Errorf("error calling the request")).
			Once()

		at, err := aa.CreateTenant(ctx, CreateTenantRequest{})
		require.EqualError(t, err, "making request to create tenant: error calling the request")
		assert.Empty(t, at)
	})

	t.Run("error trying to login to admin API", func(t *testing.T) {
		loginResponse := `{Invalid credentials.}`
		response := &http.Response{
			Body:       io.NopCloser(strings.NewReader(loginResponse)),
			StatusCode: http.StatusUnauthorized,
		}
		httpClientMock.On("Do", mock.AnythingOfType("*http.Request")).Return(response, nil).Once()

		at, err := aa.CreateTenant(ctx, CreateTenantRequest{})
		require.EqualError(t, err, "unexpected status code when creating tenant: 401")
		assert.Empty(t, at)
	})

	t.Run("error invalid response body", func(t *testing.T) {
		response := &http.Response{
			Body:       io.NopCloser(strings.NewReader(`invalid response body`)),
			StatusCode: http.StatusCreated,
		}
		httpClientMock.
			On("Do", mock.AnythingOfType("*http.Request")).
			Return(response, nil).
			Once()

		at, err := aa.CreateTenant(ctx, CreateTenantRequest{})
		require.ErrorContains(t, err, "decoding response when creating tenant")
		assert.Empty(t, at)
	})

	t.Run("successfully creating tenant", func(t *testing.T) {
		response := &http.Response{
			Body:       io.NopCloser(strings.NewReader(`{"id":"id"}`)),
			StatusCode: http.StatusCreated,
		}
		httpClientMock.
			On("Do", mock.AnythingOfType("*http.Request")).
			Return(response, nil).
			Once()

		at, err := aa.CreateTenant(ctx, CreateTenantRequest{})
		require.NoError(t, err)
		assert.Equal(t, "id", at.ID)
	})

	httpClientMock.AssertExpectations(t)
}

func Test_AdminAPIIntegrationTests_AuthHeader(t *testing.T) {
	aa := AdminAPIIntegrationTests{
		AccountID: "accountId",
		APIKey:    "apiKey",
	}

	authHeader := aa.AuthHeader()
	assert.Equal(t, "Basic YWNjb3VudElkOmFwaUtleQ==", authHeader)
}
