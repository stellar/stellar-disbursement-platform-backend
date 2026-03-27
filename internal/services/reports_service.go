package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stellar/go-stellar-sdk/clients/horizonclient"
	"github.com/stellar/go-stellar-sdk/protocols/horizon/base"
	"github.com/stellar/go-stellar-sdk/protocols/horizon/operations"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/assets"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

const (
	// MaxStatementPaymentsPages caps pagination over Horizon payments to avoid unbounded loops.
	MaxStatementPaymentsPages = 50
	// StatementPaymentsPageLimit is the page size when fetching payments from Horizon.
	StatementPaymentsPageLimit = 200
)

var (
	ErrStatementAccountNotStellar = errors.New("statement is only supported for Stellar distribution accounts")
	ErrStatementAssetNotFound     = errors.New("asset not found for account")
)

// ReportsServiceInterface defines the interface for generating account statements (used by reports handler).
type ReportsServiceInterface interface {
	GetStatement(ctx context.Context, account *schema.TransactionAccount, assetCode string, fromDate, toDate time.Time) (*StatementResult, error)
}

// StatementResult is the full statement response.
type StatementResult struct {
	Summary StatementSummary `json:"summary"`
}

// StatementSummary holds the statement summary section.
type StatementSummary struct {
	Account string                  `json:"account"`
	Assets  []StatementAssetSummary `json:"assets"`
}

// StatementAssetSummary holds per-asset summary and transactions.
type StatementAssetSummary struct {
	Code             string                 `json:"code"`
	BeginningBalance string                 `json:"beginning_balance"`
	TotalCredits     string                 `json:"total_credits"`
	TotalDebits      string                 `json:"total_debits"`
	EndingBalance    string                 `json:"ending_balance"`
	Transactions     []StatementTransaction `json:"transactions"`
}

// AssetRef is a minimal asset reference for JSON.
type AssetRef struct {
	Code string `json:"code"`
}

// StatementTransaction is a single transaction line in the statement.
type StatementTransaction struct {
	ID                  string `json:"id"`
	CreatedAt           string `json:"created_at"`
	UpdatedAt           string `json:"updated_at"`
	Type                string `json:"type"`
	Amount              string `json:"amount"`
	CounterpartyAddress string `json:"counterparty_address"`
	CounterpartyName    string `json:"counterparty_name,omitempty"`
	ExternalPaymentID   string `json:"external_payment_id,omitempty"`
}

// StatementTotals holds the totals section.
type StatementTotals struct {
	TotalDebits  string `json:"total_debits"`
	TotalCredits string `json:"total_credits"`
	Balance      string `json:"balance"`
}

// ReportsService generates account statements from Horizon and DB (for report PDFs).
type ReportsService struct {
	HorizonClient          horizonclient.ClientInterface
	DistributionAccountSvc DistributionAccountServiceInterface
	Models                 *data.Models
}

// NewReportsService creates a new ReportsService.
func NewReportsService(
	horizonClient horizonclient.ClientInterface,
	distSvc DistributionAccountServiceInterface,
	models *data.Models,
) *ReportsService {
	return &ReportsService{
		HorizonClient:          horizonClient,
		DistributionAccountSvc: distSvc,
		Models:                 models,
	}
}

var _ ReportsServiceInterface = (*ReportsService)(nil)

