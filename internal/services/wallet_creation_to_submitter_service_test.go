package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

func TestWalletCreationToSubmitterService_SendBatchWalletCreations(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	sdpModels, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	service, err := NewWalletCreationToSubmitterService(WalletCreationToSubmitterServiceOptions{
		Models:              sdpModels,
		TSSDBConnectionPool: dbConnectionPool,
	})
	require.NoError(t, err)

	defer data.DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)
	tssModel := store.NewTransactionModel(dbConnectionPool)

	tenant := &schema.Tenant{ID: "tenant-id"}
	runCtx := sdpcontext.SetTenantInContext(ctx, tenant)

	wasmHash := "e5da3b9950524b4276ccf2051e6cc8220bb581e869b892a6ff7812d7709c7a50"
	pubKey := "04f5549c5ef833ab0ade80d9c1f3fb34fb93092503a8ce105773d676288653df384a024a92cc73cb8089c45ed76ed073433b6a72c64a6ed23630b77327beb65f23"

	wallet1 := data.CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "wallet-1", wasmHash, "", "cred-1", pubKey, data.PendingWalletStatus)
	wallet2 := data.CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "wallet-2", wasmHash, "", "cred-2", pubKey, data.PendingWalletStatus)

	err = service.SendBatchWalletCreations(runCtx, 10)
	require.NoError(t, err)

	updated1, err := sdpModels.EmbeddedWallets.GetByToken(ctx, dbConnectionPool, wallet1.Token)
	require.NoError(t, err)
	assert.Equal(t, data.ProcessingWalletStatus, updated1.WalletStatus)

	updated2, err := sdpModels.EmbeddedWallets.GetByToken(ctx, dbConnectionPool, wallet2.Token)
	require.NoError(t, err)
	assert.Equal(t, data.ProcessingWalletStatus, updated2.WalletStatus)

	transactions, err := tssModel.GetAllByExternalIDs(ctx, []string{wallet1.Token, wallet2.Token})
	require.NoError(t, err)
	assert.Len(t, transactions, 2)
}

func TestWalletCreationToSubmitterService_SendBatchWalletCreations_NoTenant(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	sdpModels, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	service, err := NewWalletCreationToSubmitterService(WalletCreationToSubmitterServiceOptions{
		Models:              sdpModels,
		TSSDBConnectionPool: dbConnectionPool,
	})
	require.NoError(t, err)

	err = service.SendBatchWalletCreations(context.Background(), 5)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tenant")
}
