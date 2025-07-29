package engine

import (
	"errors"
	"sync"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/utils"
)

const (
	DefaultBundlesSelectionLimit         = 8
	IndeterminateResponsesToleranceLimit = 10
	MinutesInWindow                      = 3
)

// TransactionProcessingLimiter is an interface that defines the methods that the manager and transaction worker use to
// share metadata about and adjust the rate at which transactions are processed based on responses from Horizon and RPC.
//
//go:generate mockery --name=TransactionProcessingLimiter --case=underscore --structname=MockTransactionProcessingLimiter
type TransactionProcessingLimiter interface {
	// AdjustLimitIfNeeded is used to temporarily adjust the limitValue variable, returned by the LimitValue() getter,
	// if it starts seeing a high number of indeterminate responses from Horizon or RPC, which are indicative of network
	// congestion. The following error codes are considered indeterminate responses:
	//   Horizon:
	//   - 504: Timeout
	//   - 429: Too Many Requests
	//   - 400 - tx_insufficient_fee: Bad Request
	//   RPC:
	//   - Network errors (connection timeouts, DNS failures)
	//   - Rate limit errors (429)
	//   - Resource errors (CPU/memory limits)
	AdjustLimitIfNeeded(err utils.TransactionError)
	// LimitValue returns the current value of the limitValue variable, which is used to determine the number of channel
	// accounts to process transactions for in a single iteration. If the value being returned was downsized due to
	// indeterminate responses, the method will restore it to the original value after a fixed window of time has
	// passed.
	LimitValue() int
}

var _ TransactionProcessingLimiter = (*TransactionProcessingLimiterImpl)(nil)

// TransactionProcessingLimiter is an interface that defines the methods that the manager and transaction worker use to
// share metadata about and adjust the rate at which transactions are processed based on responses from Horizon.
type TransactionProcessingLimiterImpl struct {
	CurrNumChannelAccounts        int
	IndeterminateResponsesCounter int
	CounterLastUpdated            time.Time
	limitValue                    int
	mutex                         sync.Mutex
}

func NewTransactionProcessingLimiter(limit int) *TransactionProcessingLimiterImpl {
	if limit < 0 {
		limit = DefaultBundlesSelectionLimit
	}

	return &TransactionProcessingLimiterImpl{
		CurrNumChannelAccounts:        limit,
		IndeterminateResponsesCounter: 0,
		CounterLastUpdated:            time.Now(),
		limitValue:                    limit,
	}
}

// shouldAdjustLimitForHorizonError determines if a Horizon error should trigger limit adjustment
func shouldAdjustLimitForHorizonError(hErr *utils.HorizonErrorWrapper) bool {
	return hErr.IsRateLimit() || hErr.IsGatewayTimeout() || hErr.IsTxInsufficientFee()
}

// shouldAdjustLimitForRPCError determines if an RPC error should trigger limit adjustment
// These are errors that indicate network congestion or resource constraints
func shouldAdjustLimitForRPCError(rpcErr *utils.RPCErrorWrapper) bool {
	return rpcErr.IsRetryable()
}

func (tpl *TransactionProcessingLimiterImpl) adjustLimitIfNeeded() {
	tpl.mutex.Lock()
	defer tpl.mutex.Unlock()

	tpl.IndeterminateResponsesCounter++
	// We can tweak the following values as needed, and maybe add additional functionality to
	// dynamically determine values for the default selection limit rather than using the default hardcoded values
	if tpl.IndeterminateResponsesCounter >= IndeterminateResponsesToleranceLimit {
		tpl.limitValue = DefaultBundlesSelectionLimit
		tpl.CounterLastUpdated = time.Now()
	}
}

func (tpl *TransactionProcessingLimiterImpl) AdjustLimitIfNeeded(err utils.TransactionError) {
	var horizonErr *utils.HorizonErrorWrapper
	var rpcErr *utils.RPCErrorWrapper

	if errors.As(err, &horizonErr) {
		if horizonErr.IsHorizonError() {
			if shouldAdjustLimitForHorizonError(horizonErr) {
				tpl.adjustLimitIfNeeded()
			}
		}
	} else if errors.As(err, &rpcErr) {
		if rpcErr.IsRPCError() {
			if shouldAdjustLimitForRPCError(rpcErr) {
				tpl.adjustLimitIfNeeded()
			}
		}
	}
}

func (tpl *TransactionProcessingLimiterImpl) LimitValue() int {
	tpl.mutex.Lock()
	defer tpl.mutex.Unlock()
	// refresh counter on a fixed window basis
	now := time.Now()
	if now.After(tpl.CounterLastUpdated.Add(MinutesInWindow * time.Minute)) {
		tpl.IndeterminateResponsesCounter = 0
		tpl.CounterLastUpdated = now
		tpl.limitValue = tpl.CurrNumChannelAccounts
	}

	return tpl.limitValue
}