// GetStatement returns the statement for the given account, asset (optional), and date range.
func (s *ReportsService) GetStatement(ctx context.Context, account *schema.TransactionAccount, assetCode string, fromDate, toDate time.Time) (*StatementResult, error) {
	if !account.IsStellar() {
		return nil, ErrStatementAccountNotStellar
	}

	fromStart := time.Date(fromDate.Year(), fromDate.Month(), fromDate.Day(), 0, 0, 0, 0, time.UTC)
	toEnd := time.Date(toDate.Year(), toDate.Month(), toDate.Day(), 23, 59, 59, 999999999, time.UTC)

	var assetsToProcess []*data.Asset
	if assetCode != "" {
		asset, err := s.resolveAsset(ctx, assetCode)
		if err != nil {
			return nil, err
		}
		assetsToProcess = []*data.Asset{asset}
	} else {
		balances, err := s.DistributionAccountSvc.GetBalances(ctx, account)
		if err != nil {
			return nil, fmt.Errorf("getting balances: %w", err)
		}
		for a := range balances {
			asset := a
			assetsToProcess = append(assetsToProcess, &asset)
		}
	}

	assetSummaries := make([]StatementAssetSummary, 0, len(assetsToProcess))
	now := time.Now().UTC()
	afterPeriodStart := time.Date(toDate.Year(), toDate.Month(), toDate.Day()+1, 0, 0, 0, 0, time.UTC)
	afterPeriodEnd := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 999999999, time.UTC)

	for _, asset := range assetsToProcess {
		currentBalance, err := s.DistributionAccountSvc.GetBalance(ctx, account, *asset)
		if err != nil {
			if errors.Is(err, ErrNoBalanceForAsset) {
				continue
			}
			return nil, fmt.Errorf("getting balance: %w", err)
		}

		transactions, totalCredits, totalDebits, err := s.fetchPaymentsInRange(ctx, account.Address, asset, fromStart, toEnd)
		if err != nil {
			return nil, err
		}

		var creditsAfter, debitsAfter decimal.Decimal
		if !afterPeriodStart.After(afterPeriodEnd) {
			creditsAfter, debitsAfter, err = s.fetchTotalsInRange(ctx, account.Address, asset, afterPeriodStart, afterPeriodEnd)
			if err != nil {
				return nil, err
			}
		}

		endingBalance := currentBalance.Sub(creditsAfter).Add(debitsAfter)
		beginningBalance := endingBalance.Sub(totalCredits).Add(totalDebits)
		if beginningBalance.LessThan(decimal.Zero) {
			beginningBalance = decimal.Zero
		}

		codeDisplay := asset.Code
		if asset.IsNative() {
			codeDisplay = assets.XLMAssetCode
		}

		assetSummaries = append(assetSummaries, StatementAssetSummary{
			Code:             codeDisplay,
			BeginningBalance: formatStellarAmount(beginningBalance),
			TotalCredits:     formatStellarAmount(totalCredits),
			TotalDebits:      formatStellarAmount(totalDebits),
			EndingBalance:    formatStellarAmount(endingBalance),
			Transactions:     transactions,
		})
	}

	if len(assetSummaries) == 0 && assetCode != "" {
		return nil, ErrStatementAssetNotFound
	}

	return &StatementResult{
		Summary: StatementSummary{
			Account: "stellar:" + account.Address,
			Assets:  assetSummaries,
		},
	}, nil
}

func (s *ReportsService) resolveAsset(ctx context.Context, assetCode string) (*data.Asset, error) {
	code := assetCode
	if code == assets.XLMAssetCodeAlias {
		code = assets.XLMAssetCode
	}
	if code == assets.XLMAssetCode {
		return &data.Asset{Code: assets.XLMAssetCode, Issuer: ""}, nil
	}
	// Non-native: look up by code; use empty issuer to match any issuer, or get first by code from GetAll
	all, err := s.Models.Assets.GetAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing assets: %w", err)
	}
	for i := range all {
		if all[i].Code == code {
			return &all[i], nil
		}
	}
	return nil, ErrStatementAssetNotFound
}

type transactionAccumulator struct {
	transactions        []StatementTransaction
	totalCredits        decimal.Decimal
	totalDebits         decimal.Decimal
	collectTransactions bool
}

func (s *ReportsService) fetchPaymentsInRange(
	ctx context.Context,
	accountAddress string,
	asset *data.Asset,
	fromStart, toEnd time.Time,
) ([]StatementTransaction, decimal.Decimal, decimal.Decimal, error) {
	accumulator := transactionAccumulator{collectTransactions: true}

	req := horizonclient.OperationRequest{
		ForAccount: accountAddress,
		Order:      horizonclient.OrderDesc,
		Limit:      StatementPaymentsPageLimit,
	}

	for pageCount := 0; pageCount < MaxStatementPaymentsPages; pageCount++ {
		page, err := s.HorizonClient.Payments(req)
		if err != nil {
			return nil, decimal.Zero, decimal.Zero, fmt.Errorf("fetching payments: %w", err)
		}

		shouldStop, err := s.processPaymentPage(ctx, page, accountAddress, asset, fromStart, toEnd, &accumulator)
		if err != nil {
			return nil, decimal.Zero, decimal.Zero, err
		}
		if shouldStop {
			break
		}

		if !s.shouldContinuePaymentsPagination(page) {
			break
		}

		cursor := getNextPaymentsPageCursor(page)
		if cursor == "" {
			break
		}
		req.Cursor = cursor
	}

	// Reverse to chronological (ascending) order for display.
	for i, j := 0, len(accumulator.transactions)-1; i < j; i, j = i+1, j-1 {
		accumulator.transactions[i], accumulator.transactions[j] = accumulator.transactions[j], accumulator.transactions[i]
	}

	return accumulator.transactions, accumulator.totalCredits, accumulator.totalDebits, nil
}

