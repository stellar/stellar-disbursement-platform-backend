package paymentdispatchers

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/testutils"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing/mocks"
	sigMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing/mocks"
	txSubStore "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

func Test_StellarPaymentDispatcher_DispatchPayments(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	tssModel := txSubStore.NewTransactionModel(models.DBConnectionPool)

	tenantID := "tenant-id"

	// Disbursement
	disbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{})

	// Receivers
	receiver1 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})

	// Receiver Wallets
	rw1Registered := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver1.ID, disbursement.Wallet.ID, data.RegisteredReceiversWalletStatus)

	// Payments
	payment1 := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		ReceiverWallet: rw1Registered,
		Disbursement:   disbursement,
		Asset:          *disbursement.Asset,
		Amount:         "100",
		Status:         data.ReadyPaymentStatus,
	})

	tests := []struct {
		name               string
		paymentsToDispatch []*data.Payment
		wantErr            error
		fnSetup            func(*testing.T, *mocks.MockDistributionAccountResolver)
		fnAsserts          func(*testing.T, db.SQLExecuter)
	}{
		{
			name:               "failure fetching distribution account",
			paymentsToDispatch: []*data.Payment{payment1},
			wantErr:            fmt.Errorf("getting distribution account: distribution account not found"),
			fnSetup: func(t *testing.T, mDistAccountResolver *mocks.MockDistributionAccountResolver) {
				mDistAccountResolver.On("DistributionAccountFromContext", ctx).
					Return(schema.TransactionAccount{}, fmt.Errorf("distribution account not found")).
					Once()
			},
		},
		{
			name:               "distribution account is not a Stellar account",
			paymentsToDispatch: []*data.Payment{payment1},
			wantErr:            fmt.Errorf("distribution account is not a Stellar account for tenant tenant-id"),
			fnSetup: func(t *testing.T, mDistAccountResolver *mocks.MockDistributionAccountResolver) {
				mDistAccountResolver.On("DistributionAccountFromContext", ctx).
					Return(schema.TransactionAccount{Type: schema.DistributionAccountCircleDBVault}, nil).
					Once()
			},
		},
		{
			name: "unable to parse payment amount",
			paymentsToDispatch: []*data.Payment{
				{ID: "123", Amount: "invalid-amount"},
			},
			wantErr: fmt.Errorf("parsing payment amount invalid-amount for payment ID 123: strconv.ParseFloat: parsing \"invalid-amount\": invalid syntax"),
			fnSetup: func(t *testing.T, mDistAccountResolver *mocks.MockDistributionAccountResolver) {
				mDistAccountResolver.On("DistributionAccountFromContext", ctx).
					Return(schema.TransactionAccount{Type: schema.DistributionAccountStellarEnv}, nil).
					Once()
			},
		},
		{
			name:               "success posting transfer to Stellar",
			paymentsToDispatch: []*data.Payment{payment1},
			wantErr:            nil,
			fnSetup: func(t *testing.T, mDistAccountResolver *mocks.MockDistributionAccountResolver) {
				mDistAccountResolver.On("DistributionAccountFromContext", ctx).
					Return(schema.TransactionAccount{Type: schema.DistributionAccountStellarEnv}, nil).
					Once()
			},
			fnAsserts: func(t *testing.T, sqlExecuter db.SQLExecuter) {
				// Payment should be marked as pending
				payment, assertErr := models.Payment.Get(ctx, payment1.ID, sqlExecuter)
				require.NoError(t, assertErr)
				assert.Equal(t, data.PendingPaymentStatus, payment.Status)

				// Transaction should be created
				transactions, assertErr := tssModel.GetAllByPaymentIDs(ctx, []string{payment1.ID})
				require.NoError(t, assertErr)
				assert.Len(t, transactions, 1)

				tx := transactions[0]
				assert.Equal(t, txSubStore.TransactionStatusPending, tx.Status)
				assert.Equal(t, payment1.Asset.Code, tx.AssetCode)
				assert.Equal(t, payment1.Asset.Issuer, tx.AssetIssuer)
				assert.Equal(t, payment1.Amount, strconv.FormatFloat(tx.Amount, 'f', 7, 32))
				assert.Equal(t, payment1.ReceiverWallet.StellarAddress, tx.Destination)
				assert.Equal(t, payment1.ID, tx.ExternalID)
				assert.Equal(t, "tenant-id", tx.TenantID)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mDistAccountResolver := sigMocks.NewMockDistributionAccountResolver(t)
			dispatcher := NewStellarPaymentDispatcher(models, tssModel, mDistAccountResolver)
			tssTx := testutils.BeginTxWithRollback(t, ctx, tssModel.DBConnectionPool)

			if tt.fnSetup != nil {
				tt.fnSetup(t, mDistAccountResolver)
			}
			runErr := dispatcher.DispatchPayments(ctx, tssTx, tenantID, tt.paymentsToDispatch)
			if tt.wantErr != nil {
				assert.ErrorContains(t, runErr, tt.wantErr.Error())
			} else {
				assert.NoError(t, runErr)
			}

			if tt.fnAsserts != nil {
				tt.fnAsserts(t, tssTx)
			}
		})
	}
}

func Test_StellarPaymentDispatcher_SupportedPlatform(t *testing.T) {
	dispatcher := StellarPaymentDispatcher{}
	assert.Equal(t, schema.StellarPlatform, dispatcher.SupportedPlatform())
}
