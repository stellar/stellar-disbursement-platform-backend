package utils

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/protocols/horizon"
	"github.com/stellar/go/support/render/problem"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewTransactionStatusUpdateError(t *testing.T) {
	status := "ERROR"
	txID := "some-tx-id"
	forRetry := false
	err := fmt.Errorf("some error")
	txStatusUpdateErr := NewTransactionStatusUpdateError(status, txID, forRetry, err)

	wantTxStatusUpdateErr := &TransactionStatusUpdateError{
		Status:   status,
		TxID:     txID,
		ForRetry: forRetry,
		Err:      err,
	}
	require.Equal(t, wantTxStatusUpdateErr, txStatusUpdateErr)
}

func Test_TransactionStatusUpdateError_Error(t *testing.T) {
	testCases := []struct {
		name             string
		status           string
		txID             string
		forRetry         bool
		err              error
		wantStringResult string
	}{
		{
			name:             "PENDING for retry",
			status:           "PENDING",
			txID:             "foo",
			forRetry:         true,
			err:              fmt.Errorf("some causing error"),
			wantStringResult: "updating transaction(ID=\"foo\") status to PENDING (for retry): some causing error",
		},
		{
			name:             "ERROR without retry",
			status:           "ERROR",
			txID:             "bar",
			forRetry:         false,
			err:              fmt.Errorf("another causing error"),
			wantStringResult: "updating transaction(ID=\"bar\") status to ERROR: another causing error",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			txStatusUpdateErr := NewTransactionStatusUpdateError(tc.status, tc.txID, tc.forRetry, tc.err)
			require.Equal(t, tc.wantStringResult, txStatusUpdateErr.Error())
		})
	}
}

func Test_TransactionStatusUpdateError_Unwrap_and_Is(t *testing.T) {
	err := fmt.Errorf("some causing error")
	txStatusUpdateErr := NewTransactionStatusUpdateError("ERROR", "some-tx-id", false, err)
	require.Equal(t, err, txStatusUpdateErr.Unwrap())
	require.True(t, errors.Is(txStatusUpdateErr, err))
}

func Test_TransactionStatusUpdateError_As(t *testing.T) {
	err := fmt.Errorf("some causing error")
	var someError error = NewTransactionStatusUpdateError("ERROR", "some-tx-id", false, err)

	var txStatusUpdateErr *TransactionStatusUpdateError
	require.True(t, errors.As(someError, &txStatusUpdateErr))

	err = fmt.Errorf("sandwich the error: %w", txStatusUpdateErr)
	require.True(t, errors.As(err, &txStatusUpdateErr))
}

