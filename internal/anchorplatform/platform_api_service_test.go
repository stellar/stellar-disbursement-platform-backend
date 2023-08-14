package anchorplatform

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_UpdateAnchorTransactions(t *testing.T) {
	httpClientMock := httpclient.HttpClientMock{}
	anchorPlatformAPIService, err := NewAnchorPlatformAPIService(&httpClientMock, "http://mock_anchor.com/", "")
	require.NoError(t, err)
	ctx := context.Background()

	t.Run("error calling httpClient.Do", func(t *testing.T) {
		httpClientMock.On("Do", mock.AnythingOfType("*http.Request")).Return(nil, fmt.Errorf("error calling the request")).Once()
		transaction := &Transaction{
			TransactionValues: TransactionValues{
				ID:                 "test-transaction-id",
				Status:             "pending_anchor",
				Sep:                "24",
				Kind:               "deposit",
				DestinationAccount: "stellar_address",
				Memo:               "stellar_memo",
				KYCVerified:        true,
			},
		}
		err := anchorPlatformAPIService.UpdateAnchorTransactions(ctx, []Transaction{*transaction})
		require.EqualError(t, err, "error making request to anchor platform: error calling the request")

		httpClientMock.AssertExpectations(t)
	})

	t.Run("error trying to update transactions on anchor platform", func(t *testing.T) {
		transactionResponse := `{The 'id' of the transaction first determined to be invalid.}`
		response := &http.Response{
			Body:       io.NopCloser(strings.NewReader(transactionResponse)),
			StatusCode: http.StatusBadRequest,
		}
		httpClientMock.On("Do", mock.Anything).Return(response, nil).Once()

		transaction := &Transaction{
			TransactionValues: TransactionValues{
				ID:                 "test-transaction-id",
				Status:             "pending_anchor",
				Sep:                "24",
				Kind:               "deposit",
				DestinationAccount: "stellar_address",
				Memo:               "stellar_memo",
				KYCVerified:        true,
			},
		}
		err := anchorPlatformAPIService.UpdateAnchorTransactions(ctx, []Transaction{*transaction})
		require.EqualError(t, err, "error updating transaction on anchor platform, response.StatusCode: 400")

		httpClientMock.AssertExpectations(t)
	})

	t.Run("succesfully update transaction on anchor platform", func(t *testing.T) {
		transactionResponse := `{
			"transaction":{
				"id": "test-transaction-id",
				"status": "pending_anchor",
				"sep": "24",
				"kind": "deposit",
				"destination_account": "stellar_address",
				"memo": "stellar_memo"
				"kyc_verified": true,
			}
		}`
		response := &http.Response{
			Body:       io.NopCloser(strings.NewReader(transactionResponse)),
			StatusCode: http.StatusOK,
		}
		httpClientMock.On("Do", mock.Anything).Return(response, nil).Once()

		transaction := &Transaction{
			TransactionValues: TransactionValues{
				ID:                 "test-transaction-id",
				Status:             "pending_anchor",
				Sep:                "24",
				Kind:               "deposit",
				DestinationAccount: "stellar_address",
				Memo:               "stellar_memo",
				KYCVerified:        true,
			},
		}
		err := anchorPlatformAPIService.UpdateAnchorTransactions(ctx, []Transaction{*transaction})
		require.NoError(t, err)

		httpClientMock.AssertExpectations(t)
	})
}

func Test_GetJWTToken(t *testing.T) {
	t.Run("returns ErrJWTSecretNotSet when a JWT secret is not set", func(t *testing.T) {
		apService := AnchorPlatformAPIService{}
		transactions := []Transaction{
			{TransactionValues{ID: "1"}},
			{TransactionValues{ID: "2"}},
		}
		token, err := apService.GetJWTToken(transactions)
		require.ErrorIs(t, err, ErrJWTManagerNotSet)
		require.Empty(t, token)
	})

	t.Run("returns token successfully 🎉", func(t *testing.T) {
		jwtManager, err := NewJWTManager("1234567890ab", 5000)
		require.NoError(t, err)

		apService := AnchorPlatformAPIService{jwtManager: jwtManager}
		transactions := []Transaction{
			{TransactionValues{ID: "1"}},
			{TransactionValues{ID: "2"}},
		}
		token, err := apService.GetJWTToken(transactions)
		require.NoError(t, err)
		require.NotEmpty(t, token)

		// verify the token
		claims, err := jwtManager.ParseDefaultTokenClaims(token)
		require.NoError(t, err)
		assert.Nil(t, claims.Valid())
		assert.Equal(t, "1,2", claims.ID)
		assert.Equal(t, "stellar-disbursement-platform-backend", claims.Subject)
	})
}
