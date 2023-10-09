package jobs

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/anchorplatform"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewPatchAnchorPlatformTransactionCompletionJob(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	t.Run("exits with status 1 when AP API Client is missing", func(t *testing.T) {
		if os.Getenv("TEST_FATAL") == "1" {
			NewPatchAnchorPlatformTransactionsCompletionJob(nil, nil)
			return
		}

		// We're using a strategy to setup a cmd inside the test that calls the test itself and verifies if it exited with exit status '1'.
		// Ref: https://go.dev/talks/2014/testing.slide#23
		cmd := exec.Command(os.Args[0], fmt.Sprintf("-test.run=%s", t.Name()))
		cmd.Env = append(os.Environ(), "TEST_FATAL=1")

		err := cmd.Run()
		if exitError, ok := err.(*exec.ExitError); ok {
			assert.False(t, exitError.Success())
			return
		}

		t.Fatalf("process ran with err %v, want exit status 1", err)
	})

	t.Run("exits with status 1 when SDP Models are missing", func(t *testing.T) {
		if os.Getenv("TEST_FATAL") == "1" {
			NewPatchAnchorPlatformTransactionsCompletionJob(&anchorplatform.AnchorPlatformAPIService{}, nil)
			return
		}

		// We're using a strategy to setup a cmd inside the test that calls the test itself and verifies if it exited with exit status '1'.
		// Ref: https://go.dev/talks/2014/testing.slide#23
		cmd := exec.Command(os.Args[0], fmt.Sprintf("-test.run=%s", t.Name()))
		cmd.Env = append(os.Environ(), "TEST_FATAL=1")

		err := cmd.Run()
		if exitError, ok := err.(*exec.ExitError); ok {
			assert.False(t, exitError.Success())
			return
		}

		t.Fatalf("process ran with err %v, want exit status 1", err)
	})

	t.Run("returns a job instance successfully", func(t *testing.T) {
		models, err := data.NewModels(dbConnectionPool)
		require.NoError(t, err)
		svc := NewPatchAnchorPlatformTransactionsCompletionJob(&anchorplatform.AnchorPlatformAPIService{}, models)
		assert.NotNil(t, svc)
	})
}

func Test_PatchAnchorPlatformTransactionsCompletionJob(t *testing.T) {
	j := PatchAnchorPlatformTransactionsCompletionJob{}

	assert.Equal(t, patchAnchorPlatformTransactionsCompletionJobName, j.GetName())
	assert.Equal(t, patchAnchorPlatformTransactionsCompletionJobIntervalSeconds*time.Second, j.GetInterval())
}

func Test_PatchAnchorPlatformTransactionsCompletionJob_Execute(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	apAPISvcMock := anchorplatform.AnchorPlatformAPIServiceMock{}
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	t.Run("executes the job successfully", func(t *testing.T) {
		j := NewPatchAnchorPlatformTransactionsCompletionJob(&apAPISvcMock, models)

		data.DeleteAllFixtures(t, ctx, dbConnectionPool)

		country := data.CreateCountryFixture(t, ctx, dbConnectionPool, "BRA", "Brazil")
		wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "Wallet", "https://www.wallet.com", "www.wallet.com", "wallet://")
		asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")

		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
		receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.RegisteredReceiversWalletStatus)

		disbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
			Country:           country,
			Wallet:            wallet,
			Asset:             asset,
			Status:            data.StartedDisbursementStatus,
			VerificationField: data.VerificationFieldDateOfBirth,
		})

		payment := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-1",
			StellarOperationID:   "operation-id-1",
			Status:               data.SuccessPaymentStatus,
			Disbursement:         disbursement,
			ReceiverWallet:       receiverWallet,
			Asset:                *asset,
		})

		getEntries := log.DefaultLogger.StartTest(log.DebugLevel)

		apAPISvcMock.
			On("PatchAnchorTransactionsPostSuccessCompletion", ctx, anchorplatform.APSep24TransactionPatchPostSuccess{
				ID:     receiverWallet.AnchorPlatformTransactionID,
				SEP:    "24",
				Status: anchorplatform.APTransactionStatusCompleted,
				StellarTransactions: []anchorplatform.APStellarTransaction{
					{
						ID:       payment.StellarTransactionID,
						Memo:     receiverWallet.StellarMemo,
						MemoType: receiverWallet.StellarMemoType,
					},
				},
				CompletedAt: &payment.UpdatedAt,
				AmountOut: anchorplatform.APAmount{
					Amount: payment.Amount,
					Asset:  anchorplatform.NewStellarAssetInAIF(payment.Asset.Code, payment.Asset.Issuer),
				},
			}).
			Return(nil).
			Once()

		err := j.Execute(ctx)
		require.NoError(t, err)

		const q = "SELECT anchor_platform_transaction_synced_at FROM receiver_wallets WHERE id = $1"
		var syncedAt pq.NullTime
		err = dbConnectionPool.GetContext(ctx, &syncedAt, q, receiverWallet.ID)
		require.NoError(t, err)
		assert.False(t, syncedAt.Time.IsZero())

		entries := getEntries()
		require.Len(t, entries, 2)
		assert.Equal(t, "PatchAnchorPlatformTransactionService: got 1 payments to process", entries[0].Message)
		assert.Equal(t, "PatchAnchorPlatformTransactionService: updating anchor platform transaction synced at for 1 receiver wallet(s)", entries[1].Message)
	})
}
