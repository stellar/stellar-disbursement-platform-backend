package integrationtests

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"path"

	"github.com/gocarina/gocsv"
	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
)

// logErrorResponses logs the response body for requests with error status.
func logErrorResponses(ctx context.Context, body io.ReadCloser) {
	respBody, err := io.ReadAll(body)
	if err == nil {
		log.Ctx(ctx).Infof("error message response: %s", string(respBody))
	}
}

func readDisbursementCSV(disbursementFilePath string, disbursementFileName string) ([]*data.DisbursementInstruction, error) {
	filePath := path.Join(disbursementFilePath, disbursementFileName)

	csvBytes, err := fs.ReadFile(DisbursementCSVFiles, filePath)
	if err != nil {
		return nil, fmt.Errorf("error reading csv file: %w", err)
	}

	instructions := []*data.DisbursementInstruction{}
	if err = gocsv.UnmarshalBytes(csvBytes, &instructions); err != nil {
		return nil, fmt.Errorf("error parsing csv file: %w", err)
	}

	return instructions, nil
}

type PaymentHorizon struct {
	ReceiverAccount       string `json:"to"`
	Amount                string `json:"amount"`
	AssetCode             string `json:"asset_code"`
	AssetIssuer           string `json:"asset_issuer"`
	TransactionSuccessful bool   `json:"transaction_successful"`
}

func getTransactionOnHorizon(client horizonclient.ClientInterface, transactionID string) (*PaymentHorizon, error) {
	ph := &PaymentHorizon{}
	records, err := client.Payments(horizonclient.OperationRequest{ForTransaction: transactionID})
	if err != nil {
		return nil, fmt.Errorf("error checking payment in horizon: %w", err)
	}
	paymentRecord, err := json.Marshal(records.Embedded.Records[0])
	if err != nil {
		return nil, fmt.Errorf("error marshaling payment record: %w", err)
	}
	err = json.Unmarshal(paymentRecord, ph)
	if err != nil {
		return nil, fmt.Errorf("error unmarshling payment record: %w", err)
	}

	return ph, nil
}