func Test_NewHorizonErrorWrapper(t *testing.T) {
	hError := horizonclient.Error{
		Problem: problem.P{
			Title:  "Transaction Failed",
			Type:   "transaction_failed",
			Status: http.StatusBadRequest,
			Detail: "",
			Extras: map[string]interface{}{
				"result_codes": map[string]interface{}{
					"transaction": "tx_failed",
					"operations":  []string{"op_underfunded"},
				},
			},
		},
	}

	testCases := []struct {
		name                   string
		originalErr            error
		wantHorizonResponseErr *HorizonErrorWrapper
	}{
		{
			name:                   "nil error",
			originalErr:            nil,
			wantHorizonResponseErr: nil,
		},
		{
			name:                   "non-horizon error",
			originalErr:            fmt.Errorf("some error"),
			wantHorizonResponseErr: &HorizonErrorWrapper{Err: fmt.Errorf("some error")},
		},
		{
			name:        "horizon error (value)",
			originalErr: hError,
			wantHorizonResponseErr: &HorizonErrorWrapper{
				StatusCode: http.StatusBadRequest,
				Problem:    hError.Problem,
				Err:        hError,
				ResultCodes: &horizon.TransactionResultCodes{
					TransactionCode:      "tx_failed",
					InnerTransactionCode: "",
					OperationCodes:       []string{"op_underfunded"},
				},
			},
		},
		{
			name:        "horizon error (pointer)",
			originalErr: &hError,
			wantHorizonResponseErr: &HorizonErrorWrapper{
				StatusCode: http.StatusBadRequest,
				Problem:    hError.Problem,
				Err:        &hError,
				ResultCodes: &horizon.TransactionResultCodes{
					TransactionCode:      "tx_failed",
					InnerTransactionCode: "",
					OperationCodes:       []string{"op_underfunded"},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			horizonResponseErr := NewHorizonErrorWrapper(tc.originalErr)
			require.Equal(t, tc.wantHorizonResponseErr, horizonResponseErr)
		})
	}
}

func Test_HorizonErrorWrapper_Error(t *testing.T) {
	testCases := []struct {
		name             string
		originalErr      error
		wantStringResult string
	}{
		{
			name:             "non-horizon error",
			originalErr:      fmt.Errorf("something went wrong with TCP IP stuff"),
			wantStringResult: "horizon response error: something went wrong with TCP IP stuff",
		},
		{
			name: "horizon error",
			originalErr: horizonclient.Error{
				Problem: problem.P{
					Title:  "Transaction Failed",
					Type:   "transaction_failed",
					Status: http.StatusBadRequest,
					Detail: "some-detail",
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{
							"transaction": "tx_failed",
							"operations":  []string{"op_underfunded"},
						},
					},
				},
			},
			wantStringResult: `horizon response error: StatusCode=400, Type=transaction_failed, Title=Transaction Failed, Detail=some-detail, Extras=transaction: tx_failed - operation codes: [ op_underfunded ]`,
		},
		{
			name: "horizon error with less fields",
			originalErr: horizonclient.Error{
				Problem: problem.P{
					Type:   "transaction_failed",
					Status: http.StatusBadRequest,
				},
			},
			wantStringResult: "horizon response error: StatusCode=400, Type=transaction_failed",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			txStatusUpdateErr := NewHorizonErrorWrapper(tc.originalErr)
			require.Equal(t, tc.wantStringResult, txStatusUpdateErr.Error())
		})
	}
}

func Test_HorizonErrorWrapper_Unwrap_and_Is(t *testing.T) {
	err := fmt.Errorf("some causing error")
	horizonErrorWrapper := NewHorizonErrorWrapper(err)
	require.Equal(t, err, horizonErrorWrapper.Unwrap())
	require.True(t, errors.Is(horizonErrorWrapper, err))
}

func Test_HorizonErrorWrapper_As(t *testing.T) {
	err := fmt.Errorf("some causing error")
	var someError error = NewHorizonErrorWrapper(err)

	var horizonErrorWrapper *HorizonErrorWrapper
	require.True(t, errors.As(someError, &horizonErrorWrapper))
	require.NotNil(t, horizonErrorWrapper)

	err = fmt.Errorf("sandwich the error: %w", horizonErrorWrapper)
	require.True(t, errors.As(err, &horizonErrorWrapper))
}

func Test_HorizonErrorWrapper_IsNotFound(t *testing.T) {
	testCases := []struct {
		name        string
		originalErr error
		wantResult  bool
	}{
		{
			name:        "non-horizon error, returns FALSE",
			originalErr: fmt.Errorf("something went wrong with TCP IP stuff"),
			wantResult:  false,
		},
		{
			name:        "400 horizon error, returns FALSE",
			originalErr: horizonclient.Error{Problem: problem.P{Status: http.StatusBadRequest}},
			wantResult:  false,
		},
		{
			name:        "404 horizon error, returns TRUE",
			originalErr: horizonclient.Error{Problem: problem.P{Status: http.StatusNotFound}},
			wantResult:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			txStatusUpdateErr := NewHorizonErrorWrapper(tc.originalErr)
			require.Equal(t, tc.wantResult, txStatusUpdateErr.IsNotFound())
		})
	}
}

func Test_HorizonErrorWrapper_IsRateLimit(t *testing.T) {
	testCases := []struct {
		name        string
		originalErr error
		wantResult  bool
	}{
		{
			name:        "non-horizon error, returns FALSE",
			originalErr: fmt.Errorf("something went wrong with TCP IP stuff"),
			wantResult:  false,
		},
		{
			name:        "400 horizon error, returns FALSE",
			originalErr: horizonclient.Error{Problem: problem.P{Status: http.StatusBadRequest}},
			wantResult:  false,
		},
		{
			name:        "429 horizon error, returns TRUE",
			originalErr: horizonclient.Error{Problem: problem.P{Status: http.StatusTooManyRequests}},
			wantResult:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			txStatusUpdateErr := NewHorizonErrorWrapper(tc.originalErr)
			require.Equal(t, tc.wantResult, txStatusUpdateErr.IsRateLimit())
		})
	}
}

