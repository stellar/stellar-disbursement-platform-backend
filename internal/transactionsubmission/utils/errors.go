package utils

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/protocols/horizon"
	"github.com/stellar/go/support/log"
	"github.com/stellar/go/support/render/problem"
	sdpUtils "github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"golang.org/x/exp/slices"
)

// TransactionStatusUpdateError is an error that occurs when failing to update a transaction's status.
type TransactionStatusUpdateError struct {
	Status   string
	TxID     string
	ForRetry bool
	// Err is the underlying error that caused the transaction status update to fail.
	Err error
}

func (e *TransactionStatusUpdateError) Error() string {
	forRetry := ""
	if e.ForRetry {
		forRetry = " (for retry)"
	}
	return fmt.Sprintf("updating transaction(ID=%q) status to %s%s: %v", e.TxID, e.Status, forRetry, e.Err)
}

func (e *TransactionStatusUpdateError) Unwrap() error {
	return e.Err
}

func NewTransactionStatusUpdateError(status, txID string, forRetry bool, err error) *TransactionStatusUpdateError {
	return &TransactionStatusUpdateError{
		Status:   status,
		TxID:     txID,
		ForRetry: forRetry,
		Err:      err,
	}
}

var _ error = &TransactionStatusUpdateError{}

// HorizonErrorWrapper is an error that occurs when a horizon response is not successful.
type HorizonErrorWrapper struct {
	StatusCode  int
	Problem     problem.P
	Err         error
	ResultCodes *horizon.TransactionResultCodes
}

func NewHorizonErrorWrapper(err error) *HorizonErrorWrapper {
	if err == nil {
		return nil
	}

	hError := horizonclient.GetError(err)
	if hError == nil {
		return &HorizonErrorWrapper{
			Err: err,
		}
	}

	resultCodes, resCodeErr := hError.ResultCodes()
	if resCodeErr != nil {
		log.Errorf("parsing result_codes: %v", resCodeErr)
	}

	return &HorizonErrorWrapper{
		Err:         err,
		Problem:     hError.Problem,
		StatusCode:  hError.Problem.Status,
		ResultCodes: resultCodes,
	}
}

func (e *HorizonErrorWrapper) Unwrap() error {
	return e.Err
}

func (e *HorizonErrorWrapper) Error() string {
	if !e.IsHorizonError() {
		return fmt.Sprintf("horizon response error: %v", e.Err)
	}

	msgBuilder := &strings.Builder{}
	msgBuilder.WriteString(fmt.Sprintf("horizon response error: StatusCode=%d", e.StatusCode))
	if e.Problem.Type != "" {
		msgBuilder.WriteString(fmt.Sprintf(", Type=%s", e.Problem.Type))
	}
	if e.Problem.Title != "" {
		msgBuilder.WriteString(fmt.Sprintf(", Title=%s", e.Problem.Title))
	}
	if e.Problem.Detail != "" {
		msgBuilder.WriteString(fmt.Sprintf(", Detail=%s", e.Problem.Detail))
	}
	// TODO: place extras right after status codes, for better readability. Details are pretty verbose and not that useful.
	if e.HasResultCodes() {
		e.handleExtrasResultCodes(msgBuilder)
	}
	return msgBuilder.String()
}

func (e *HorizonErrorWrapper) IsHorizonError() bool {
	return !sdpUtils.IsEmpty(e.Problem)
}

func (e *HorizonErrorWrapper) IsNotFound() bool {
	return e.IsHorizonError() && e.StatusCode == http.StatusNotFound
}

func (e *HorizonErrorWrapper) IsRateLimit() bool {
	return e.IsHorizonError() && e.StatusCode == http.StatusTooManyRequests
}

func (e *HorizonErrorWrapper) IsGatewayTimeout() bool {
	return e.IsHorizonError() && e.StatusCode == http.StatusGatewayTimeout
}

func (e *HorizonErrorWrapper) HasResultCodes() bool {
	return e.IsHorizonError() && e.ResultCodes != nil
}

// IsNotEnoughLumens verifies if the Horizon Error is related to the
// transaction attempting to bring the source account lumens balance below the minimum reserve.
func (e *HorizonErrorWrapper) IsNotEnoughLumens() bool {
	if !e.HasResultCodes() {
		return false
	}

	code := "tx_insufficient_balance"
	opCode := "op_underfunded"
	return (e.ResultCodes.TransactionCode == code ||
		e.ResultCodes.InnerTransactionCode == code ||
		slices.Contains(e.ResultCodes.OperationCodes, opCode))
}

// IsNoSourceAccount verifies if the Horizon Error is related to the
// source account not being found.
func (e *HorizonErrorWrapper) IsNoSourceAccount() bool {
	if !e.HasResultCodes() {
		return false
	}

	txCode := "tx_no_source_account"
	opCode := "op_no_source_account"
	return (e.ResultCodes.TransactionCode == txCode ||
		e.ResultCodes.InnerTransactionCode == txCode ||
		slices.Contains(e.ResultCodes.OperationCodes, opCode))
}

// IsNoIssuer verifies if the Horizon Error is related to the
// issuer of the asset not existing.
func (e *HorizonErrorWrapper) IsNoIssuer() bool {
	if !e.HasResultCodes() {
		return false
	}

	opCode := "op_no_issuer"
	return slices.Contains(e.ResultCodes.OperationCodes, opCode)
}

// IsSourceNotAuthorized verifies if the Horizon Error is related to the
// source account not having authorization from the asset issuer to send the asset.
func (e *HorizonErrorWrapper) IsSourceAccountNotAuthorized() bool {
	if !e.HasResultCodes() {
		return false
	}

	opCode := "op_src_not_authorized"
	return slices.Contains(e.ResultCodes.OperationCodes, opCode)
}

