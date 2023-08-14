package engine

import (
	"net/http"
	"testing"
	"time"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/support/render/problem"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/utils"
	"github.com/stretchr/testify/assert"
)

func Test_TxProcessingLimiter_AdjustLimitIfNeeded(t *testing.T) {
	currNumChannelAccounts := 50

	testCases := []struct {
		name       string
		hErr       *utils.HorizonErrorWrapper
		wantResult *TransactionProcessingLimiter
	}{
		{
			name: "adjusts limit if the horizon client error is too_many_requests",
			hErr: utils.NewHorizonErrorWrapper(
				&horizonclient.Error{
					Problem: problem.P{Status: http.StatusTooManyRequests},
				},
			),
			wantResult: &TransactionProcessingLimiter{
				limitValue:                    defaultBundlesSelectionLimit,
				IndeterminateResponsesCounter: indeterminateResponsesToleranceLimit,
			},
		},
		{
			name: "adjusts limit if the horizon client error is gateway_timeout",
			hErr: utils.NewHorizonErrorWrapper(
				&horizonclient.Error{
					Problem: problem.P{Status: http.StatusGatewayTimeout},
				},
			),
			wantResult: &TransactionProcessingLimiter{
				limitValue:                    defaultBundlesSelectionLimit,
				IndeterminateResponsesCounter: indeterminateResponsesToleranceLimit,
			},
		},
		{
			name: "adjusts limit if one of the operation error is tx_insufficient_fee",
			hErr: utils.NewHorizonErrorWrapper(
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
			wantResult: &TransactionProcessingLimiter{
				limitValue:                    defaultBundlesSelectionLimit,
				IndeterminateResponsesCounter: indeterminateResponsesToleranceLimit,
			},
		},
		{
			name: "no adjustment for determinate error",
			hErr: utils.NewHorizonErrorWrapper(
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
			wantResult: &TransactionProcessingLimiter{
				limitValue:                    currNumChannelAccounts,
				IndeterminateResponsesCounter: indeterminateResponsesToleranceLimit - 1,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			txProcessingLimiter := &TransactionProcessingLimiter{
				CurrNumChannelAccounts:        currNumChannelAccounts,
				limitValue:                    currNumChannelAccounts,
				IndeterminateResponsesCounter: indeterminateResponsesToleranceLimit - 1,
				CounterLastUpdated:            time.Now(),
			}
			txProcessingLimiter.AdjustLimitIfNeeded(tc.hErr)

			assert.Equal(t, txProcessingLimiter.limitValue, tc.wantResult.limitValue)
			assert.Equal(t, txProcessingLimiter.IndeterminateResponsesCounter, tc.wantResult.IndeterminateResponsesCounter)
		})
	}
}

func Test_TxProcessingLimiter_LimitValue(t *testing.T) {
	initialLimitValue := 100
	currNumChannelAccounts := 50

	testCases := []struct {
		name       string
		wait       func(tpl *TransactionProcessingLimiter)
		wantResult *TransactionProcessingLimiter
	}{
		{
			name: "no change when the time is before current window is complete",
			wait: func(tpl *TransactionProcessingLimiter) {},
			wantResult: &TransactionProcessingLimiter{
				limitValue:                    initialLimitValue,
				IndeterminateResponsesCounter: indeterminateResponsesToleranceLimit - 1,
			},
		},
		{
			name: "change when the time is after current window is complete",
			wait: func(tpl *TransactionProcessingLimiter) {
				tpl.CounterLastUpdated = tpl.CounterLastUpdated.Add(-10 * time.Minute)
			},
			wantResult: &TransactionProcessingLimiter{
				limitValue:                    currNumChannelAccounts,
				IndeterminateResponsesCounter: 0,
			},
		},
	}

	for _, tc := range testCases {
		txProcessingLimiter := &TransactionProcessingLimiter{
			CurrNumChannelAccounts:        currNumChannelAccounts,
			limitValue:                    initialLimitValue,
			IndeterminateResponsesCounter: indeterminateResponsesToleranceLimit - 1,
			CounterLastUpdated:            time.Now(),
		}
		tc.wait(txProcessingLimiter)
		lv := txProcessingLimiter.LimitValue()

		assert.Equal(t, tc.wantResult.limitValue, txProcessingLimiter.limitValue)
		assert.Equal(t, tc.wantResult.IndeterminateResponsesCounter, txProcessingLimiter.IndeterminateResponsesCounter)
		assert.Equal(t, tc.wantResult.limitValue, lv)
	}
}