func Test_HorizonErrorWrapper_IsGatewayTimeout(t *testing.T) {
	testCases := []struct {
		name        string
		originalErr error
		wantResult  bool
	}{
		{
			name:        "non-horizon error, returns FALSE",
			originalErr: fmt.Errorf("something went wrong with TCP IP stuff"),
			wantResult:  false,
		},
		{
			name:        "400 horizon error, returns FALSE",
			originalErr: horizonclient.Error{Problem: problem.P{Status: http.StatusBadRequest}},
			wantResult:  false,
		},
		{
			name:        "504 horizon error, returns TRUE",
			originalErr: horizonclient.Error{Problem: problem.P{Status: http.StatusGatewayTimeout}},
			wantResult:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			txStatusUpdateErr := NewHorizonErrorWrapper(tc.originalErr)
			require.Equal(t, tc.wantResult, txStatusUpdateErr.IsGatewayTimeout())
		})
	}
}

func Test_HorizonErrorWrapper_handleExtrasResultCodes(t *testing.T) {
	testCases := []struct {
		name       string
		hErr       error
		wantResult string
	}{
		{
			name: "doesn't write any content when there's no result codes",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{},
				},
			},
		},
		{
			name: "doesn't write any content when result codes is empty",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{},
					},
				},
			},
		},
		{
			name: "writes the content of transaction key",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{
							"transaction": "tx_fee_bump_inner_failed",
						},
					},
				},
			},
			wantResult: ", Extras=transaction: tx_fee_bump_inner_failed",
		},
		{
			name: "writes the content of inner_transaction key",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{
							"inner_transaction": "tx_too_early",
						},
					},
				},
			},
			wantResult: ", Extras=inner transaction: tx_too_early",
		},
		{
			name: "writes the content of operations key",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{
							"operations": []string{"op_failed_1", "op_failed_2", "op_failed_3"},
						},
					},
				},
			},
			wantResult: ", Extras=operation codes: [ op_failed_1, op_failed_2, op_failed_3 ]",
		},
		{
			name: "writes the content of transaction and inner_transaction keys",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{
							"transaction":       "tx_fee_bump_inner_failed",
							"inner_transaction": "tx_too_early",
						},
					},
				},
			},
			wantResult: ", Extras=transaction: tx_fee_bump_inner_failed - inner transaction: tx_too_early",
		},
		{
			name: "writes the content of all keys",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{
							"transaction":       "tx_fee_bump_inner_failed",
							"inner_transaction": "tx_too_early",
							"operations":        []string{"op_failed_1", "op_failed_2", "op_failed_3"},
						},
					},
				},
			},
			wantResult: ", Extras=transaction: tx_fee_bump_inner_failed - inner transaction: tx_too_early - operation codes: [ op_failed_1, op_failed_2, op_failed_3 ]",
		},
	}

	msgBuilder := new(strings.Builder)
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			wrapper := NewHorizonErrorWrapper(tc.hErr)
			wrapper.handleExtrasResultCodes(msgBuilder)

			if tc.wantResult == "" {
				assert.Empty(t, msgBuilder.String())
			} else {
				assert.Contains(t, msgBuilder.String(), tc.wantResult)
			}

			msgBuilder.Reset()
		})
	}
}

