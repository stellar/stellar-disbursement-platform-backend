package httphandler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/network"
	"github.com/stellar/go/txnbuild"
	"github.com/stellar/go/xdr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sorobanutils"
)

func txXDR(t *testing.T, tx *txnbuild.GenericTransaction) string {
	t.Helper()

	if feeBumpTx, ok := tx.FeeBump(); ok {
		feeBumpXDR, err := feeBumpTx.Base64()
		require.NoError(t, err)
		return feeBumpXDR
	}

	if tx, ok := tx.Transaction(); ok {
		txXDR, err := tx.Base64()
		require.NoError(t, err)
		return txXDR
	}

	panic("transaction is not a fee bump transaction or a simple transaction")
}

func createFeeBumpTx(t *testing.T) *txnbuild.GenericTransaction {
	t.Helper()

	kp := keypair.MustRandom()
	genericTx := createMultiOpTx(t)
	tx, ok := genericTx.Transaction()
	require.True(t, ok, "transaction is not a simple transaction")

	feeBumpTx, err := txnbuild.NewFeeBumpTransaction(txnbuild.FeeBumpTransactionParams{
		Inner:      tx,
		FeeAccount: kp.Address(),
		BaseFee:    txnbuild.MinBaseFee,
	})
	require.NoError(t, err)

	return txnbuild.NewGenericTransactionWithFeeBumpTransaction(feeBumpTx)
}

func createMultiOpTx(t *testing.T) *txnbuild.GenericTransaction {
	t.Helper()

	kp := keypair.MustRandom()
	tx, err := txnbuild.NewTransaction(txnbuild.TransactionParams{
		SourceAccount: &txnbuild.SimpleAccount{
			AccountID: kp.Address(),
			Sequence:  1,
		},
		Operations: []txnbuild.Operation{
			&txnbuild.Payment{
				Destination: kp.Address(),
				Amount:      "10",
				Asset:       txnbuild.NativeAsset{},
			},
			&txnbuild.Payment{
				Destination: kp.Address(),
				Amount:      "20",
				Asset:       txnbuild.NativeAsset{},
			},
		},
		BaseFee: txnbuild.MinBaseFee,
		Preconditions: txnbuild.Preconditions{
			TimeBounds: txnbuild.NewTimeout(300),
		},
	})
	require.NoError(t, err)

	return txnbuild.NewGenericTransactionWithTransaction(tx)
}

func createNonInvokeHostFunctionTx(t *testing.T) *txnbuild.GenericTransaction {
	t.Helper()

	kp := keypair.MustRandom()
	tx, err := txnbuild.NewTransaction(txnbuild.TransactionParams{
		SourceAccount: &txnbuild.SimpleAccount{
			AccountID: kp.Address(),
			Sequence:  1,
		},
		Operations: []txnbuild.Operation{
			&txnbuild.Payment{
				Destination: kp.Address(),
				Amount:      "10",
				Asset:       txnbuild.NativeAsset{},
			},
		},
		BaseFee: txnbuild.MinBaseFee,
		Preconditions: txnbuild.Preconditions{
			TimeBounds: txnbuild.NewTimeout(300),
		},
	})
	require.NoError(t, err)

	return txnbuild.NewGenericTransactionWithTransaction(tx)
}

func createNonContractInvocationTx(t *testing.T) *txnbuild.GenericTransaction {
	t.Helper()

	kp := keypair.MustRandom()
	tx, err := txnbuild.NewTransaction(txnbuild.TransactionParams{
		SourceAccount: &txnbuild.SimpleAccount{
			AccountID: kp.Address(),
			Sequence:  1,
		},
		Operations: []txnbuild.Operation{
			&txnbuild.InvokeHostFunction{
				HostFunction: xdr.HostFunction{
					Type: xdr.HostFunctionTypeHostFunctionTypeCreateContract,
					CreateContract: &xdr.CreateContractArgs{
						ContractIdPreimage: xdr.ContractIdPreimage{
							Type:      xdr.ContractIdPreimageTypeContractIdPreimageFromAsset,
							FromAsset: &xdr.Asset{Type: xdr.AssetTypeAssetTypeNative},
						},
						Executable: xdr.ContractExecutable{
							Type: xdr.ContractExecutableTypeContractExecutableStellarAsset,
						},
					},
				},
				Ext: xdr.TransactionExt{},
			},
		},
		BaseFee: txnbuild.MinBaseFee,
		Preconditions: txnbuild.Preconditions{
			TimeBounds: txnbuild.NewTimeout(300),
		},
	})
	require.NoError(t, err)

	return txnbuild.NewGenericTransactionWithTransaction(tx)
}