// fetchTotalsInRange returns total credits and debits in the given range without building the transaction list.
func (s *ReportsService) fetchTotalsInRange(
	ctx context.Context,
	accountAddress string,
	asset *data.Asset,
	fromStart, toEnd time.Time,
) (totalCredits, totalDebits decimal.Decimal, err error) {
	accumulator := transactionAccumulator{collectTransactions: false}

	req := horizonclient.OperationRequest{
		ForAccount: accountAddress,
		Order:      horizonclient.OrderDesc,
		Limit:      StatementPaymentsPageLimit,
	}

	for pageCount := 0; pageCount < MaxStatementPaymentsPages; pageCount++ {
		page, err := s.HorizonClient.Payments(req)
		if err != nil {
			return decimal.Zero, decimal.Zero, fmt.Errorf("fetching payments: %w", err)
		}

		shouldStop, err := s.processPaymentPage(ctx, page, accountAddress, asset, fromStart, toEnd, &accumulator)
		if err != nil {
			return decimal.Zero, decimal.Zero, err
		}
		if shouldStop {
			break
		}

		if !s.shouldContinuePaymentsPagination(page) {
			break
		}

		cursor := getNextPaymentsPageCursor(page)
		if cursor == "" {
			break
		}
		req.Cursor = cursor
	}

	return accumulator.totalCredits, accumulator.totalDebits, nil
}

func (s *ReportsService) processPaymentPage(
	ctx context.Context,
	page operations.OperationsPage,
	accountAddress string,
	asset *data.Asset,
	fromStart, toEnd time.Time,
	accumulator *transactionAccumulator,
) (shouldStop bool, err error) {
	for i := range page.Embedded.Records {
		op := page.Embedded.Records[i]
		createdAt := op.GetBase().LedgerCloseTime

		if createdAt.After(toEnd) {
			continue
		}

		if createdAt.Before(fromStart) {
			return true, nil
		}

		line, credits, debits, err := s.processPaymentOperation(ctx, op, accountAddress, asset)
		if err != nil {
			return false, err
		}
		if line == nil {
			continue
		}

		if accumulator.collectTransactions {
			accumulator.transactions = append(accumulator.transactions, *line)
		}
		accumulator.totalCredits = accumulator.totalCredits.Add(credits)
		accumulator.totalDebits = accumulator.totalDebits.Add(debits)
	}
	return false, nil
}

func (s *ReportsService) shouldContinuePaymentsPagination(page operations.OperationsPage) bool {
	return len(page.Embedded.Records) >= StatementPaymentsPageLimit
}

// Returns the last record's paging token.
// Horizon cursors are exclusive, so this fetches the next page without skipping records.
func getNextPaymentsPageCursor(page operations.OperationsPage) string {
	records := page.Embedded.Records
	if len(records) == 0 {
		return ""
	}
	return records[len(records)-1].PagingToken()
}