func Test_HorizonErrorWrapper_IsNotEnoughLumens(t *testing.T) {
	testCases := []struct {
		name       string
		hErr       error
		wantResult bool
	}{
		{
			name: "returns false when there's no result_codes",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{},
				},
			},
		},
		{
			name: "returns false when result_codes has no keys",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{},
					},
				},
			},
		},
		{
			name: "returns false when the result_codes is not related to tx_insufficient_balance or op_underfunded",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{
							"transaction":       "tx_fee_bump_inner_failed",
							"inner_transaction": "inner_tx_failed",
							"operations":        []string{"op_failed_1"},
						},
					},
				},
			},
			wantResult: false,
		},
		{
			name: "returns true when the transaction key is tx_insufficient_balance",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{
							"transaction": "tx_insufficient_balance",
						},
					},
				},
			},
			wantResult: true,
		},
		{
			name: "returns true when the inner_transaction key is tx_insufficient_balance",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{
							"transaction":       "tx_fee_bump_inner_failed",
							"inner_transaction": "tx_insufficient_balance",
						},
					},
				},
			},
			wantResult: true,
		},
		{
			name: "returns true when the operations key contains op_underfunded",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{
							"operations": []string{"op_underfunded"},
						},
					},
				},
			},
			wantResult: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			wrapper := NewHorizonErrorWrapper(tc.hErr)
			assert.Equal(t, tc.wantResult, wrapper.IsNotEnoughLumens())
		})
	}
}

func Test_HorizonErrorWrapper_IsNoSourceAccount(t *testing.T) {
	testCases := []struct {
		name       string
		hErr       error
		wantResult bool
	}{
		{
			name: "returns false when there's no result_codes",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{},
				},
			},
		},
		{
			name: "returns false when result_codes has no keys",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{},
					},
				},
			},
		},
		{
			name: "returns false when the result_codes is not related to tx_no_source_account or op_no_source_account",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{
							"transaction":       "tx_fee_bump_inner_failed",
							"inner_transaction": "inner_tx_failed",
							"operations":        []string{"op_failed"},
						},
					},
				},
			},
			wantResult: false,
		},
		{
			name: "returns true when the transaction key is tx_no_source_account",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{
							"transaction": "tx_no_source_account",
						},
					},
				},
			},
			wantResult: true,
		},
		{
			name: "returns true when the inner_transaction key is tx_no_source_account",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{
							"transaction":       "tx_fee_bump_inner_failed",
							"inner_transaction": "tx_no_source_account",
						},
					},
				},
			},
			wantResult: true,
		},
		{
			name: "returns true when the operations key contains op_no_source_account",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{
							"transaction":       "tx_fee_bump_inner_failed",
							"inner_transaction": "tx_inner_transaction",
							"operations":        []string{"op_no_source_account"},
						},
					},
				},
			},
			wantResult: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			wrapper := NewHorizonErrorWrapper(tc.hErr)
			assert.Equal(t, tc.wantResult, wrapper.IsNoSourceAccount())
		})
	}
}

func Test_HorizonErrorWrapper_IsNoIssuer(t *testing.T) {
	testCases := []struct {
		name       string
		hErr       error
		wantResult bool
	}{
		{
			name: "returns false when there's no result_codes",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{},
				},
			},
		},
		{
			name: "returns false when result_codes has no keys",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{},
					},
				},
			},
		},
		{
			name: "returns false when the result_codes is not related to op_no_issuer",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{
							"transaction":       "tx_fee_bump_inner_failed",
							"inner_transaction": "inner_tx_failed",
							"operations":        []string{"op_failed"},
						},
					},
				},
			},
			wantResult: false,
		},
		{
			name: "returns true when the operations key contains op_no_issuer",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{
							"operations": []string{"op_no_issuer"},
						},
					},
				},
			},
			wantResult: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			wrapper := NewHorizonErrorWrapper(tc.hErr)
			assert.Equal(t, tc.wantResult, wrapper.IsNoIssuer())
		})
	}
}

func Test_HorizonErrorWrapper_IsSourceAccountNotAuthorized(t *testing.T) {
	testCases := []struct {
		name       string
		hErr       error
		wantResult bool
	}{
		{
			name: "returns false when there's no result_codes",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{},
				},
			},
		},
		{
			name: "returns false when result_codes has no keys",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{},
					},
				},
			},
		},
		{
			name: "returns false when the result_codes is not related to op_src_not_authorized",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{
							"transaction":       "tx_fee_bump_inner_failed",
							"inner_transaction": "inner_tx_failed",
							"operations":        []string{"op_failed"},
						},
					},
				},
			},
			wantResult: false,
		},
		{
			name: "returns true when the operations key contains op_src_not_authorized",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{
							"operations": []string{"op_src_not_authorized"},
						},
					},
				},
			},
			wantResult: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			wrapper := NewHorizonErrorWrapper(tc.hErr)
			assert.Equal(t, tc.wantResult, wrapper.IsSourceAccountNotAuthorized())
		})
	}
}