// IsSourceNoTrustline verifies if the Horizon Error is related to the
// source account not having a trustline for the asset being sent.
func (e *HorizonErrorWrapper) IsSourceNoTrustline() bool {
	if !e.HasResultCodes() {
		return false
	}

	opCode := "op_src_no_trust"
	return slices.Contains(e.ResultCodes.OperationCodes, opCode)
}

// IsDestinationAccountNotAuthorized verifies if the Horizon Error is related to the
// destination account is not being authorized by the asset issuer to receive the asset.
func (e *HorizonErrorWrapper) IsDestinationAccountNotAuthorized() bool {
	if !e.HasResultCodes() {
		return false
	}

	opCode := "op_not_authorized"
	return slices.Contains(e.ResultCodes.OperationCodes, opCode)
}

// IsNoTrustline verifies if the Horizon Error is related to the
// destination account not having a trustline for the asset being sent.
func (e *HorizonErrorWrapper) IsDestinationNoTrustline() bool {
	if !e.HasResultCodes() {
		return false
	}

	opCode := "op_no_trust"
	return slices.Contains(e.ResultCodes.OperationCodes, opCode)
}

// IsLineFull verifies if the Horizon Error is related to the
// destination account not having sufficient limits to receive the payment amount
// and still satisfy its buying liabilities.
func (e *HorizonErrorWrapper) IsLineFull() bool {
	if !e.HasResultCodes() {
		return false
	}

	opCode := "op_line_full"
	return slices.Contains(e.ResultCodes.OperationCodes, opCode)
}

// IsNoDestinationAccount verifies if the Horizon Error is related to the
// destination account not existing.
func (e *HorizonErrorWrapper) IsNoDestinationAccount() bool {
	if !e.HasResultCodes() {
		return false
	}

	opCode := "op_no_destination"
	return slices.Contains(e.ResultCodes.OperationCodes, opCode)
}

// IsBadAuthentication verifies if the Horizon Error is related to
// invalid transaction or operation signatures.
func (e *HorizonErrorWrapper) IsBadAuthentication() bool {
	if !e.HasResultCodes() {
		return false
	}

	txCodes := []string{"tx_bad_auth", "tx_bad_auth_extra"}
	opCode := "op_bad_auth"
	return (slices.Contains(txCodes, e.ResultCodes.TransactionCode) ||
		slices.Contains(txCodes, e.ResultCodes.InnerTransactionCode) ||
		slices.Contains(e.ResultCodes.OperationCodes, opCode))
}

// IsTxInsufficientFee verifies if the Horizon Error is related to the
// fee submitted being too small to be accepted by to the ledger by
// the network.
func (e *HorizonErrorWrapper) IsTxInsufficientFee() bool {
	if !e.HasResultCodes() {
		return false
	}

	txCode := "tx_insufficient_fee"
	return e.ResultCodes.TransactionCode == txCode
}

// IsSourceAccountNotReady verifies if the Horizon Error is related to the
// source account of the transaction. It gathers all errors that would happen
// in a transaction because of a misconfiguration of the source account.
func (e *HorizonErrorWrapper) IsSourceAccountNotReady() bool {
	return (e.IsNotEnoughLumens() ||
		e.IsNoSourceAccount() ||
		e.IsSourceAccountNotAuthorized() ||
		e.IsSourceNoTrustline())
}

// IsDestinationAccountNotReady verifies if the Horizon Error is related to the
// destination account of the transaction. It gathers all errors that would happen
// in a transaction because of a misconfiguration of the destination account.
func (e *HorizonErrorWrapper) IsDestinationAccountNotReady() bool {
	return (e.IsDestinationAccountNotAuthorized() ||
		e.IsDestinationNoTrustline() ||
		e.IsNoDestinationAccount() ||
		e.IsLineFull())
}

// ShouldMarkAsError determines whether a transaction neeeds to be marked as an error based on the
// transaction error code or failed op code so that TSS can determine whether it needs
// to be retried.
func (e *HorizonErrorWrapper) ShouldMarkAsError() bool {
	failedTxErrCodes := []string{
		"tx_bad_auth",
		"tx_bad_auth_extra",
		"tx_insufficient_balance",
	}
	if slices.Contains(failedTxErrCodes, e.ResultCodes.TransactionCode) || slices.Contains(failedTxErrCodes, e.ResultCodes.InnerTransactionCode) {
		return true
	}

	failedOpCodes := []string{
		"op_bad_auth",
		"op_underfunded",
		"op_src_not_authorized",
		"op_no_destination",
		"op_no_trust",
		"op_line_full",
		"op_not_authorized",
		"op_no_issuer",
	}
	for _, opResult := range e.ResultCodes.OperationCodes {
		if slices.Contains(failedOpCodes, opResult) {
			return true
		}
	}

	return false
}

func (e *HorizonErrorWrapper) handleExtrasResultCodes(msgBuilder *strings.Builder) {
	if !e.HasResultCodes() {
		return
	}

	extras := []string{}
	if e.ResultCodes.TransactionCode != "" {
		extras = append(extras, fmt.Sprintf("transaction: %s", e.ResultCodes.TransactionCode))
	}

	if e.ResultCodes.InnerTransactionCode != "" {
		extras = append(extras, fmt.Sprintf("inner transaction: %s", e.ResultCodes.InnerTransactionCode))
	}

	if len(e.ResultCodes.OperationCodes) > 0 {
		msg := fmt.Sprintf("operation codes: [ %s ]", strings.Join(e.ResultCodes.OperationCodes, ", "))
		extras = append(extras, msg)
	}

	if len(extras) > 0 {
		msgBuilder.WriteString(", Extras=")
		msgBuilder.WriteString(strings.Join(extras, " - "))
	}
}

var _ error = &HorizonErrorWrapper{}