// processPaymentOperation processes a single payment operation and returns a StatementTransaction line
// (or nil if the operation doesn't match the asset), plus credits and debits for the operation.
func (s *ReportsService) processPaymentOperation(
	ctx context.Context,
	op operations.Operation,
	accountAddress string,
	asset *data.Asset,
) (*StatementTransaction, decimal.Decimal, decimal.Decimal, error) {
	from, to, amountStr, paymentAsset, opID, ok := extractPaymentOperation(op)
	if !ok {
		return nil, decimal.Zero, decimal.Zero, nil
	}
	if !assetMatchesHorizonAsset(asset, paymentAsset) {
		return nil, decimal.Zero, decimal.Zero, nil
	}

	amount, err := decimal.NewFromString(amountStr)
	if err != nil {
		return nil, decimal.Zero, decimal.Zero, fmt.Errorf("invalid amount %q: %w", amountStr, err)
	}
	if amount.LessThanOrEqual(decimal.Zero) {
		return nil, decimal.Zero, decimal.Zero, nil
	}

	txHash := op.GetTransactionHash()
	createdAtStr := op.GetBase().LedgerCloseTime.UTC().Format(time.RFC3339)

	var dbPayment *data.Payment
	dbPayment, err = s.Models.Payment.GetByStellarTransactionIDAndOperationID(ctx, s.Models.DBConnectionPool, txHash, opID)
	if err != nil && errors.Is(err, data.ErrRecordNotFound) {
		dbPayment, err = s.Models.Payment.GetByStellarTransactionID(ctx, s.Models.DBConnectionPool, txHash)
	}
	if err != nil {
		dbPayment = nil
	}

	updatedAtStr := createdAtStr
	counterpartyName := ""
	externalPaymentID := ""
	if dbPayment != nil {
		if t, ok := dbPayment.StatusHistory.GetSuccessTimestamp(); ok {
			updatedAtStr = t.UTC().Format(time.RFC3339)
		}
		if dbPayment.ReceiverWallet != nil && dbPayment.ReceiverWallet.Receiver.ExternalID != "" {
			counterpartyName = dbPayment.ReceiverWallet.Receiver.ExternalID
		}
		externalPaymentID = dbPayment.ExternalPaymentID
	}
	if counterpartyName == "" {
		counterpartyName = s.resolveCounterparty(ctx, from)
		if counterpartyName == "" {
			counterpartyName = s.resolveCounterparty(ctx, to)
		}
	}

	var txType string
	var counterparty string
	var credits, debits decimal.Decimal
	if from == accountAddress {
		txType = "debit"
		counterparty = to
		debits = debits.Add(amount)
	} else {
		txType = "credit"
		counterparty = from
		credits = credits.Add(amount)
	}

	return &StatementTransaction{
		ID:                  txHash,
		CreatedAt:           createdAtStr,
		UpdatedAt:           updatedAtStr,
		Type:                txType,
		Amount:              amountStr,
		CounterpartyAddress: counterparty,
		CounterpartyName:    counterpartyName,
		ExternalPaymentID:   externalPaymentID,
	}, credits, debits, nil
}

// extractPaymentOperation returns (from, to, amount, destination asset, operation ID, true) for
// payment-like operations (Payment, PathPayment, PathPaymentStrictSend). Path payments are included
// so credits received via path payments appear in the statement.
func extractPaymentOperation(op operations.Operation) (from, to, amountStr string, paymentAsset base.Asset, opID string, ok bool) {
	switch v := op.(type) {
	case operations.Payment:
		return v.From, v.To, v.Amount, v.Asset, v.GetID(), true
	case *operations.Payment:
		return v.From, v.To, v.Amount, v.Asset, v.GetID(), true
	case operations.PathPayment:
		return v.From, v.To, v.Amount, v.Asset, v.GetID(), true
	case *operations.PathPayment:
		return v.From, v.To, v.Amount, v.Asset, v.GetID(), true
	case operations.PathPaymentStrictSend:
		return v.From, v.To, v.Amount, v.Asset, v.GetID(), true
	case *operations.PathPaymentStrictSend:
		return v.From, v.To, v.Amount, v.Asset, v.GetID(), true
	default:
		return "", "", "", base.Asset{}, "", false
	}
}

func assetMatchesHorizonAsset(asset *data.Asset, h base.Asset) bool {
	if asset.IsNative() && h.Type == "native" {
		return true
	}
	return asset.Code == h.Code && (asset.Issuer == h.Issuer || (asset.Issuer == "" && h.Issuer == ""))
}

func (s *ReportsService) resolveCounterparty(ctx context.Context, stellarAddress string) string {
	rw, err := s.Models.ReceiverWallet.GetByStellarAddress(ctx, stellarAddress)
	if err != nil {
		return ""
	}
	return rw.Receiver.ExternalID
}

func formatStellarAmount(d decimal.Decimal) string {
	return d.StringFixed(7)
}