func Test_HorizonErrorWrapper_IsSourceNoTrustline(t *testing.T) {
	testCases := []struct {
		name       string
		hErr       error
		wantResult bool
	}{
		{
			name: "returns false when there's no result_codes",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{},
				},
			},
		},
		{
			name: "returns false when result_codes has no keys",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{},
					},
				},
			},
		},
		{
			name: "returns false when the result_codes is not related to op_src_no_trust",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{
							"transaction":       "tx_fee_bump_inner_failed",
							"inner_transaction": "inner_tx_failed",
							"operations":        []string{"op_failed"},
						},
					},
				},
			},
			wantResult: false,
		},
		{
			name: "returns true when the operations key contains op_src_no_trust",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{
							"operations": []string{"op_src_no_trust"},
						},
					},
				},
			},
			wantResult: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			wrapper := NewHorizonErrorWrapper(tc.hErr)
			assert.Equal(t, tc.wantResult, wrapper.IsSourceNoTrustline())
		})
	}
}

func Test_HorizonErrorWrapper_IsDestinationAccountNotAuthorized(t *testing.T) {
	testCases := []struct {
		name       string
		hErr       error
		wantResult bool
	}{
		{
			name: "returns false when there's no result_codes",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{},
				},
			},
		},
		{
			name: "returns false when result_codes has no keys",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{},
					},
				},
			},
		},
		{
			name: "returns false when the result_codes is not related to op_not_authorized",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{
							"transaction":       "tx_fee_bump_inner_failed",
							"inner_transaction": "inner_tx_failed",
							"operations":        []string{"op_failed"},
						},
					},
				},
			},
			wantResult: false,
		},
		{
			name: "returns true when the operations key contains op_not_authorized",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{
							"operations": []string{"op_not_authorized"},
						},
					},
				},
			},
			wantResult: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			wrapper := NewHorizonErrorWrapper(tc.hErr)
			assert.Equal(t, tc.wantResult, wrapper.IsDestinationAccountNotAuthorized())
		})
	}
}

func Test_HorizonErrorWrapper_IsDestinationNoTrustline(t *testing.T) {
	testCases := []struct {
		name       string
		hErr       error
		wantResult bool
	}{
		{
			name: "returns false when there's no result_codes",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{},
				},
			},
		},
		{
			name: "returns false when result_codes has no keys",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{},
					},
				},
			},
		},
		{
			name: "returns false when the result_codes is not related to op_no_trust",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{
							"transaction":       "tx_fee_bump_inner_failed",
							"inner_transaction": "inner_tx_failed",
							"operations":        []string{"op_failed"},
						},
					},
				},
			},
			wantResult: false,
		},
		{
			name: "returns true when the operations key contains op_no_trust",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{
							"operations": []string{"op_no_trust"},
						},
					},
				},
			},
			wantResult: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			wrapper := NewHorizonErrorWrapper(tc.hErr)
			assert.Equal(t, tc.wantResult, wrapper.IsDestinationNoTrustline())
		})
	}
}

func Test_HorizonErrorWrapper_IsLineFull(t *testing.T) {
	testCases := []struct {
		name       string
		hErr       error
		wantResult bool
	}{
		{
			name: "returns false when there's no result_codes",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{},
				},
			},
		},
		{
			name: "returns false when result_codes has no keys",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{},
					},
				},
			},
		},
		{
			name: "returns false when the result_codes is not related to op_line_full",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{
							"transaction":       "tx_fee_bump_inner_failed",
							"inner_transaction": "inner_tx_failed",
							"operations":        []string{"op_failed"},
						},
					},
				},
			},
			wantResult: false,
		},
		{
			name: "returns true when the operations key contains op_line_full",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{
							"operations": []string{"op_line_full"},
						},
					},
				},
			},
			wantResult: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			wrapper := NewHorizonErrorWrapper(tc.hErr)
			assert.Equal(t, tc.wantResult, wrapper.IsLineFull())
		})
	}
}

