package engine

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/support/render/problem"
	"github.com/stretchr/testify/assert"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/stellar"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/utils"
)

func Test_TxProcessingLimiterImpl_AdjustLimitIfNeeded(t *testing.T) {
	currNumChannelAccounts := 50

	testCases := []struct {
		name       string
		err        utils.TransactionError
		wantResult *TransactionProcessingLimiterImpl
	}{
		// Horizon transaction error test cases
		{
			name: "adjusts limit for HorizonTransactionError with rate limit",
			err: &utils.HorizonTransactionError{
				HorizonErrorWrapper: utils.NewHorizonErrorWrapper(
					&horizonclient.Error{
						Problem: problem.P{Status: http.StatusTooManyRequests},
					},
				),
			},
			wantResult: &TransactionProcessingLimiterImpl{
				limitValue:                    DefaultBundlesSelectionLimit,
				IndeterminateResponsesCounter: IndeterminateResponsesToleranceLimit,
			},
		},
		{
			name: "adjusts limit for HorizonTransactionError with gateway timeout",
			err: &utils.HorizonTransactionError{
				HorizonErrorWrapper: utils.NewHorizonErrorWrapper(
					&horizonclient.Error{
						Problem: problem.P{Status: http.StatusGatewayTimeout},
					},
				),
			},
			wantResult: &TransactionProcessingLimiterImpl{
				limitValue:                    DefaultBundlesSelectionLimit,
				IndeterminateResponsesCounter: IndeterminateResponsesToleranceLimit,
			},
		},
		{
			name: "adjusts limit for HorizonTransactionError with tx_insufficient_fee",
			err: &utils.HorizonTransactionError{
				HorizonErrorWrapper: utils.NewHorizonErrorWrapper(
					&horizonclient.Error{
						Problem: problem.P{
							Status: http.StatusBadRequest,
							Extras: map[string]interface{}{
								"result_codes": map[string]interface{}{
									"transaction": "tx_insufficient_fee",
								},
							},
						},
					},
				),
			},
			wantResult: &TransactionProcessingLimiterImpl{
				limitValue:                    DefaultBundlesSelectionLimit,
				IndeterminateResponsesCounter: IndeterminateResponsesToleranceLimit,
			},
		},
		{
			name: "no adjustment for HorizonTransactionError with determinate error",
			err: &utils.HorizonTransactionError{
				HorizonErrorWrapper: utils.NewHorizonErrorWrapper(
					&horizonclient.Error{
						Problem: problem.P{
							Status: http.StatusBadRequest,
							Extras: map[string]interface{}{
								"result_codes": map[string]interface{}{
									"transaction": "tx_bad_auth",
								},
							},
						},
					},
				),
			},
			wantResult: &TransactionProcessingLimiterImpl{
				limitValue:                    currNumChannelAccounts,
				IndeterminateResponsesCounter: IndeterminateResponsesToleranceLimit - 1,
			},
		},
		// RPC transaction error test cases
		{
			name: "adjusts limit for RPCTransactionError with network error",
			err: &utils.RPCTransactionError{
				RPCErrorWrapper: &utils.RPCErrorWrapper{
					SimulationError: stellar.NewSimulationErrorWithType(stellar.SimulationErrorTypeNetwork, errors.New("network error"), nil),
				},
			},
			wantResult: &TransactionProcessingLimiterImpl{
				limitValue:                    DefaultBundlesSelectionLimit,
				IndeterminateResponsesCounter: IndeterminateResponsesToleranceLimit,
			},
		},
		{
			name: "adjusts limit for RPCTransactionError with resource error",
			err: &utils.RPCTransactionError{
				RPCErrorWrapper: &utils.RPCErrorWrapper{
					SimulationError: stellar.NewSimulationErrorWithType(stellar.SimulationErrorTypeResource, errors.New("cpu limit exceeded"), nil),
				},
			},
			wantResult: &TransactionProcessingLimiterImpl{
				limitValue:                    DefaultBundlesSelectionLimit,
				IndeterminateResponsesCounter: IndeterminateResponsesToleranceLimit,
			},
		},
		{
			name: "no adjustment for RPCTransactionError with contract execution error",
			err: &utils.RPCTransactionError{
				RPCErrorWrapper: &utils.RPCErrorWrapper{
					SimulationError: stellar.NewSimulationErrorWithType(stellar.SimulationErrorTypeContractExecution, errors.New("contract execution failed"), nil),
				},
			},
			wantResult: &TransactionProcessingLimiterImpl{
				limitValue:                    currNumChannelAccounts,
				IndeterminateResponsesCounter: IndeterminateResponsesToleranceLimit - 1,
			},
		},
		{
			name: "no adjustment for RPCTransactionError with auth error",
			err: &utils.RPCTransactionError{
				RPCErrorWrapper: &utils.RPCErrorWrapper{
					SimulationError: stellar.NewSimulationErrorWithType(stellar.SimulationErrorTypeAuth, errors.New("authorization failed"), nil),
				},
			},
			wantResult: &TransactionProcessingLimiterImpl{
				limitValue:                    currNumChannelAccounts,
				IndeterminateResponsesCounter: IndeterminateResponsesToleranceLimit - 1,
			},
		},
		{
			name: "no adjustment for RPCTransactionError with nil simulation error",
			err: &utils.RPCTransactionError{
				RPCErrorWrapper: &utils.RPCErrorWrapper{SimulationError: nil},
			},
			wantResult: &TransactionProcessingLimiterImpl{
				limitValue:                    currNumChannelAccounts,
				IndeterminateResponsesCounter: IndeterminateResponsesToleranceLimit - 1,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			txProcessingLimiter := &TransactionProcessingLimiterImpl{
				CurrNumChannelAccounts:        currNumChannelAccounts,
				limitValue:                    currNumChannelAccounts,
				IndeterminateResponsesCounter: IndeterminateResponsesToleranceLimit - 1,
				CounterLastUpdated:            time.Now(),
			}
			txProcessingLimiter.AdjustLimitIfNeeded(tc.err)

			assert.Equal(t, txProcessingLimiter.limitValue, tc.wantResult.limitValue)
			assert.Equal(t, txProcessingLimiter.IndeterminateResponsesCounter, tc.wantResult.IndeterminateResponsesCounter)
		})
	}
}

