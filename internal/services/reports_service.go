package services

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
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
	ErrStatementPeriodTooLong     = errors.New("statement period has more payments than one scan reads")
	ErrStatementPeriodTooOld      = errors.New("account has more payments since the statement period than one scan reads")
)

// ReportsServiceInterface defines the interface for generating account statements (used by reports handler).
type ReportsServiceInterface interface {
	GetStatement(ctx context.Context, account *schema.TransactionAccount, walletID, assetCode string, fromDate, toDate time.Time) (*StatementResult, error)
}

// StatementResult is the full statement response.
type StatementResult struct {
	Summary StatementSummary `json:"summary"`
}

// StatementSummary holds the statement summary section.
type StatementSummary struct {
	Account     string                  `json:"account"`
	AccountName string                  `json:"account_name,omitempty"`
	Assets      []StatementAssetSummary `json:"assets"`
}

// StatementAssetSummary holds per-asset summary and transactions.
type StatementAssetSummary struct {
	Code             string `json:"code"`
	BeginningBalance string `json:"beginning_balance"`
	TotalCredits     string `json:"total_credits"`
	TotalDebits      string `json:"total_debits"`
	EndingBalance    string `json:"ending_balance"`
	// Reconciled is false when the counted payments cannot explain the current balance (the
	// derived beginning balance went negative), which means uncounted ledger activity exists.
	Reconciled   bool                   `json:"reconciled"`
	Transactions []StatementTransaction `json:"transactions"`
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

// GetStatement returns the statement for one distribution account: account is its on-chain identity
// (ledger reads), walletID its SDP identity (matching the payments it sent). assetCode is optional.
func (s *ReportsService) GetStatement(ctx context.Context, account *schema.TransactionAccount, walletID, assetCode string, fromDate, toDate time.Time) (*StatementResult, error) {
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
		// Map order is random; sort so every download lists the assets the same way.
		slices.SortFunc(assetsToProcess, func(a, b *data.Asset) int {
			return cmp.Or(cmp.Compare(a.Code, b.Code), cmp.Compare(a.Issuer, b.Issuer))
		})
	}

	// Balances first: an asset the account no longer holds is left out of the statement and of the scan.
	held := make([]*data.Asset, 0, len(assetsToProcess))
	currentBalances := make([]decimal.Decimal, 0, len(assetsToProcess))
	for _, asset := range assetsToProcess {
		currentBalance, err := s.DistributionAccountSvc.GetBalance(ctx, account, *asset)
		if err != nil {
			if errors.Is(err, ErrNoBalanceForAsset) {
				continue
			}
			return nil, fmt.Errorf("getting balance: %w", err)
		}
		held = append(held, asset)
		currentBalances = append(currentBalances, currentBalance)
	}

	activity, err := s.scanPayments(ctx, account.Address, walletID, held, fromStart, toEnd)
	if err != nil {
		return nil, err
	}

	assetSummaries := make([]StatementAssetSummary, 0, len(held))
	for i, asset := range held {
		a := activity[i]
		endingBalance := currentBalances[i].Sub(a.creditsAfter).Add(a.debitsAfter)
		beginningBalance := endingBalance.Sub(a.credits).Add(a.debits)
		reconciled := !beginningBalance.LessThan(decimal.Zero)
		if !reconciled {
			beginningBalance = decimal.Zero
		}

		codeDisplay := asset.Code
		if asset.IsNative() {
			codeDisplay = assets.XLMAssetCode
		}

		assetSummaries = append(assetSummaries, StatementAssetSummary{
			Code:             codeDisplay,
			BeginningBalance: formatStellarAmount(beginningBalance),
			TotalCredits:     formatStellarAmount(a.credits),
			TotalDebits:      formatStellarAmount(a.debits),
			EndingBalance:    formatStellarAmount(endingBalance),
			Reconciled:       reconciled,
			Transactions:     a.transactions,
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

// assetActivity is one asset's share of the ledger scan.
type assetActivity struct {
	transactions              []StatementTransaction
	credits, debits           decimal.Decimal // in the period
	creditsAfter, debitsAfter decimal.Decimal // after the period, up to now
}

// scanPayments reads the account's payments once, newest first, down to the start of the period, and splits them
// per asset into the period's lines and totals and the totals after it. Hitting the page cap first is an error.
func (s *ReportsService) scanPayments(
	ctx context.Context,
	accountAddress, walletID string,
	assets []*data.Asset,
	fromStart, toEnd time.Time,
) ([]assetActivity, error) {
	if len(assets) == 0 {
		return nil, nil
	}
	activity := make([]assetActivity, len(assets))
	labels := map[string]string{} // counterparty address -> receiver external ID, for this statement

	req := horizonclient.OperationRequest{
		ForAccount: accountAddress,
		Order:      horizonclient.OrderDesc,
		Limit:      StatementPaymentsPageLimit,
	}

	complete, inPeriod := false, false
	for pageCount := 0; pageCount < MaxStatementPaymentsPages && !complete; pageCount++ {
		page, err := s.HorizonClient.Payments(req)
		if err != nil {
			return nil, fmt.Errorf("fetching payments: %w", err)
		}

		reachedStart, err := s.scanPaymentPage(ctx, page, accountAddress, walletID, assets, fromStart, toEnd, activity, labels)
		if err != nil {
			return nil, err
		}
		cursor := getNextPaymentsPageCursor(page)
		complete = reachedStart || !s.shouldContinuePaymentsPagination(page) || cursor == ""
		if records := page.Embedded.Records; len(records) > 0 && !records[len(records)-1].GetBase().LedgerCloseTime.After(toEnd) {
			inPeriod = true // the page's oldest payment falls inside the period
		}
		req.Cursor = cursor
	}
	// The scan hit MaxStatementPaymentsPages before the period start, so the statement would be incomplete.
	if !complete {
		if inPeriod {
			return nil, ErrStatementPeriodTooLong
		}
		return nil, ErrStatementPeriodTooOld
	}

	for i := range activity {
		slices.Reverse(activity[i].transactions) // chronological for display
	}
	return activity, nil
}

func (s *ReportsService) scanPaymentPage(
	ctx context.Context,
	page operations.OperationsPage,
	accountAddress, walletID string,
	assets []*data.Asset,
	fromStart, toEnd time.Time,
	activity []assetActivity,
	labels map[string]string,
) (reachedStart bool, err error) {
	for _, op := range page.Embedded.Records {
		createdAt := op.GetBase().LedgerCloseTime
		if createdAt.Before(fromStart) {
			return true, nil
		}

		payment, ok, err := matchStatementPayment(op, accountAddress, assets)
		if err != nil {
			return false, err
		}
		if !ok {
			continue
		}

		a := &activity[payment.asset]
		if createdAt.After(toEnd) {
			// Later payments only move today's balance back to the period's end; no lookups needed.
			if payment.debit {
				a.debitsAfter = a.debitsAfter.Add(payment.amount)
			} else {
				a.creditsAfter = a.creditsAfter.Add(payment.amount)
			}
			continue
		}

		line, err := s.statementLine(ctx, op, payment, walletID, labels)
		if err != nil {
			return false, err
		}
		a.transactions = append(a.transactions, line)
		if payment.debit {
			a.debits = a.debits.Add(payment.amount)
		} else {
			a.credits = a.credits.Add(payment.amount)
		}
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

// statementPayment is a positive payment operation in one of the statement's assets.
type statementPayment struct {
	asset          int // index into the scanned assets
	from, to, opID string
	amountStr      string
	amount         decimal.Decimal
	debit          bool
}

func matchStatementPayment(op operations.Operation, accountAddress string, assets []*data.Asset) (statementPayment, bool, error) {
	from, to, amountStr, paymentAsset, opID, ok := extractPaymentOperation(op)
	if !ok {
		return statementPayment{}, false, nil
	}
	assetIdx := slices.IndexFunc(assets, func(a *data.Asset) bool { return assetMatchesHorizonAsset(a, paymentAsset) })
	if assetIdx < 0 {
		return statementPayment{}, false, nil
	}

	amount, err := decimal.NewFromString(amountStr)
	if err != nil {
		return statementPayment{}, false, fmt.Errorf("invalid amount %q: %w", amountStr, err)
	}
	if amount.LessThanOrEqual(decimal.Zero) {
		return statementPayment{}, false, nil
	}

	return statementPayment{
		asset: assetIdx, from: from, to: to, opID: opID,
		amountStr: amountStr, amount: amount, debit: from == accountAddress,
	}, true, nil
}

// statementLine builds the statement row for a payment in the period, labelled from the SDP's own records.
func (s *ReportsService) statementLine(
	ctx context.Context,
	op operations.Operation,
	payment statementPayment,
	walletID string,
	labels map[string]string,
) (StatementTransaction, error) {
	txHash := op.GetTransactionHash()
	createdAtStr := op.GetBase().LedgerCloseTime.UTC().Format(time.RFC3339)

	dbPayment, err := s.Models.Payment.GetSuccessfulByStellarTransaction(ctx, s.Models.DBConnectionPool, walletID, txHash, payment.opID)
	if err != nil && !errors.Is(err, data.ErrRecordNotFound) {
		return StatementTransaction{}, fmt.Errorf("finding the SDP payment for transaction %s: %w", txHash, err)
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
	txType, counterparty := "credit", payment.from
	if payment.debit {
		txType, counterparty = "debit", payment.to
	}
	if counterpartyName == "" {
		if counterpartyName, err = s.counterpartyLabel(ctx, counterparty, labels); err != nil {
			return StatementTransaction{}, err
		}
	}

	return StatementTransaction{
		ID:                  txHash,
		CreatedAt:           createdAtStr,
		UpdatedAt:           updatedAtStr,
		Type:                txType,
		Amount:              payment.amountStr,
		CounterpartyAddress: counterparty,
		CounterpartyName:    counterpartyName,
		ExternalPaymentID:   externalPaymentID,
	}, nil
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

// counterpartyLabel returns the external ID of the receiver holding address, or "" when none does. Answers are kept
// in labels, so a counterparty that appears on many rows is looked up once.
func (s *ReportsService) counterpartyLabel(ctx context.Context, address string, labels map[string]string) (string, error) {
	if label, ok := labels[address]; ok {
		return label, nil
	}
	rw, err := s.Models.ReceiverWallet.GetByStellarAddress(ctx, address)
	switch {
	case errors.Is(err, data.ErrRecordNotFound):
		labels[address] = ""
	case err != nil:
		return "", fmt.Errorf("finding the receiver for %s: %w", address, err)
	default:
		labels[address] = rw.Receiver.ExternalID
	}
	return labels[address], nil
}

func formatStellarAmount(d decimal.Decimal) string {
	return d.StringFixed(7)
}