func Test_HorizonErrorWrapper_IsNoDestinationAccount(t *testing.T) {
	testCases := []struct {
		name       string
		hErr       error
		wantResult bool
	}{
		{
			name: "returns false when there's no result_codes",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{},
				},
			},
		},
		{
			name: "returns false when result_codes has no keys",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{},
					},
				},
			},
		},
		{
			name: "returns false when the result_codes is not related to op_no_destination",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{
							"transaction":       "tx_fee_bump_inner_failed",
							"inner_transaction": "inner_tx_failed",
							"operations":        []string{"op_failed"},
						},
					},
				},
			},
			wantResult: false,
		},
		{
			name: "returns true when the operations key contains op_no_destination",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{
							"operations": []string{"op_no_destination"},
						},
					},
				},
			},
			wantResult: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			wrapper := NewHorizonErrorWrapper(tc.hErr)
			assert.Equal(t, tc.wantResult, wrapper.IsNoDestinationAccount())
		})
	}
}

func Test_HorizonErrorWrapper_IsBadAuthentication(t *testing.T) {
	testCases := []struct {
		name       string
		hErr       error
		wantResult bool
	}{
		{
			name: "returns false when there's no result_codes",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{},
				},
			},
		},
		{
			name: "returns false when result_codes has no keys",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{},
					},
				},
			},
		},
		{
			name: "returns false when the result_codes is not related to tx_bad_auth, tx_bad_auth_extra, or op_bad_auth",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{
							"transaction":       "tx_fee_bump_inner_failed",
							"inner_transaction": "inner_tx_failed",
							"operations":        []string{"op_failed"},
						},
					},
				},
			},
			wantResult: false,
		},
		{
			name: "returns true when the transaction key is tx_bad_auth",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{
							"transaction": "tx_bad_auth",
						},
					},
				},
			},
			wantResult: true,
		},
		{
			name: "returns true when the inner_transaction key is tx_bad_auth",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{
							"transaction":       "tx_fee_bump_inner_failed",
							"inner_transaction": "tx_bad_auth",
						},
					},
				},
			},
			wantResult: true,
		},
		{
			name: "returns true when the transaction key is tx_bad_auth_extra",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{
							"transaction": "tx_bad_auth_extra",
						},
					},
				},
			},
			wantResult: true,
		},
		{
			name: "returns true when the inner_transaction key is tx_bad_auth_extra",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{
							"transaction":       "tx_fee_bump_inner_failed",
							"inner_transaction": "tx_bad_auth_extra",
						},
					},
				},
			},
			wantResult: true,
		},
		{
			name: "returns true when the operations key contains op_bad_auth",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{
							"transaction":       "tx_fee_bump_inner_failed",
							"inner_transaction": "tx_inner_transaction",
							"operations":        []string{"op_bad_auth"},
						},
					},
				},
			},
			wantResult: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			wrapper := NewHorizonErrorWrapper(tc.hErr)
			assert.Equal(t, tc.wantResult, wrapper.IsBadAuthentication())
		})
	}
}

func Test_HorizonErrorWrapper_IsTxInsufficientFee(t *testing.T) {
	testCases := []struct {
		name       string
		hErr       error
		wantResult bool
	}{
		{
			name: "returns false when there's no result_codes",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{},
				},
			},
		},
		{
			name: "returns false when result_codes has no keys",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{},
					},
				},
			},
		},
		{
			name: "returns false when the result_codes is not related to tx_insufficient_fee",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{
							"transaction":       "tx_fee_bump_inner_failed",
							"inner_transaction": "inner_tx_failed",
							"operations":        []string{"op_failed"},
						},
					},
				},
			},
		},
		{
			name: "returns true when the transaction key is tx_insufficient_fee",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{
							"transaction": "tx_insufficient_fee",
						},
					},
				},
			},
			wantResult: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			wrapper := NewHorizonErrorWrapper(tc.hErr)
			assert.Equal(t, tc.wantResult, wrapper.IsTxInsufficientFee())
		})
	}
}

