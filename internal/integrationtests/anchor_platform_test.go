package integrationtests

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stellar/go/keypair"
	"github.com/stellar/go/txnbuild"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_StartChallengeTransaction(t *testing.T) {
	httpClientMock := httpclient.HttpClientMock{}
	receiverAccountID := "GDJNLIFC2JTGKTD4LA4D77TSEGMQLZKBIEXMMJT64AEWWVYC5JKJHH2X"

	serverPublicKey := "GD57H5NAK3NFZVR66OGPBAV4FUFUQXQTPOQTSKFLW63SVQVQ4FSQAXMA"
	serverPrivateKey := "SBG2NGVW7VYIZDK4R775UXNRZUODJBS3N3H6ICKKAAMXUSWBOHUXETE4"

	ap := AnchorPlatformIntegrationTests{
		HttpClient:               &httpClientMock,
		AnchorPlatformBaseSepURL: "http://mock_anchor.com/",
		ReceiverAccountPublicKey: receiverAccountID,
		Sep10SigningPublicKey:    serverPublicKey,
	}

	t.Run("error calling httpClient.Do", func(t *testing.T) {
		httpClientMock.On("Do", mock.AnythingOfType("*http.Request")).Return(nil, fmt.Errorf("error calling the request")).Once()
		ct, err := ap.StartChallengeTransaction()
		require.EqualError(t, err, "error making request to anchor platform get AUTH: error calling the request")
		assert.Empty(t, ct)

		httpClientMock.AssertExpectations(t)
	})

	t.Run("error trying to create challenge transaction on anchor platform", func(t *testing.T) {
		transactionResponse := `{Error creating challenge transaction}`
		response := &http.Response{
			Body:       io.NopCloser(strings.NewReader(transactionResponse)),
			StatusCode: http.StatusBadRequest,
		}
		httpClientMock.On("Do", mock.AnythingOfType("*http.Request")).Return(response, nil).Once()

		ct, err := ap.StartChallengeTransaction()
		require.EqualError(t, err, "error creating challenge transaction on anchor platform")
		assert.Empty(t, ct)

		httpClientMock.AssertExpectations(t)
	})

	t.Run("error invalid response body", func(t *testing.T) {
		transactionResponse := ``
		response := &http.Response{
			Body:       io.NopCloser(strings.NewReader(transactionResponse)),
			StatusCode: http.StatusOK,
		}
		httpClientMock.On("Do", mock.AnythingOfType("*http.Request")).Return(response, nil).Once()

		ct, err := ap.StartChallengeTransaction()
		require.EqualError(t, err, "error decoding response body: EOF")
		assert.Empty(t, ct)

		httpClientMock.AssertExpectations(t)
	})

	t.Run("error reading challenge transaction on anchor platform", func(t *testing.T) {
		invalidServerPrivateKey := "SAB4UJB2NCL5SUJUBDNOVAN6ILOULGDB3G6TTBZ32TERX2N454ORSUIY"
		mockCT, err := txnbuild.BuildChallengeTx(
			invalidServerPrivateKey,
			receiverAccountID,
			"localhost:8080",
			"localhost:8080",
			"Test SDF Network ; September 2015",
			time.Second*300,
			nil,
		)
		require.NoError(t, err)

		transactionStr, err := mockCT.Base64()
		require.NoError(t, err)

		transactionResponse := fmt.Sprintf(`{
			"transaction": %q,
			"network_passphrase": "Test SDF Network ; September 2015"
		}`, transactionStr)

		response := &http.Response{
			Body:       io.NopCloser(strings.NewReader(transactionResponse)),
			StatusCode: http.StatusOK,
		}
		httpClientMock.On("Do", mock.AnythingOfType("*http.Request")).Return(response, nil).Once()

		ct, err := ap.StartChallengeTransaction()
		require.EqualError(t, err, "error reading challenge transaction: transaction source account is not equal to server's account")
		assert.Empty(t, ct)

		httpClientMock.AssertExpectations(t)
	})

	t.Run("succesfully creating challenge transaction on anchor platform", func(t *testing.T) {
		mockCT, err := txnbuild.BuildChallengeTx(
			serverPrivateKey,
			receiverAccountID,
			"localhost:8080",
			"localhost:8080",
			"Test SDF Network ; September 2015",
			time.Second*300,
			nil,
		)
		require.NoError(t, err)

		transactionStr, err := mockCT.Base64()
		require.NoError(t, err)

		transactionResponse := fmt.Sprintf(`{
			"transaction": %q,
			"network_passphrase": "Test SDF Network ; September 2015"
		}`, transactionStr)

		response := &http.Response{
			Body:       io.NopCloser(strings.NewReader(transactionResponse)),
			StatusCode: http.StatusOK,
		}
		httpClientMock.On("Do", mock.AnythingOfType("*http.Request")).Return(response, nil).Once()

		ct, err := ap.StartChallengeTransaction()
		require.NoError(t, err)

		assert.Equal(t, "Test SDF Network ; September 2015", ct.NetworkPassphrase)
		assert.Equal(t, transactionStr, ct.TransactionStr)
		assert.Equal(t, mockCT, ct.Transaction)

		httpClientMock.AssertExpectations(t)
	})
}