func createContractInvocationTx(t *testing.T, contractID, functionName string, withSubinvocations bool) *txnbuild.GenericTransaction {
	t.Helper()

	opts := sorobanutils.InvokeContractOptions{
		ContractID:   contractID,
		FunctionName: functionName,
		Args:         []xdr.ScVal{},
	}
	invokeOp, err := sorobanutils.CreateContractInvocationOp(opts)
	require.NoError(t, err)

	if withSubinvocations {
		invokeContract := invokeOp.HostFunction.MustInvokeContract()
		invokeOp.Auth = []xdr.SorobanAuthorizationEntry{
			{
				RootInvocation: xdr.SorobanAuthorizedInvocation{
					Function: xdr.SorobanAuthorizedFunction{
						Type: xdr.SorobanAuthorizedFunctionTypeSorobanAuthorizedFunctionTypeContractFn,
						ContractFn: &xdr.InvokeContractArgs{
							ContractAddress: invokeContract.ContractAddress,
							FunctionName:    xdr.ScSymbol(opts.FunctionName),
							Args:            opts.Args,
						},
					},
					SubInvocations: []xdr.SorobanAuthorizedInvocation{
						{
							Function: xdr.SorobanAuthorizedFunction{
								Type: xdr.SorobanAuthorizedFunctionTypeSorobanAuthorizedFunctionTypeCreateContractHostFn,
								CreateContractHostFn: &xdr.CreateContractArgs{
									ContractIdPreimage: xdr.ContractIdPreimage{
										Type:      xdr.ContractIdPreimageTypeContractIdPreimageFromAsset,
										FromAsset: &xdr.Asset{Type: xdr.AssetTypeAssetTypeNative},
									},
									Executable: xdr.ContractExecutable{
										Type: xdr.ContractExecutableTypeContractExecutableStellarAsset,
									},
								},
							},
						},
					},
				},
			},
		}
	}

	kp := keypair.MustRandom()
	tx, err := txnbuild.NewTransaction(txnbuild.TransactionParams{
		SourceAccount: &txnbuild.SimpleAccount{
			AccountID: kp.Address(),
			Sequence:  1,
		},
		Operations: []txnbuild.Operation{&invokeOp},
		BaseFee:    txnbuild.MinBaseFee,
		Preconditions: txnbuild.Preconditions{
			TimeBounds: txnbuild.NewTimeout(300),
		},
	})
	require.NoError(t, err)

	return txnbuild.NewGenericTransactionWithTransaction(tx)
}

