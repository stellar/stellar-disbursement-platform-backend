package engine

import (
	"net/http"
	"testing"
	"time"

	"github.com/stellar/go-stellar-sdk/clients/horizonclient"
	"github.com/stellar/go-stellar-sdk/support/render/problem"
	"github.com/stretchr/testify/assert"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/utils"
)

func Test_TxProcessingLimiterImpl_AdjustLimitIfNeeded(t *testing.T) {
	currNumChannelAccounts := 50

	testCases := []struct {
		name       string
		hErr       *utils.HorizonErrorWrapper
		wantResult *TransactionProcessingLimiterImpl
	}{
		{
			name: "adjusts limit if the horizon client error is too_many_requests",
			hErr: utils.NewHorizonErrorWrapper(
				&horizonclient.Error{
					Problem: problem.P{Status: http.StatusTooManyRequests},
				},
			),
			wantResult: &TransactionProcessingLimiterImpl{
				limitValue:                    DefaultBundlesSelectionLimit,
				IndeterminateResponsesCounter: IndeterminateResponsesToleranceLimit,
			},
		},
		{
			name: "adjusts limit if the horizon client error is gateway_timeout",
			hErr: utils.NewHorizonErrorWrapper(
				&horizonclient.Error{
					Problem: problem.P{Status: http.StatusGatewayTimeout},
				},
			),
			wantResult: &TransactionProcessingLimiterImpl{
				limitValue:                    DefaultBundlesSelectionLimit,
				IndeterminateResponsesCounter: IndeterminateResponsesToleranceLimit,
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
			wantResult: &TransactionProcessingLimiterImpl{
				limitValue:                    DefaultBundlesSelectionLimit,
				IndeterminateResponsesCounter: IndeterminateResponsesToleranceLimit,
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
			txProcessingLimiter.AdjustLimitIfNeeded(tc.hErr)

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