func Test_HorizonErrorWrapper_IsSourceAccountNotReady(t *testing.T) {
	testCases := []struct {
		name       string
		hErr       error
		wantResult bool
	}{
		{
			name: "returns false when there's no result_codes",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{},
				},
			},
		},
		{
			name: "returns false when result_codes has no keys",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{},
					},
				},
			},
		},
		{
			name: "returns false when the result_codes is not related to any Source Account misconfiguration",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{
							"transaction":       "tx_fee_bump_inner_failed",
							"inner_transaction": "inner_tx_failed",
							"operations":        []string{"op_failed"},
						},
					},
				},
			},
			wantResult: false,
		},
		{
			name: "returns true when source account has not enough lumens",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{
							"transaction":       "tx_fee_bump_inner_failed",
							"inner_transaction": "tx_insufficient_balance",
						},
					},
				},
			},
			wantResult: true,
		},
		{
			name: "returns true when the source account does not exist",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{
							"transaction":       "tx_fee_bump_inner_failed",
							"inner_transaction": "tx_no_source_account",
							"operations":        []string{"op_no_source_account"},
						},
					},
				},
			},
			wantResult: true,
		},
		{
			name: "returns true when the source account is not authorized to send the asset",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{
							"transaction": "tx_fee_bump_inner_failed",
							"operations":  []string{"op_src_not_authorized"},
						},
					},
				},
			},
			wantResult: true,
		},
		{
			name: "returns true when the source account does not have trustline for the asset",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{
							"transaction": "tx_fee_bump_inner_failed",
							"operations":  []string{"op_src_no_trust"},
						},
					},
				},
			},
			wantResult: true,
		},
		{
			name: "returns true when the source account is underfunded",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{
							"transaction": "tx_fee_bump_inner_failed",
							"operations":  []string{"op_underfunded"},
						},
					},
				},
			},
			wantResult: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			wrapper := NewHorizonErrorWrapper(tc.hErr)
			assert.Equal(t, tc.wantResult, wrapper.IsSourceAccountNotReady())
		})
	}
}

func Test_HorizonErrorWrapper_IsDestinationAccountNotReady(t *testing.T) {
	testCases := []struct {
		name       string
		hErr       error
		wantResult bool
	}{
		{
			name: "returns false when there's no result_codes",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{},
				},
			},
		},
		{
			name: "returns false when result_codes has no keys",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{},
					},
				},
			},
		},
		{
			name: "returns false when the result_codes is not related to any Destination Account misconfiguration",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{
							"transaction":       "tx_fee_bump_inner_failed",
							"inner_transaction": "inner_tx_failed",
							"operations":        []string{"op_failed"},
						},
					},
				},
			},
			wantResult: false,
		},
		{
			name: "returns true when destination account is not authorized to receive the asset",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{
							"transaction": "tx_fee_bump_inner_failed",
							"operations":  []string{"op_not_authorized"},
						},
					},
				},
			},
			wantResult: true,
		},
		{
			name: "returns true when the destination account has no trustline for the asset",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{
							"transaction": "tx_fee_bump_inner_failed",
							"operations":  []string{"op_no_trust"},
						},
					},
				},
			},
			wantResult: true,
		},
		{
			name: "returns true when the destination account does not exist",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{
							"transaction": "tx_fee_bump_inner_failed",
							"operations":  []string{"op_no_destination"},
						},
					},
				},
			},
			wantResult: true,
		},
		{
			name: "returns true when the destination account has no sufficient limit",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{
							"transaction": "tx_fee_bump_inner_failed",
							"operations":  []string{"op_line_full"},
						},
					},
				},
			},
			wantResult: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			wrapper := NewHorizonErrorWrapper(tc.hErr)
			assert.Equal(t, tc.wantResult, wrapper.IsDestinationAccountNotReady())
		})
	}
}

func Test_HorizonErrorWrapper_ShouldMarkAsError(t *testing.T) {
	testCases := []struct {
		name       string
		hErr       error
		wantResult bool
	}{
		{
			name: "returns true if tx code in failed tx codes",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{
							"transaction": "tx_bad_auth",
						},
					},
				},
			},
			wantResult: true,
		},
		{
			name: "returns true if op code in failed op codes",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{
							"transaction": "tx_fee_bump_inner_failed",
							"operations":  []string{"op_no_destination"},
						},
					},
				},
			},
			wantResult: true,
		},
		{
			name: "returns false if tx code not in failed tx codes",
			hErr: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{
							"transaction": "tx_insufficient_fee",
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			wrapper := NewHorizonErrorWrapper(tc.hErr)
			assert.Equal(t, tc.wantResult, wrapper.ShouldMarkAsError())
		})
	}
}
