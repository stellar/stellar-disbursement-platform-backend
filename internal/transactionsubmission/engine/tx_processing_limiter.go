package engine

import (
	"sync"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/utils"
)

const (
	defaultBundlesSelectionLimit         = 8
	indeterminateResponsesToleranceLimit = 10
	minutesInWindow                      = 3
)

// TransactionProcessingLimiter is utilized by the manager and transaction worker to share metadata about and adjust
// the rate at which tss processes transactions based on responses from Horizon.
type TransactionProcessingLimiter struct {
	CurrNumChannelAccounts        int
	IndeterminateResponsesCounter int
	CounterLastUpdated            time.Time
	limitValue                    int
	mutex                         sync.Mutex
}

func NewTransactionProcessingLimiter(limit int) *TransactionProcessingLimiter {
	if limit < 0 {
		limit = defaultBundlesSelectionLimit
	}

	return &TransactionProcessingLimiter{
		CurrNumChannelAccounts:        limit,
		IndeterminateResponsesCounter: 0,
		CounterLastUpdated:            time.Now(),
		limitValue:                    limit,
	}
}

// AdjustLimitIfNeeded re-establishes the transaction processing limit based on how many transactions result in
// - `504`, 429`, `400` - tx_insufficient_fee` which are indicators for network congestion causing a cascade of further
// transaction failures and need for retries.
func (tpl *TransactionProcessingLimiter) AdjustLimitIfNeeded(hErr *utils.HorizonErrorWrapper) {
	tpl.mutex.Lock()
	defer tpl.mutex.Unlock()

	if !(hErr.IsRateLimit() || hErr.IsGatewayTimeout() || hErr.IsTxInsufficientFee()) {
		return
	}

	tpl.IndeterminateResponsesCounter++
	// We can tweek the following values as needed, and maybe add additional functionality to
	// dynamically determine values for the default selection limit rather than using the default harcoded values
	if tpl.IndeterminateResponsesCounter >= indeterminateResponsesToleranceLimit {
		tpl.limitValue = defaultBundlesSelectionLimit
		tpl.CounterLastUpdated = time.Now()
	}
}

// LimitValue resets the necessary counter-related values when the current time is well outside the fixed
// window of the last refresh, and serves as a getter for the `limitValue` field.
func (tpl *TransactionProcessingLimiter) LimitValue() int {
	tpl.mutex.Lock()
	defer tpl.mutex.Unlock()
	// refresh counter on a fixed window basis
	now := time.Now()
	if now.After(tpl.CounterLastUpdated.Add(minutesInWindow * time.Minute)) {
		tpl.IndeterminateResponsesCounter = 0
		tpl.CounterLastUpdated = now
		tpl.limitValue = tpl.CurrNumChannelAccounts
	}

	return tpl.limitValue
}
