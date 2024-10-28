package integrationtests

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"testing"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/protocols/horizon/operations"

	"github.com/stellar/go/support/log"
	"github.com/stellar/go/support/render/problem"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_logErrorResponses(t *testing.T) {
	body := `{error response body}`
	response := &http.Response{
		Body: io.NopCloser(strings.NewReader(body)),
	}
	ctx := context.Background()

	buf := new(strings.Builder)
	log.DefaultLogger.SetOutput(buf)
	log.DefaultLogger.SetLevel(log.InfoLevel)

	logErrorResponses(ctx, response.Body)

	require.Contains(t, buf.String(), `level=info msg="error message response: {error response body}`)
}

func Test_readDisbursementCSV(t *testing.T) {
	t.Run("error if file path is traversal", func(t *testing.T) {
		expectedError := "validating file path: path cannot contain path traversal"

		data, err := readDisbursementCSV("resources", "../invalid_traversal_path.csv")
		require.EqualError(t, err, expectedError)
		assert.Empty(t, data)
	})

	t.Run("error trying read csv file", func(t *testing.T) {
		filePath := path.Join("resources", "invalid_file.csv")
		expectedError := fmt.Sprintf("reading csv file: open %s: file does not exist", filePath)

		data, err := readDisbursementCSV("resources", "invalid_file.csv")
		require.EqualError(t, err, expectedError)
		assert.Empty(t, data)
	})

	t.Run("error opening empty csv file", func(t *testing.T) {
		data, err := readDisbursementCSV("resources", "empty_csv_file.csv")
		require.EqualError(t, err, "parsing csv file: empty csv file given")
		assert.Empty(t, data)
	})

	t.Run("reading csv file", func(t *testing.T) {
		data, err := readDisbursementCSV("resources", "disbursement_instructions_phone.csv")
		require.NoError(t, err)
		assert.Equal(t, "0.1", data[0].Amount)
		assert.NotNil(t, data[0].Phone)
		assert.Equal(t, "+12025550191", data[0].Phone)
		assert.Equal(t, "1", data[0].ID)
		assert.Equal(t, "1999-03-30", data[0].VerificationValue)
	})
}

func Test_getTransactionInHorizon(t *testing.T) {
	mockHorizonClient := &horizonclient.MockClient{}
	mockTransactionID := "transactionID"

	t.Run("error trying to get transaction on horizon", func(t *testing.T) {
		mockHorizonClient.
			On("Payments", horizonclient.OperationRequest{ForTransaction: mockTransactionID}).
			Return(operations.OperationsPage{}, horizonclient.Error{
				Problem: problem.NotFound,
			}).
			Once()

		ph, err := getTransactionOnHorizon(mockHorizonClient, mockTransactionID)
		require.EqualError(t, err, "error checking payment in horizon: horizon error: \"Resource Missing\" - check horizon.Error.Problem for more information")
		assert.Empty(t, ph)

		mockHorizonClient.AssertExpectations(t)
	})

	horizonResponse := `{
		"_embedded": {
		  "records": [
			{
			  "_links": {
				"self": {
				  "href": ""
				},
				"transaction": {
				  "href": ""
				},
				"effects": {
				  "href": ""
				},
				"succeeds": {
				  "href": ""
				},
				"precedes": {
				  "href": ""
				}
			  },
			  "id": "123456",
			  "paging_token": "67890",
			  "transaction_successful": true,
			  "source_account": "GBZF7AS3TBASAL5RQ7ECJODFWFLBDCKJK5SMPUCO5R36CJUIZRWQJTGB",
			  "type": "payment",
			  "type_i": 1,
			  "created_at": "2023-06-15T14:01:59Z",
			  "transaction_hash": "17qw02bb7aaa949e9a852b48176e64dae381f4ce20af454b5f4d405ce67wsad1",
			  "asset_type": "credit_alphanum4",
			  "asset_code": "USDC",
			  "asset_issuer": "GBZF7AS3TBASAL5RQ7ECJODFWFLBDCKJK5SMPUCO5R36CJUIZRWQJTGB",
			  "from": "GBZF7AS3TBASAL5RQ7ECJODFWFLBDCKJK5SMPUCO5R36CJUIZRWQJTGB",
			  "to": "GD44L3Q6NYRFPVOX4CJUUV63QEOOU3R5JNQJBLR6WWXFWYHEGK2YVBQ7",
			  "amount": "100.0000000"
			}
		  ]
		}
	  }
	`
	var paymentPage operations.OperationsPage

	err := json.Unmarshal([]byte(horizonResponse), &paymentPage)
	require.NoError(t, err)

	t.Run("successful get transaction on horizon", func(t *testing.T) {
		mockHorizonClient.
			On("Payments", horizonclient.OperationRequest{ForTransaction: mockTransactionID}).
			Return(paymentPage, nil).
			Once()

		ph, err := getTransactionOnHorizon(mockHorizonClient, mockTransactionID)
		require.NoError(t, err)
		assert.Equal(t, "GD44L3Q6NYRFPVOX4CJUUV63QEOOU3R5JNQJBLR6WWXFWYHEGK2YVBQ7", ph.ReceiverAccount)
		assert.Equal(t, "USDC", ph.AssetCode)
		assert.Equal(t, "GBZF7AS3TBASAL5RQ7ECJODFWFLBDCKJK5SMPUCO5R36CJUIZRWQJTGB", ph.AssetIssuer)
		assert.Equal(t, "100.0000000", ph.Amount)
		assert.Equal(t, true, ph.TransactionSuccessful)

		mockHorizonClient.AssertExpectations(t)
	})
}