func Test_SignChallengeTransaction(t *testing.T) {
	receiverPrivateKey := "SCKUFVMBEBE7NCJPJ6DIURH5ECC5ORPYPNG46YFQWCTECEPF35QK4XTO"
	receiverAccountID := "GDJNLIFC2JTGKTD4LA4D77TSEGMQLZKBIEXMMJT64AEWWVYC5JKJHH2X"

	serverPrivateKey := "SBG2NGVW7VYIZDK4R775UXNRZUODJBS3N3H6ICKKAAMXUSWBOHUXETE4"

	mockCT, err := txnbuild.BuildChallengeTx(
		serverPrivateKey,
		receiverAccountID,
		"mock_anchor.com",
		"mock_anchor.com",
		"Test SDF Network ; September 2015",
		time.Second*300,
		nil,
	)
	require.NoError(t, err)

	transactionStr, err := mockCT.Base64()
	require.NoError(t, err)

	ct := &ChallengeTransaction{
		TransactionStr:    transactionStr,
		Transaction:       mockCT,
		NetworkPassphrase: "Test SDF Network ; September 2015",
	}

	t.Run("error getting stellar keypair", func(t *testing.T) {
		ap := AnchorPlatformIntegrationTests{
			ReceiverAccountPrivateKey: "invalid private key",
		}
		st, err := ap.SignChallengeTransaction(ct)
		require.EqualError(t, err, "error getting receiver keypair: non-canonical strkey; unused leftover character")
		assert.Empty(t, st)
	})

	t.Run("signing challenge transaction", func(t *testing.T) {
		ap := AnchorPlatformIntegrationTests{
			ReceiverAccountPrivateKey: receiverPrivateKey,
		}
		st, err := ap.SignChallengeTransaction(ct)
		require.NoError(t, err)

		assert.Equal(t, "Test SDF Network ; September 2015", st.NetworkPassphrase)
		assert.Equal(t, transactionStr, st.TransactionStr)
		assert.Equal(t, mockCT, st.Transaction)

		kp, err := keypair.ParseFull(receiverPrivateKey)
		require.NoError(t, err)
		signedTx, err := mockCT.Sign("Test SDF Network ; September 2015", kp)
		require.NoError(t, err)

		assert.Equal(t, signedTx, st.SignedTransaction)
	})
}