func Test_TxProcessingLimiterImpl_LimitValue(t *testing.T) {
	initialLimitValue := 100
	currNumChannelAccounts := 50

	testCases := []struct {
		name       string
		wait       func(tpl *TransactionProcessingLimiterImpl)
		wantResult *TransactionProcessingLimiterImpl
	}{
		{
			name: "no change when the time is before current window is complete",
			wait: func(tpl *TransactionProcessingLimiterImpl) {},
			wantResult: &TransactionProcessingLimiterImpl{
				limitValue:                    initialLimitValue,
				IndeterminateResponsesCounter: IndeterminateResponsesToleranceLimit - 1,
			},
		},
		{
			name: "change when the time is after current window is complete",
			wait: func(tpl *TransactionProcessingLimiterImpl) {
				tpl.CounterLastUpdated = tpl.CounterLastUpdated.Add(-10 * time.Minute)
			},
			wantResult: &TransactionProcessingLimiterImpl{
				limitValue:                    currNumChannelAccounts,
				IndeterminateResponsesCounter: 0,
			},
		},
	}

	for _, tc := range testCases {
		txProcessingLimiter := &TransactionProcessingLimiterImpl{
			CurrNumChannelAccounts:        currNumChannelAccounts,
			limitValue:                    initialLimitValue,
			IndeterminateResponsesCounter: IndeterminateResponsesToleranceLimit - 1,
			CounterLastUpdated:            time.Now(),
		}
		tc.wait(txProcessingLimiter)
		lv := txProcessingLimiter.LimitValue()

		assert.Equal(t, tc.wantResult.limitValue, txProcessingLimiter.limitValue)
		assert.Equal(t, tc.wantResult.IndeterminateResponsesCounter, txProcessingLimiter.IndeterminateResponsesCounter)
		assert.Equal(t, tc.wantResult.limitValue, lv)
	}
}
