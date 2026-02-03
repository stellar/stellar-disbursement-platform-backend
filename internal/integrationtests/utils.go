package integrationtests

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"path"

	"github.com/gocarina/gocsv"
	"github.com/stellar/go-stellar-sdk/clients/horizonclient"
	"github.com/stellar/go-stellar-sdk/protocols/horizon/operations"
	"github.com/stellar/go-stellar-sdk/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

// logErrorResponses logs the response body for requests with error status.
func logErrorResponses(ctx context.Context, body io.ReadCloser) {
	respBody, err := io.ReadAll(body)
	defer utils.DeferredClose(ctx, body, "closing response body")
	if err == nil {
		log.Ctx(ctx).Infof("error message response: %s", string(respBody))
	}
}

func readDisbursementCSV(disbursementFilePath string, disbursementFileName string) ([]*data.DisbursementInstruction, error) {
	err := utils.ValidatePathIsNotTraversal(disbursementFileName)
	if err != nil {
		return nil, fmt.Errorf("validating file path: %w", err)
	}

	filePath := path.Join(disbursementFilePath, disbursementFileName)

	csvBytes, err := fs.ReadFile(DisbursementCSVFiles, filePath)
	if err != nil {
		return nil, fmt.Errorf("reading csv file: %w", err)
	}

	instructions := []*data.DisbursementInstruction{}
	if err = gocsv.UnmarshalBytes(csvBytes, &instructions); err != nil {
		return nil, fmt.Errorf("parsing csv file: %w", err)
	}

	return instructions, nil
}

func getTransactionOnHorizon(client horizonclient.ClientInterface, transactionID string) (*operations.Payment, error) {
	records, err := client.Payments(horizonclient.OperationRequest{ForTransaction: transactionID})
	if err != nil {
		return nil, fmt.Errorf("checking payment in horizon: %w", err)
	}

	if len(records.Embedded.Records) == 0 {
		return nil, fmt.Errorf("no payment records found in horizon for transaction %s", transactionID)
	}

	hPayment, ok := records.Embedded.Records[0].(operations.Payment)
	if !ok {
		return nil, fmt.Errorf("casting payment record to operations.Payment")
	}

	return &hPayment, nil
}

// getInvokeHostFunctionOnHorizon retrieves an InvokeHostFunction operation from a transaction.
// This is used for contract account payments which use Soroban smart contracts instead of
// regular Payment operations.
func getInvokeHostFunctionOnHorizon(client horizonclient.ClientInterface, transactionID string) (*operations.InvokeHostFunction, error) {
	records, err := client.Operations(horizonclient.OperationRequest{ForTransaction: transactionID})
	if err != nil {
		return nil, fmt.Errorf("fetching operations from horizon: %w", err)
	}

	if len(records.Embedded.Records) == 0 {
		return nil, fmt.Errorf("no operation records found in horizon for transaction %s", transactionID)
	}

	for _, record := range records.Embedded.Records {
		if ihf, ok := record.(operations.InvokeHostFunction); ok {
			return &ihf, nil
		}
	}

	return nil, fmt.Errorf("no InvokeHostFunction operation found for transaction %s", transactionID)
}