func Test_parseAndValidateTransaction(t *testing.T) {
	testCases := []struct {
		name               string
		txXDR              string
		intendedContractID string
		wantErrContains    string
	}{
		{
			name:               "🔴empty_transaction_xdr",
			txXDR:              "",
			intendedContractID: "CA3D5KRYM6CB7OWQ6TWYRR3Z4T7GNZLKERYNZGGA5SOAOPIFY6YQGAXE",
			wantErrContains:    "transaction_xdr cannot be empty",
		},
		{
			name:               "🔴invalid_transaction_xdr",
			txXDR:              "invalid_xdr_string",
			intendedContractID: "CA3D5KRYM6CB7OWQ6TWYRR3Z4T7GNZLKERYNZGGA5SOAOPIFY6YQGAXE",
			wantErrContains:    "invalid transaction XDR:",
		},
		{
			name:               "🔴fee_bump_transaction",
			txXDR:              txXDR(t, createFeeBumpTx(t)),
			intendedContractID: "CA3D5KRYM6CB7OWQ6TWYRR3Z4T7GNZLKERYNZGGA5SOAOPIFY6YQGAXE",
			wantErrContains:    "generic transaction could not be converted into transaction, ensure you're not sending a fee bump transaction",
		},
		{
			name:               "🔴multiple_operations",
			txXDR:              txXDR(t, createMultiOpTx(t)),
			intendedContractID: "CA3D5KRYM6CB7OWQ6TWYRR3Z4T7GNZLKERYNZGGA5SOAOPIFY6YQGAXE",
			wantErrContains:    "transaction must have exactly one operation",
		},
		{
			name:               "🔴not_invoke_host_function",
			txXDR:              txXDR(t, createNonInvokeHostFunctionTx(t)),
			intendedContractID: "CA3D5KRYM6CB7OWQ6TWYRR3Z4T7GNZLKERYNZGGA5SOAOPIFY6YQGAXE",
			wantErrContains:    "transaction operation is not an invoke host function",
		},
		{
			name:               "🔴not_contract_invocation",
			txXDR:              txXDR(t, createNonContractInvocationTx(t)),
			intendedContractID: "CA3D5KRYM6CB7OWQ6TWYRR3Z4T7GNZLKERYNZGGA5SOAOPIFY6YQGAXE",
			wantErrContains:    "invoke host function is not a contract invocation",
		},
		{
			name:               "🔴wrong_contract_being_invoked",
			txXDR:              txXDR(t, createContractInvocationTx(t, "CBXYGEUV46XRDLQU4MIIOEYGRLGPDHDBTLYUWBQP2OLFMWKJUBH46QXF", RotateSignerFnName, false)),
			intendedContractID: "CA3D5KRYM6CB7OWQ6TWYRR3Z4T7GNZLKERYNZGGA5SOAOPIFY6YQGAXE",
			wantErrContains:    "wrong contract being invoked",
		},
		{
			name:               "🔴wrong_function_being_invoked",
			txXDR:              txXDR(t, createContractInvocationTx(t, "CA3D5KRYM6CB7OWQ6TWYRR3Z4T7GNZLKERYNZGGA5SOAOPIFY6YQGAXE", "foo_bar", false)),
			intendedContractID: "CA3D5KRYM6CB7OWQ6TWYRR3Z4T7GNZLKERYNZGGA5SOAOPIFY6YQGAXE",
			wantErrContains:    "wrong function being called",
		},
		{
			name:               "🔴subinvocations_not_allowed",
			txXDR:              txXDR(t, createContractInvocationTx(t, "CA3D5KRYM6CB7OWQ6TWYRR3Z4T7GNZLKERYNZGGA5SOAOPIFY6YQGAXE", RotateSignerFnName, true)),
			intendedContractID: "CA3D5KRYM6CB7OWQ6TWYRR3Z4T7GNZLKERYNZGGA5SOAOPIFY6YQGAXE",
			wantErrContains:    "contract operation has subinvocations which are not allowed",
		},
		{
			name:               "🟢valid_transaction",
			txXDR:              txXDR(t, createContractInvocationTx(t, "CA3D5KRYM6CB7OWQ6TWYRR3Z4T7GNZLKERYNZGGA5SOAOPIFY6YQGAXE", RotateSignerFnName, false)),
			intendedContractID: "CA3D5KRYM6CB7OWQ6TWYRR3Z4T7GNZLKERYNZGGA5SOAOPIFY6YQGAXE",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseAndValidateTransaction(tc.txXDR, tc.intendedContractID)
			if tc.wantErrContains != "" {
				assert.ErrorContains(t, err, tc.wantErrContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_CosignRecoveryHandler_CosignRecovery(t *testing.T) {
	// Setup
	const contractAddress = "CA3D5KRYM6CB7OWQ6TWYRR3Z4T7GNZLKERYNZGGA5SOAOPIFY6YQGAXE"
	const networkPassphrase = network.TestNetworkPassphrase
	recoveryKP := keypair.MustRandom()

	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	data.CreateEmbeddedWalletFixture(t, context.Background(), dbConnectionPool,
		"test-token", "test-hash", contractAddress, "test-credential",
		"test@example.com", "EMAIL", data.PendingWalletStatus)

	// Create a valid transaction for testing
	tx := createContractInvocationTx(t, contractAddress, RotateSignerFnName, false)
	txXDR := txXDR(t, tx)

	// Create handler
	handler := CosignRecoveryHandler{
		Models:            models,
		RotationSecretKey: recoveryKP.Seed(),
		NetworkPassphrase: networkPassphrase,
	}

	// Create router
	r := chi.NewRouter()
	r.Post("/cosign-recovery/{contractAddress}", handler.CosignRecovery)

	testCases := []struct {
		name            string
		txXDR           string
		wantStatus      int
		wantErrContains string
	}{
		{
			name:            "🔴invalid_transaction_xdr",
			txXDR:           "invalid_xdr_string",
			wantStatus:      http.StatusBadRequest,
			wantErrContains: "invalid transaction XDR:",
		},
		{
			name:       "🟢successful_cosign_recovery",
			txXDR:      txXDR,
			wantStatus: http.StatusOK,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create & execute request
			bodyBytes, err := json.Marshal(CosignRecoveryRequest{TransactionXDR: tc.txXDR})
			require.NoError(t, err)
			req := httptest.NewRequest(http.MethodPost, "/cosign-recovery/"+contractAddress, strings.NewReader(string(bodyBytes)))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, tc.wantStatus, w.Code)
			if tc.wantErrContains != "" {
				assert.Contains(t, w.Body.String(), tc.wantErrContains)
			} else {
				// parse tx XDR from response
				var response CosignRecoveryResponse
				err = json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)
				assert.NotEmpty(t, response.SignedTransactionXDR, "signed transaction XDR should not be empty")

				// sign tx XDR
				tx, err := txnbuild.TransactionFromXDR(tc.txXDR)
				require.NoError(t, err)
				signedTx, ok := tx.Transaction()
				require.True(t, ok)
				signedTx, err = signedTx.Sign(networkPassphrase, recoveryKP)
				require.NoError(t, err)
				signedTxXDR, err := signedTx.Base64()
				require.NoError(t, err)
				assert.Equal(t, response.SignedTransactionXDR, signedTxXDR)
			}
		})
	}
}