func Test_SendSignedChallengeTransaction(t *testing.T) {
	httpClientMock := httpclient.HttpClientMock{}

	ap := AnchorPlatformIntegrationTests{
		HttpClient:               &httpClientMock,
		AnchorPlatformBaseSepURL: "http://mock_anchor.com/",
	}

	receiverPrivateKey := "SCKUFVMBEBE7NCJPJ6DIURH5ECC5ORPYPNG46YFQWCTECEPF35QK4XTO"
	receiverAccountID := "GDJNLIFC2JTGKTD4LA4D77TSEGMQLZKBIEXMMJT64AEWWVYC5JKJHH2X"

	serverPrivateKey := "SBG2NGVW7VYIZDK4R775UXNRZUODJBS3N3H6ICKKAAMXUSWBOHUXETE4"

	mockCT, err := txnbuild.BuildChallengeTx(
		serverPrivateKey,
		receiverAccountID,
		"mock_anchor.com",
		"mock_anchor.com",
		"Test SDF Network ; September 2015",
		time.Second*300,
		nil,
	)
	require.NoError(t, err)

	transactionStr, err := mockCT.Base64()
	require.NoError(t, err)

	kp, err := keypair.ParseFull(receiverPrivateKey)
	require.NoError(t, err)
	signedTx, err := mockCT.Sign("Test SDF Network ; September 2015", kp)
	require.NoError(t, err)

	st := &SignedChallengeTransaction{
		ChallengeTransaction: &ChallengeTransaction{
			TransactionStr:    transactionStr,
			Transaction:       mockCT,
			NetworkPassphrase: "Test SDF Network ; September 2015",
		},
		SignedTransaction: signedTx,
	}

	t.Run("error converting signed transaction to base 64", func(t *testing.T) {
		httpClientMock.On("Do", mock.AnythingOfType("*http.Request")).Return(nil, fmt.Errorf("error calling the request")).Once()

		at, err := ap.SendSignedChallengeTransaction(st)
		require.EqualError(t, err, "error making request to anchor platform post AUTH: error calling the request")
		assert.Empty(t, at)

		httpClientMock.AssertExpectations(t)
	})

	t.Run("error trying to send signed challenge transaction on anchor platform", func(t *testing.T) {
		transactionResponse := `{Error sending signed challenge transaction}`
		response := &http.Response{
			Body:       io.NopCloser(strings.NewReader(transactionResponse)),
			StatusCode: http.StatusBadRequest,
		}
		httpClientMock.On("Do", mock.AnythingOfType("*http.Request")).Return(response, nil).Once()

		at, err := ap.SendSignedChallengeTransaction(st)
		require.EqualError(t, err, "error sending signed challenge transaction on anchor platform")
		assert.Empty(t, at)

		httpClientMock.AssertExpectations(t)
	})

	t.Run("error invalid response body", func(t *testing.T) {
		transactionResponse := ``

		response := &http.Response{
			Body:       io.NopCloser(strings.NewReader(transactionResponse)),
			StatusCode: http.StatusOK,
		}
		httpClientMock.On("Do", mock.AnythingOfType("*http.Request")).Return(response, nil).Once()

		at, err := ap.SendSignedChallengeTransaction(st)
		require.EqualError(t, err, "error decoding response body: EOF")
		assert.Empty(t, at)

		httpClientMock.AssertExpectations(t)
	})

	t.Run("succesfully sending signed challenge transaction on anchor platform", func(t *testing.T) {
		authToken := "valid token"

		transactionResponse := fmt.Sprintf(`{
			"token": %q
		}`, authToken)

		response := &http.Response{
			Body:       io.NopCloser(strings.NewReader(transactionResponse)),
			StatusCode: http.StatusOK,
		}
		httpClientMock.On("Do", mock.AnythingOfType("*http.Request")).Return(response, nil).Once()

		at, err := ap.SendSignedChallengeTransaction(st)
		require.NoError(t, err)

		assert.Equal(t, authToken, at.Token)

		httpClientMock.AssertExpectations(t)
	})
}

