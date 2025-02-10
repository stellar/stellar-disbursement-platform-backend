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
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

func Test_StellarPaymentDispatcher_DispatchPayments(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	tenantID := "tenant-id"
	tnt := tenant.Tenant{
		ID:      tenantID,
		BaseURL: utils.Ptr("https://example.com"),
	}

	ctx := context.Background()
	ctx = tenant.SaveTenantInContext(ctx, &tnt)
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	tssModel := txSubStore.NewTransactionModel(models.DBConnectionPool)

	// Wallets
	walletA := data.CreateWalletFixture(t, ctx, dbConnectionPool, "walletA", "https://www.a.com", "www.a.com", "a://")
	walletB := data.CreateWalletFixture(t, ctx, dbConnectionPool, "walletB", "https://www.b.com", "www.b.com", "b://")
	// Disbursement
	disbursementA := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{Wallet: walletA})
	disbursementB := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{Wallet: walletB})
	// Receiver
	receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	// Receiver Wallets
	rwWithMemo := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, disbursementA.Wallet.ID, data.RegisteredReceiversWalletStatus)
	paymentWithMemo := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		ReceiverWallet: rwWithMemo,
		Disbursement:   disbursementA,
		Asset:          *disbursementA.Asset,
		Amount:         "100",
		Status:         data.ReadyPaymentStatus,
	})

	rwWithoutMemo := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, disbursementB.Wallet.ID, data.RegisteredReceiversWalletStatus)
	err = models.ReceiverWallet.Update(ctx, rwWithoutMemo.ID, data.ReceiverWalletUpdate{
		StellarMemo:     utils.Ptr(""),
		StellarMemoType: utils.Ptr(schema.MemoType("")),
	}, dbConnectionPool)
	require.NoError(t, err)
	paymentWithoutMemo := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		ReceiverWallet: rwWithoutMemo,
		Disbursement:   disbursementB,
		Asset:          *disbursementB.Asset,
		Amount:         "100",
		Status:         data.ReadyPaymentStatus,
	})

	t.Run("🔴", func(t *testing.T) {
		failureTestCases := []struct {
			name               string
			paymentsToDispatch []*data.Payment
			wantErr            error
			fnSetup            func(*testing.T, *mocks.MockDistributionAccountResolver)
		}{
			{
				name:               "failed fetching distribution account",
				paymentsToDispatch: []*data.Payment{paymentWithMemo},
				wantErr:            fmt.Errorf("getting distribution account: distribution account not found"),
				fnSetup: func(t *testing.T, mDistAccountResolver *mocks.MockDistributionAccountResolver) {
					mDistAccountResolver.On("DistributionAccountFromContext", ctx).
						Return(schema.TransactionAccount{}, fmt.Errorf("distribution account not found")).
						Once()
				},
			},
			{
				name:               "distribution account is not a Stellar account",
				paymentsToDispatch: []*data.Payment{paymentWithMemo},
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
		}

		for _, tc := range failureTestCases {
			t.Run(tc.name, func(t *testing.T) {
				defer data.DeleteAllTransactionsFixtures(t, ctx, dbConnectionPool)
				mDistAccountResolver := sigMocks.NewMockDistributionAccountResolver(t)
				dispatcher := NewStellarPaymentDispatcher(models, tssModel, mDistAccountResolver)
				tssTx := testutils.BeginTxWithRollback(t, ctx, tssModel.DBConnectionPool)

				if tc.fnSetup != nil {
					tc.fnSetup(t, mDistAccountResolver)
				}

				runErr := dispatcher.DispatchPayments(ctx, tssTx, tenantID, tc.paymentsToDispatch)
				assert.ErrorContains(t, runErr, tc.wantErr.Error())
			})
		}
	})

	t.Run("🟢", func(t *testing.T) {
		successfulTestCases := []struct {
			name              string
			paymentToDispatch *data.Payment
			fnSetup           func(*testing.T, *mocks.MockDistributionAccountResolver)
			fnAssertMemo      func(t *testing.T, p *data.Payment, tx *txSubStore.Transaction)
		}{
			{
				name:              "success posting transfer to Stellar with ReceiverWallet memo",
				paymentToDispatch: paymentWithMemo,
				fnSetup: func(t *testing.T, mDistAccountResolver *mocks.MockDistributionAccountResolver) {
					err = models.Organizations.Update(ctx, &data.OrganizationUpdate{IsTenantMemoEnabled: utils.Ptr(false)})
					require.NoError(t, err)
					mDistAccountResolver.On("DistributionAccountFromContext", ctx).
						Return(schema.TransactionAccount{Type: schema.DistributionAccountStellarEnv}, nil).
						Once()
				},
				fnAssertMemo: func(t *testing.T, p *data.Payment, tx *txSubStore.Transaction) {
					assert.NotEmpty(t, tx.Memo)
					assert.Equal(t, p.ReceiverWallet.StellarMemo, tx.Memo)
					assert.NotEmpty(t, tx.MemoType)
					assert.Equal(t, p.ReceiverWallet.StellarMemoType, tx.MemoType)
				},
			},
			{
				name:              "success posting transfer to Stellar without ReceiverWallet nor Organization memo",
				paymentToDispatch: paymentWithoutMemo,
				fnSetup: func(t *testing.T, mDistAccountResolver *mocks.MockDistributionAccountResolver) {
					err = models.Organizations.Update(ctx, &data.OrganizationUpdate{IsTenantMemoEnabled: utils.Ptr(false)})
					require.NoError(t, err)

					mDistAccountResolver.On("DistributionAccountFromContext", ctx).
						Return(schema.TransactionAccount{Type: schema.DistributionAccountStellarEnv}, nil).
						Once()
				},
				fnAssertMemo: func(t *testing.T, p *data.Payment, tx *txSubStore.Transaction) {
					assert.Empty(t, tx.Memo)
					assert.Equal(t, p.ReceiverWallet.StellarMemo, tx.Memo)
					assert.Empty(t, tx.MemoType)
					assert.Equal(t, p.ReceiverWallet.StellarMemoType, tx.MemoType)
				},
			},
			{
				name:              "success posting transfer to Stellar with Organization memo enabled",
				paymentToDispatch: paymentWithoutMemo,
				fnSetup: func(t *testing.T, mDistAccountResolver *mocks.MockDistributionAccountResolver) {
					err = models.Organizations.Update(ctx, &data.OrganizationUpdate{IsTenantMemoEnabled: utils.Ptr(true)})
					require.NoError(t, err)

					mDistAccountResolver.On("DistributionAccountFromContext", ctx).
						Return(schema.TransactionAccount{Type: schema.DistributionAccountStellarEnv}, nil).
						Once()
				},
				fnAssertMemo: func(t *testing.T, p *data.Payment, tx *txSubStore.Transaction) {
					assert.Equal(t, GenerateHashFromBaseURL(*tnt.BaseURL), tx.Memo)
					assert.Equal(t, schema.MemoTypeText, tx.MemoType)
					assert.Equal(t, "tenant-id", tx.TenantID)
				},
			},
		}

		for _, tc := range successfulTestCases {
			t.Run(tc.name, func(t *testing.T) {
				defer data.DeleteAllTransactionsFixtures(t, ctx, dbConnectionPool)
				mDistAccountResolver := sigMocks.NewMockDistributionAccountResolver(t)
				dispatcher := NewStellarPaymentDispatcher(models, tssModel, mDistAccountResolver)
				tssTx := testutils.BeginTxWithRollback(t, ctx, tssModel.DBConnectionPool)

				if tc.fnSetup != nil {
					tc.fnSetup(t, mDistAccountResolver)
				}

				runErr := dispatcher.DispatchPayments(ctx, tssTx, tenantID, []*data.Payment{tc.paymentToDispatch})
				assert.NoError(t, runErr)

				// Assertions:
				// Payment should be marked as pending
				p, assertErr := models.Payment.Get(ctx, tc.paymentToDispatch.ID, tssTx)
				require.NoError(t, assertErr)
				assert.Equal(t, data.PendingPaymentStatus, p.Status)

				// Transaction should be created
				transactions, assertErr := tssModel.GetAllByPaymentIDs(ctx, []string{p.ID})
				require.NoError(t, assertErr)
				require.Len(t, transactions, 1)

				tx := transactions[0]
				assert.Equal(t, txSubStore.TransactionStatusPending, tx.Status)
				assert.Equal(t, p.Asset.Code, tx.AssetCode)
				assert.Equal(t, p.Asset.Issuer, tx.AssetIssuer)
				assert.Equal(t, p.Amount, strconv.FormatFloat(tx.Amount, 'f', 7, 32))
				assert.Equal(t, p.ReceiverWallet.StellarAddress, tx.Destination)
				assert.Equal(t, p.ID, tx.ExternalID)
				assert.Equal(t, "tenant-id", tx.TenantID)

				tc.fnAssertMemo(t, p, tx)
			})
		}
	})
}

func Test_StellarPaymentDispatcher_SupportedPlatform(t *testing.T) {
	dispatcher := StellarPaymentDispatcher{}
	assert.Equal(t, schema.StellarPlatform, dispatcher.SupportedPlatform())
}
