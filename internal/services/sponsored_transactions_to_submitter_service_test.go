package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/go/strkey"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

func TestSponsoredTransactionsToSubmitterService_SendBatchSponsoredTransactions(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	sdpModels, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	service, err := NewSponsoredTransactionsToSubmitterService(SponsoredTransactionsToSubmitterServiceOptions{
		Models:              sdpModels,
		TSSDBConnectionPool: dbConnectionPool,
	})
	require.NoError(t, err)

	defer data.DeleteAllSponsoredTransactionsFixtures(t, ctx, dbConnectionPool)
	tssModel := store.NewTransactionModel(dbConnectionPool)

	tenant := &schema.Tenant{ID: "tenant-id"}
	runCtx := sdpcontext.SetTenantInContext(ctx, tenant)

	contractBytes := make([]byte, 32)
	contractBytes[31] = 1
	contract, err := strkey.Encode(strkey.VersionByteContract, contractBytes)
	require.NoError(t, err)

	validOpXDR := "AAAAAAAAAAHXkotywnA8z+r365/0701QSlWouXn8m0UOoshCtNHOYQAAAAh0cmFuc2ZlcgAAAAAAAAAA"

	st1 := data.CreateSponsoredTransactionFixture(t, ctx, dbConnectionPool, contract, validOpXDR)
	st2 := data.CreateSponsoredTransactionFixture(t, ctx, dbConnectionPool, contract, validOpXDR)

	err = service.SendBatchSponsoredTransactions(runCtx, 10)
	require.NoError(t, err)

	updated1, err := sdpModels.SponsoredTransactions.GetByID(ctx, dbConnectionPool, st1.ID)
	require.NoError(t, err)
	assert.Equal(t, string(data.ProcessingSponsoredTransactionStatus), updated1.Status)

	updated2, err := sdpModels.SponsoredTransactions.GetByID(ctx, dbConnectionPool, st2.ID)
	require.NoError(t, err)
	assert.Equal(t, string(data.ProcessingSponsoredTransactionStatus), updated2.Status)

	transactions, err := tssModel.GetAllByExternalIDs(ctx, []string{st1.ID, st2.ID})
	require.NoError(t, err)
	assert.Len(t, transactions, 2)
}

func TestSponsoredTransactionsToSubmitterService_SendBatchSponsoredTransactions_NoTenant(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	sdpModels, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	service, err := NewSponsoredTransactionsToSubmitterService(SponsoredTransactionsToSubmitterServiceOptions{
		Models:              sdpModels,
		TSSDBConnectionPool: dbConnectionPool,
	})
	require.NoError(t, err)

	err = service.SendBatchSponsoredTransactions(context.Background(), 5)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tenant")
}