func Test_CreateSep24DepositTransaction(t *testing.T) {
	httpClientMock := httpclient.HttpClientMock{}

	receiverAccountID := "GDJNLIFC2JTGKTD4LA4D77TSEGMQLZKBIEXMMJT64AEWWVYC5JKJHH2X"

	ap := AnchorPlatformIntegrationTests{
		HttpClient:               &httpClientMock,
		AnchorPlatformBaseSepURL: "http://mock_anchor.com/",
		ReceiverAccountPublicKey: receiverAccountID,
		DisbursedAssetCode:       "USDC",
	}

	at := &AnchorPlatformAuthToken{
		Token: "valid token",
	}

	t.Run("error calling httpClient.Do", func(t *testing.T) {
		httpClientMock.On("Do", mock.AnythingOfType("*http.Request")).Return(nil, fmt.Errorf("error calling the request")).Once()

		at, dr, err := ap.CreateSep24DepositTransaction(at)
		require.EqualError(t, err, "error making request to anchor platform post SEP24 Deposit: error calling the request")
		assert.Empty(t, dr)
		assert.Empty(t, at)

		httpClientMock.AssertExpectations(t)
	})

	t.Run("error trying to create sep24 deposit transaction on anchor platform", func(t *testing.T) {
		depositResponse := `{Error creating sep24 deposit transaction}`
		response := &http.Response{
			Body:       io.NopCloser(strings.NewReader(depositResponse)),
			StatusCode: http.StatusBadRequest,
		}
		httpClientMock.On("Do", mock.AnythingOfType("*http.Request")).Return(response, nil).Once()

		at, dr, err := ap.CreateSep24DepositTransaction(at)
		require.EqualError(t, err, "error creating sep24 deposit transaction on anchor platform")
		assert.Empty(t, dr)
		assert.Empty(t, at)

		httpClientMock.AssertExpectations(t)
	})

	t.Run("error invalid response body", func(t *testing.T) {
		depositResponse := ``

		response := &http.Response{
			Body:       io.NopCloser(strings.NewReader(depositResponse)),
			StatusCode: http.StatusOK,
		}
		httpClientMock.On("Do", mock.AnythingOfType("*http.Request")).Return(response, nil).Once()

		at, dr, err := ap.CreateSep24DepositTransaction(at)
		require.EqualError(t, err, "error decoding response body: EOF")
		assert.Empty(t, dr)
		assert.Empty(t, at)

		httpClientMock.AssertExpectations(t)
	})

	t.Run("error invalid url in response body", func(t *testing.T) {
		depositResponse := `{
			"id": "mock_id",
			"url": "%"
		}`

		response := &http.Response{
			Body:       io.NopCloser(strings.NewReader(depositResponse)),
			StatusCode: http.StatusOK,
		}
		httpClientMock.On("Do", mock.AnythingOfType("*http.Request")).Return(response, nil).Once()

		at, dr, err := ap.CreateSep24DepositTransaction(at)
		require.EqualError(t, err, "error parsing url from AnchorPlatformDepositResponse: parse \"%\": invalid URL escape \"%\"")
		assert.Empty(t, dr)
		assert.Empty(t, at)

		httpClientMock.AssertExpectations(t)
	})

	t.Run("error invalid query params in url from response body", func(t *testing.T) {
		depositResponse := `{
			"id": "mock_id",
			"url": "http://mock_registration_url.com?q=%"
		}`

		response := &http.Response{
			Body:       io.NopCloser(strings.NewReader(depositResponse)),
			StatusCode: http.StatusOK,
		}
		httpClientMock.On("Do", mock.AnythingOfType("*http.Request")).Return(response, nil).Once()

		at, dr, err := ap.CreateSep24DepositTransaction(at)
		require.EqualError(t, err, "error parsing query params from register url: invalid URL escape \"%\"")
		assert.Empty(t, dr)
		assert.Empty(t, at)

		httpClientMock.AssertExpectations(t)
	})

	t.Run("error url from response body missing token", func(t *testing.T) {
		depositResponse := `{
			"id": "mock_id",
			"url": "http://mock_registration_url.com"
		}`

		response := &http.Response{
			Body:       io.NopCloser(strings.NewReader(depositResponse)),
			StatusCode: http.StatusOK,
		}
		httpClientMock.On("Do", mock.AnythingOfType("*http.Request")).Return(response, nil).Once()

		at, dr, err := ap.CreateSep24DepositTransaction(at)
		require.EqualError(t, err, "error register url not have a valid token")
		assert.Empty(t, dr)
		assert.Empty(t, at)

		httpClientMock.AssertExpectations(t)
	})

	t.Run("succesfully creating sep24 deposit transaction on anchor platform", func(t *testing.T) {
		depositResponse := `{
			"id": "mock_id",
			"url": "http://mock_registration_url.com?token=valid_token"
		}`

		response := &http.Response{
			Body:       io.NopCloser(strings.NewReader(depositResponse)),
			StatusCode: http.StatusOK,
		}
		httpClientMock.On("Do", mock.AnythingOfType("*http.Request")).Return(response, nil).Once()

		at, dr, err := ap.CreateSep24DepositTransaction(at)
		require.NoError(t, err)

		assert.Equal(t, "mock_id", dr.TransactionID)
		assert.Equal(t, "http://mock_registration_url.com?token=valid_token", dr.URL)
		assert.Equal(t, "valid_token", at.Token)

		httpClientMock.AssertExpectations(t)
	})
}
