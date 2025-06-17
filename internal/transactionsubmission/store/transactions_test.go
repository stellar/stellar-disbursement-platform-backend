package store

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/txnbuild"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

func Test_Transaction_BuildMemo(t *testing.T) {
	testCases := []struct {
		memoType        schema.MemoType
		memoValue       string
		wantMemo        txnbuild.Memo
		wantErrContains string
	}{
		{
			memoType:  "",
			memoValue: "",
			wantMemo:  nil,
		},
		{
			memoType:        schema.MemoTypeText,
			memoValue:       "This is a very long text that should exceed the 28-byte limit",
			wantErrContains: "text memo must be 28 bytes or less",
		},
		{
			memoType:        schema.MemoTypeText,
			memoValue:       "HelloWorld!",
			wantMemo:        txnbuild.MemoText("HelloWorld!"),
			wantErrContains: "",
		},
		{
			memoType:        schema.MemoTypeID,
			memoValue:       "not-a-valid-uint64",
			wantErrContains: "invalid Memo ID value, must be a uint64",
		},
		{
			memoType:        schema.MemoTypeID,
			memoValue:       "1234567890",
			wantMemo:        txnbuild.MemoID(1234567890),
			wantErrContains: "",
		},
		{
			memoType:        schema.MemoTypeHash,
			memoValue:       "12f37f82eb6708daa0ac372a1a67a0f33efa6a9cd213ed430517e45fefb5157712f37f82eb6708daa0ac372a1a67a0f33efa6a9cd213ed430517e45fefb51577",
			wantErrContains: "hash memo must be 64 hex characters (32 bytes)",
		},
		{
			memoType:        schema.MemoTypeHash,
			memoValue:       "12f37f82eb6708daa0ac372a1a67a0f33efa6a9cd213ed430517e45fefb51577",
			wantMemo:        txnbuild.MemoHash([]byte{0x12, 0xf3, 0x7f, 0x82, 0xeb, 0x67, 0x08, 0xda, 0xa0, 0xac, 0x37, 0x2a, 0x1a, 0x67, 0xa0, 0xf3, 0x3e, 0xfa, 0x6a, 0x9c, 0xd2, 0x13, 0xed, 0x43, 0x05, 0x17, 0xe4, 0x5f, 0xef, 0xb5, 0x15, 0x77}),
			wantErrContains: "",
		},
		{
			memoType:        schema.MemoTypeReturn,
			memoValue:       "12f37f82eb6708daa0ac372a1a67a0f33efa6a9cd213ed430517e45fefb5157712f37f82eb6708daa0ac372a1a67a0f33efa6a9cd213ed430517e45fefb51577",
			wantErrContains: "return memo must be 64 hex characters (32 bytes)",
		},
		{
			memoType:        schema.MemoTypeReturn,
			memoValue:       "12f37f82eb6708daa0ac372a1a67a0f33efa6a9cd213ed430517e45fefb51577",
			wantMemo:        txnbuild.MemoReturn([]byte{0x12, 0xf3, 0x7f, 0x82, 0xeb, 0x67, 0x08, 0xda, 0xa0, 0xac, 0x37, 0x2a, 0x1a, 0x67, 0xa0, 0xf3, 0x3e, 0xfa, 0x6a, 0x9c, 0xd2, 0x13, 0xed, 0x43, 0x05, 0x17, 0xe4, 0x5f, 0xef, 0xb5, 0x15, 0x77}),
			wantErrContains: "",
		},
	}

	for _, tc := range testCases {
		emojiPrefix := "🟢"
		if tc.wantErrContains != "" {
			emojiPrefix = "🔴"
		}
		t.Run(fmt.Sprintf("%s%s(%s)", emojiPrefix, tc.memoType, tc.memoValue), func(t *testing.T) {
			tx := &Transaction{TransactionType: TransactionTypePayment, Payment: Payment{MemoType: tc.memoType, Memo: tc.memoValue}}
			gotMemo, err := tx.BuildMemo()
			if tc.wantErrContains == "" {
				require.NoError(t, err)
				require.Equal(t, tc.wantMemo, gotMemo)
			} else {
				require.ErrorContains(t, err, tc.wantErrContains)
				require.Nil(t, gotMemo)
			}
		})
	}

	t.Run("returns an error if the transaction type is not payment", func(t *testing.T) {
		tx := &Transaction{TransactionType: TransactionTypeWalletCreation, WalletCreation: WalletCreation{}}
		gotMemo, err := tx.BuildMemo()
		require.ErrorContains(t, err, "transaction type \"WALLET_CREATION\" does not support memo")
		require.Nil(t, gotMemo)
	})
}

func Test_Transaction_IsLocked(t *testing.T) {
	const currentLedgerNumber = 10

	testCases := []struct {
		name                    string
		lockedUntilLedgerNumber sql.NullInt32
		wantResult              bool
	}{
		{
			name:                    "returns false if lockedUntilLedgerNumber is null",
			lockedUntilLedgerNumber: sql.NullInt32{},
			wantResult:              false,
		},
		{
			name:                    "returns false if lockedUntilLedgerNumber is lower than currentLedgerNumber",
			lockedUntilLedgerNumber: sql.NullInt32{Int32: currentLedgerNumber - 1, Valid: true},
			wantResult:              false,
		},
		{
			name:                    "returns true if lockedUntilLedgerNumber is equal to currentLedgerNumber",
			lockedUntilLedgerNumber: sql.NullInt32{Int32: currentLedgerNumber, Valid: true},
			wantResult:              true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tx := &Transaction{LockedUntilLedgerNumber: tc.lockedUntilLedgerNumber}
			assert.Equal(t, tc.wantResult, tx.IsLocked(currentLedgerNumber))
		})
	}
}

func Test_TransactionModel_Insert(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	txModel := NewTransactionModel(dbConnectionPool)

	t.Run("return an error if the input parameters are invalid", func(t *testing.T) {
		tx, err := txModel.Insert(ctx, Transaction{ExternalID: "external-id-1", TransactionType: TransactionTypePayment, TenantID: uuid.NewString()})
		require.Error(t, err)
		assert.EqualError(t, err, "inserting single transaction: validating transaction for insertion: validating payment transaction: asset code must have between 1 and 12 characters")
		assert.Nil(t, tx)
	})

	t.Run("🎉 successfully insert a new Payment Transaction", func(t *testing.T) {
		transaction, err := txModel.Insert(ctx, Transaction{
			ExternalID:      "external-id-1",
			TransactionType: TransactionTypePayment,
			Payment: Payment{
				AssetCode:   "USDC",
				AssetIssuer: "GCBIRB7Q5T53H4L6P5QSI3O6LPD5MBWGM5GHE7A5NY4XT5OT4VCOEZFX",
				Amount:      1,
				Destination: "GBHNIYGWZUAVZX7KTLVSMILBXJMUACVO6XBEKIN6RW7AABDFH6S7GK2Y",
			},
			TenantID: "tenant-id-1",
		})
		require.NoError(t, err)
		require.NotNil(t, transaction)

		refreshedTx, err := txModel.Get(ctx, transaction.ID)
		require.NoError(t, err)
		assert.Equal(t, transaction, refreshedTx)

		assert.Equal(t, "external-id-1", refreshedTx.ExternalID)
		assert.Equal(t, TransactionTypePayment, refreshedTx.TransactionType)
		assert.Equal(t, "USDC", refreshedTx.AssetCode)
		assert.Equal(t, "GCBIRB7Q5T53H4L6P5QSI3O6LPD5MBWGM5GHE7A5NY4XT5OT4VCOEZFX", refreshedTx.AssetIssuer)
		assert.Equal(t, float64(1), refreshedTx.Amount)
		assert.Equal(t, "GBHNIYGWZUAVZX7KTLVSMILBXJMUACVO6XBEKIN6RW7AABDFH6S7GK2Y", refreshedTx.Destination)
		assert.Equal(t, TransactionStatusPending, refreshedTx.Status)
		assert.Equal(t, "tenant-id-1", refreshedTx.TenantID)
	})

	t.Run("🎉 successfully insert a new Wallet Creation Transaction", func(t *testing.T) {
		transaction, err := txModel.Insert(ctx, Transaction{
			ExternalID:      "external-id-2",
			TransactionType: TransactionTypeWalletCreation,
			WalletCreation: WalletCreation{
				PublicKey: "04f5549c5ef833ab0ade80d9c1f3fb34fb93092503a8ce105773d676288653df384a024a92cc73cb8089c45ed76ed073433b6a72c64a6ed23630b77327beb65f23",
				WasmHash:  "e5da3b9950524b4276ccf2051e6cc8220bb581e869b892a6ff7812d7709c7a50",
			},
			TenantID: "tenant-id-2",
		})
		require.NoError(t, err)
		require.NotNil(t, transaction)

		refreshedTx, err := txModel.Get(ctx, transaction.ID)
		require.NoError(t, err)
		assert.Equal(t, transaction, refreshedTx)

		assert.Equal(t, "external-id-2", refreshedTx.ExternalID)
		assert.Equal(t, TransactionTypeWalletCreation, refreshedTx.TransactionType)
		assert.Equal(t, "04f5549c5ef833ab0ade80d9c1f3fb34fb93092503a8ce105773d676288653df384a024a92cc73cb8089c45ed76ed073433b6a72c64a6ed23630b77327beb65f23", refreshedTx.PublicKey)
		assert.Equal(t, "e5da3b9950524b4276ccf2051e6cc8220bb581e869b892a6ff7812d7709c7a50", refreshedTx.WasmHash)
		assert.Equal(t, TransactionStatusPending, refreshedTx.Status)
		assert.Equal(t, "tenant-id-2", refreshedTx.TenantID)
	})

	t.Run("successfully insert a new Sponsored Transaction", func(t *testing.T) {
		transaction, err := txModel.Insert(ctx, Transaction{
			ExternalID:      "external-id-3",
			TransactionType: TransactionTypeSponsored,
			Sponsored: Sponsored{
				SponsoredAccount:        "CDTY3P6OVY3SMZXR3DZA667NAXFECA6A3AOZXEU33DD2ACBY43CIKDPT",
				SponsoredTransactionXDR: "AAAAAgAAAADSsOMKYK7a1aALie83F4GQDoBdHrW86UX2SYVygRA+VQACywMAAAfhAAAGugAAAAEAAAAAAAAAAAAAAABoSGeWAAAAAAAAAAEAAAABAAAAANKw4wpgrtrVoAuJ7zcXgZAOgF0etbzpRfZJhXKBED5VAAAAGAAAAAAAAAAB542/zq43Jmbx2PIPe+0FykEDwNgdm5Kb2MegCDjmxIUAAAAIdHJhbnNmZXIAAAADAAAAEgAAAAExfHoGm9ws+/5izT6kpJVi5PnJL+ttrXdkxajBGZv6nAAAABIAAAAAAAAAAFvGtEMyXcvbioU2IKCSomxahpl7lUyef7ftEPxWcD4bAAAACgAAAAAAAAAAAAAAAACYloAAAAABAAAAAQAAAAExfHoGm9ws+/5izT6kpJVi5PnJL+ttrXdkxajBGZv6nH0ObdiOTpYlABXcfAAAABAAAAABAAAAAQAAABEAAAABAAAAAgAAAA8AAAAKcHVibGljX2tleQAAAAAADQAAACDSsOMKYK7a1aALie83F4GQDoBdHrW86UX2SYVygRA+VQAAAA8AAAAJc2lnbmF0dXJlAAAAAAAADQAAAEAQ7ioNcH2wRZxLNA2ssu0tNx2K9DBRsh6u0tVrwkkj0sqwsxvYdrm072z5UE9sUSmcbd5s9CHK+MxSRsrc+gUHAAAAAAAAAAHnjb/OrjcmZvHY8g977QXKQQPA2B2bkpvYx6AIOObEhQAAAAh0cmFuc2ZlcgAAAAMAAAASAAAAATF8egab3Cz7/mLNPqSklWLk+ckv622td2TFqMEZm/qcAAAAEgAAAAAAAAAAW8a0QzJdy9uKhTYgoJKibFqGmXuVTJ5/t+0Q/FZwPhsAAAAKAAAAAAAAAAAAAAAAAJiWgAAAAAAAAAABAAAAAAAAAAMAAAAGAAAAATF8egab3Cz7/mLNPqSklWLk+ckv622td2TFqMEZm/qcAAAAFAAAAAEAAAAGAAAAAeeNv86uNyZm8djyD3vtBcpBA8DYHZuSm9jHoAg45sSFAAAAFAAAAAEAAAAHURIgc/sBvxJigzDM+JHIxvPBqRHTG9KFFsMC7294utoAAAADAAAAAQAAAABbxrRDMl3L24qFNiCgkqJsWoaZe5VMnn+37RD8VnA+GwAAAAFVU0RDAAAAAODia2IsqMlWCuY6k734V/dcCafJwfI1Qq7+/0qEd68AAAAABgAAAAExfHoGm9ws+/5izT6kpJVi5PnJL+ttrXdkxajBGZv6nAAAABV9Dm3Yjk6WJQAAAAAAAAAGAAAAAeeNv86uNyZm8djyD3vtBcpBA8DYHZuSm9jHoAg45sSFAAAAEAAAAAEAAAACAAAADwAAAAdCYWxhbmNlAAAAABIAAAABMXx6BpvcLPv+Ys0+pKSVYuT5yS/rba13ZMWowRmb+pwAAAABABlJmAAADWAAAAGcAAAAAAACyp8AAAABgRA+VQAAAEAVqZBB44AhyhyYi3QN51aEvkGw62m+2D2lSGt0bO4lcUNIL10dN1acoBituE9F1Ypxb+mAyqZFhYLg8vn5n5sP",
			},
			TenantID: "tenant-id-3",
		})
		require.NoError(t, err)
		require.NotNil(t, transaction)

		refreshedTx, err := txModel.Get(ctx, transaction.ID)
		require.NoError(t, err)
		assert.Equal(t, transaction, refreshedTx)

		assert.Equal(t, "external-id-3", refreshedTx.ExternalID)
		assert.Equal(t, TransactionTypeSponsored, refreshedTx.TransactionType)
		assert.Equal(t, "CDTY3P6OVY3SMZXR3DZA667NAXFECA6A3AOZXEU33DD2ACBY43CIKDPT", refreshedTx.Sponsored.SponsoredAccount)
		assert.Equal(t, "AAAAAgAAAADSsOMKYK7a1aALie83F4GQDoBdHrW86UX2SYVygRA+VQACywMAAAfhAAAGugAAAAEAAAAAAAAAAAAAAABoSGeWAAAAAAAAAAEAAAABAAAAANKw4wpgrtrVoAuJ7zcXgZAOgF0etbzpRfZJhXKBED5VAAAAGAAAAAAAAAAB542/zq43Jmbx2PIPe+0FykEDwNgdm5Kb2MegCDjmxIUAAAAIdHJhbnNmZXIAAAADAAAAEgAAAAExfHoGm9ws+/5izT6kpJVi5PnJL+ttrXdkxajBGZv6nAAAABIAAAAAAAAAAFvGtEMyXcvbioU2IKCSomxahpl7lUyef7ftEPxWcD4bAAAACgAAAAAAAAAAAAAAAACYloAAAAABAAAAAQAAAAExfHoGm9ws+/5izT6kpJVi5PnJL+ttrXdkxajBGZv6nH0ObdiOTpYlABXcfAAAABAAAAABAAAAAQAAABEAAAABAAAAAgAAAA8AAAAKcHVibGljX2tleQAAAAAADQAAACDSsOMKYK7a1aALie83F4GQDoBdHrW86UX2SYVygRA+VQAAAA8AAAAJc2lnbmF0dXJlAAAAAAAADQAAAEAQ7ioNcH2wRZxLNA2ssu0tNx2K9DBRsh6u0tVrwkkj0sqwsxvYdrm072z5UE9sUSmcbd5s9CHK+MxSRsrc+gUHAAAAAAAAAAHnjb/OrjcmZvHY8g977QXKQQPA2B2bkpvYx6AIOObEhQAAAAh0cmFuc2ZlcgAAAAMAAAASAAAAATF8egab3Cz7/mLNPqSklWLk+ckv622td2TFqMEZm/qcAAAAEgAAAAAAAAAAW8a0QzJdy9uKhTYgoJKibFqGmXuVTJ5/t+0Q/FZwPhsAAAAKAAAAAAAAAAAAAAAAAJiWgAAAAAAAAAABAAAAAAAAAAMAAAAGAAAAATF8egab3Cz7/mLNPqSklWLk+ckv622td2TFqMEZm/qcAAAAFAAAAAEAAAAGAAAAAeeNv86uNyZm8djyD3vtBcpBA8DYHZuSm9jHoAg45sSFAAAAFAAAAAEAAAAHURIgc/sBvxJigzDM+JHIxvPBqRHTG9KFFsMC7294utoAAAADAAAAAQAAAABbxrRDMl3L24qFNiCgkqJsWoaZe5VMnn+37RD8VnA+GwAAAAFVU0RDAAAAAODia2IsqMlWCuY6k734V/dcCafJwfI1Qq7+/0qEd68AAAAABgAAAAExfHoGm9ws+/5izT6kpJVi5PnJL+ttrXdkxajBGZv6nAAAABV9Dm3Yjk6WJQAAAAAAAAAGAAAAAeeNv86uNyZm8djyD3vtBcpBA8DYHZuSm9jHoAg45sSFAAAAEAAAAAEAAAACAAAADwAAAAdCYWxhbmNlAAAAABIAAAABMXx6BpvcLPv+Ys0+pKSVYuT5yS/rba13ZMWowRmb+pwAAAABABlJmAAADWAAAAGcAAAAAAACyp8AAAABgRA+VQAAAEAVqZBB44AhyhyYi3QN51aEvkGw62m+2D2lSGt0bO4lcUNIL10dN1acoBituE9F1Ypxb+mAyqZFhYLg8vn5n5sP", refreshedTx.Sponsored.SponsoredTransactionXDR)
		assert.Equal(t, TransactionStatusPending, refreshedTx.Status)
		assert.Equal(t, "tenant-id-3", refreshedTx.TenantID)
	})
}

func Test_TransactionModel_BulkInsert(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	txModel := NewTransactionModel(dbConnectionPool)
	defer DeleteAllTransactionFixtures(t, ctx, dbConnectionPool)

	t.Run("return nil with no error if the input slice is nil", func(t *testing.T) {
		insertedTransactions, err := txModel.BulkInsert(ctx, dbConnectionPool, nil)
		require.NoError(t, err)
		assert.Nil(t, insertedTransactions)
	})

	t.Run("return nil with no error if the input slice is empty", func(t *testing.T) {
		insertedTransactions, err := txModel.BulkInsert(ctx, dbConnectionPool, []Transaction{})
		require.NoError(t, err)
		assert.Nil(t, insertedTransactions)
	})

	t.Run("return an error if the input parameters are invalid", func(t *testing.T) {
		transactionsToInsert := []Transaction{{ExternalID: "external-id-1"}}
		insertedTransactions, err := txModel.BulkInsert(ctx, dbConnectionPool, transactionsToInsert)
		require.Error(t, err)
		assert.EqualError(t, err, "validating transaction for insertion: tenant ID is required")
		assert.Nil(t, insertedTransactions)
	})

	t.Run("🎉 successfully inserts the transactions successfully", func(t *testing.T) {
		incomingTx1 := Transaction{
			ExternalID:      "external-id-1",
			TransactionType: TransactionTypePayment,
			Payment: Payment{
				AssetCode:   "USDC",
				AssetIssuer: keypair.MustRandom().Address(),
				// Lowest number in the Stellar network (ref: https://developers.stellar.org/docs/fundamentals-and-concepts/stellar-data-structures/assets#amount-precision):
				Amount:      0.0000001,
				Destination: keypair.MustRandom().Address(),
			},
			TenantID: uuid.NewString(),
		}
		incomingTx2 := Transaction{
			ExternalID:      "external-id-2",
			TransactionType: TransactionTypePayment,
			Payment: Payment{
				AssetCode:   "USDC",
				AssetIssuer: keypair.MustRandom().Address(),
				// Largest number in the Stellar network (ref: https://developers.stellar.org/docs/fundamentals-and-concepts/stellar-data-structures/assets#amount-precision):
				Amount:      922337203685.4775807,
				Destination: keypair.MustRandom().Address(),
			},
			TenantID: uuid.NewString(),
		}
		incomingTx3 := Transaction{
			ExternalID:      "external-id-3",
			TransactionType: TransactionTypeWalletCreation,
			WalletCreation: WalletCreation{
				PublicKey: "04f5549c5ef833ab0ade80d9c1f3fb34fb93092503a8ce105773d676288653df384a024a92cc73cb8089c45ed76ed073433b6a72c64a6ed23630b77327beb65f23",
				WasmHash:  "e5da3b9950524b4276ccf2051e6cc8220bb581e869b892a6ff7812d7709c7a50",
			},
			TenantID: uuid.NewString(),
		}
		incomingTx4 := Transaction{
			ExternalID:      "external-id-4",
			TransactionType: TransactionTypeSponsored,
			Sponsored: Sponsored{
				SponsoredAccount:        "CDTY3P6OVY3SMZXR3DZA667NAXFECA6A3AOZXEU33DD2ACBY43CIKDPT",
				SponsoredTransactionXDR: "AAAAAgAAAADSsOMKYK7a1aALie83F4GQDoBdHrW86UX2SYVygRA+VQACywMAAAfhAAAGugAAAAEAAAAAAAAAAAAAAABoSGeWAAAAAAAAAAEAAAABAAAAANKw4wpgrtrVoAuJ7zcXgZAOgF0etbzpRfZJhXKBED5VAAAAGAAAAAAAAAAB542/zq43Jmbx2PIPe+0FykEDwNgdm5Kb2MegCDjmxIUAAAAIdHJhbnNmZXIAAAADAAAAEgAAAAExfHoGm9ws+/5izT6kpJVi5PnJL+ttrXdkxajBGZv6nAAAABIAAAAAAAAAAFvGtEMyXcvbioU2IKCSomxahpl7lUyef7ftEPxWcD4bAAAACgAAAAAAAAAAAAAAAACYloAAAAABAAAAAQAAAAExfHoGm9ws+/5izT6kpJVi5PnJL+ttrXdkxajBGZv6nH0ObdiOTpYlABXcfAAAABAAAAABAAAAAQAAABEAAAABAAAAAgAAAA8AAAAKcHVibGljX2tleQAAAAAADQAAACDSsOMKYK7a1aALie83F4GQDoBdHrW86UX2SYVygRA+VQAAAA8AAAAJc2lnbmF0dXJlAAAAAAAADQAAAEAQ7ioNcH2wRZxLNA2ssu0tNx2K9DBRsh6u0tVrwkkj0sqwsxvYdrm072z5UE9sUSmcbd5s9CHK+MxSRsrc+gUHAAAAAAAAAAHnjb/OrjcmZvHY8g977QXKQQPA2B2bkpvYx6AIOObEhQAAAAh0cmFuc2ZlcgAAAAMAAAASAAAAATF8egab3Cz7/mLNPqSklWLk+ckv622td2TFqMEZm/qcAAAAEgAAAAAAAAAAW8a0QzJdy9uKhTYgoJKibFqGmXuVTJ5/t+0Q/FZwPhsAAAAKAAAAAAAAAAAAAAAAAJiWgAAAAAAAAAABAAAAAAAAAAMAAAAGAAAAATF8egab3Cz7/mLNPqSklWLk+ckv622td2TFqMEZm/qcAAAAFAAAAAEAAAAGAAAAAeeNv86uNyZm8djyD3vtBcpBA8DYHZuSm9jHoAg45sSFAAAAFAAAAAEAAAAHURIgc/sBvxJigzDM+JHIxvPBqRHTG9KFFsMC7294utoAAAADAAAAAQAAAABbxrRDMl3L24qFNiCgkqJsWoaZe5VMnn+37RD8VnA+GwAAAAFVU0RDAAAAAODia2IsqMlWCuY6k734V/dcCafJwfI1Qq7+/0qEd68AAAAABgAAAAExfHoGm9ws+/5izT6kpJVi5PnJL+ttrXdkxajBGZv6nAAAABV9Dm3Yjk6WJQAAAAAAAAAGAAAAAeeNv86uNyZm8djyD3vtBcpBA8DYHZuSm9jHoAg45sSFAAAAEAAAAAEAAAACAAAADwAAAAdCYWxhbmNlAAAAABIAAAABMXx6BpvcLPv+Ys0+pKSVYuT5yS/rba13ZMWowRmb+pwAAAABABlJmAAADWAAAAGcAAAAAAACyp8AAAABgRA+VQAAAEAVqZBB44AhyhyYi3QN51aEvkGw62m+2D2lSGt0bO4lcUNIL10dN1acoBituE9F1Ypxb+mAyqZFhYLg8vn5n5sP",
			},
			TenantID: uuid.NewString(),
		}
		insertedTransactions, err := txModel.BulkInsert(ctx, dbConnectionPool, []Transaction{incomingTx1, incomingTx2, incomingTx3, incomingTx4})
		require.NoError(t, err)
		assert.NotNil(t, insertedTransactions)
		assert.Len(t, insertedTransactions, 4)

		var insertedTx1, insertedTx2, insertedTx3, insertedTx4 Transaction
		for _, tx := range insertedTransactions {
			if tx.ExternalID == incomingTx1.ExternalID {
				insertedTx1 = tx
			} else if tx.ExternalID == incomingTx2.ExternalID {
				insertedTx2 = tx
			} else if tx.ExternalID == incomingTx3.ExternalID {
				insertedTx3 = tx
			} else if tx.ExternalID == incomingTx4.ExternalID {
				insertedTx4 = tx
			} else {
				require.FailNow(t, "unexpected transaction: %v", tx)
			}
		}

		assert.Equal(t, incomingTx1.ExternalID, insertedTx1.ExternalID)
		assert.Equal(t, incomingTx1.TransactionType, insertedTx1.TransactionType)
		assert.Equal(t, incomingTx1.AssetCode, insertedTx1.AssetCode)
		assert.Equal(t, incomingTx1.AssetIssuer, insertedTx1.AssetIssuer)
		assert.Equal(t, incomingTx1.Amount, insertedTx1.Amount)
		assert.Equal(t, incomingTx1.Destination, insertedTx1.Destination)
		assert.Equal(t, TransactionStatusPending, insertedTx1.Status)

		assert.Equal(t, incomingTx2.ExternalID, insertedTx2.ExternalID)
		assert.Equal(t, incomingTx2.TransactionType, insertedTx2.TransactionType)
		assert.Equal(t, incomingTx2.AssetCode, insertedTx2.AssetCode)
		assert.Equal(t, incomingTx2.AssetIssuer, insertedTx2.AssetIssuer)
		assert.Equal(t, incomingTx2.Amount, insertedTx2.Amount)
		assert.Equal(t, incomingTx2.Destination, insertedTx2.Destination)
		assert.Equal(t, TransactionStatusPending, insertedTx2.Status)

		assert.Equal(t, incomingTx3.ExternalID, insertedTx3.ExternalID)
		assert.Equal(t, incomingTx3.TransactionType, insertedTx3.TransactionType)
		assert.Equal(t, incomingTx3.PublicKey, insertedTx3.PublicKey)
		assert.Equal(t, incomingTx3.WasmHash, insertedTx3.WasmHash)
		assert.Equal(t, TransactionStatusPending, insertedTx3.Status)

		assert.Equal(t, incomingTx4.ExternalID, insertedTx4.ExternalID)
		assert.Equal(t, incomingTx4.TransactionType, insertedTx4.TransactionType)
		assert.Equal(t, incomingTx4.SponsoredAccount, insertedTx4.SponsoredAccount)
		assert.Equal(t, incomingTx4.SponsoredTransactionXDR, insertedTx4.SponsoredTransactionXDR)
		assert.Equal(t, TransactionStatusPending, insertedTx4.Status)
	})
}

func Test_TransactionModel_UpdateStatusToSuccess(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	txModel := NewTransactionModel(dbConnectionPool)

	testCases := []struct {
		name              string
		transactionStatus TransactionStatus
		wantErrContains   string
	}{
		{
			name:              "cannot transition PENDING->SUCCESS",
			transactionStatus: TransactionStatusPending,
			wantErrContains:   "attempting to transition transaction status to TransactionStatusSuccess: cannot transition from PENDING to SUCCESS",
		},
		{
			name:              "🎉 successfully transition PROCESSING->SUCCESS",
			transactionStatus: TransactionStatusProcessing,
		},
		{
			name:              "cannot transition SUCCESS->SUCCESS",
			transactionStatus: TransactionStatusSuccess,
			wantErrContains:   "attempting to transition transaction status to TransactionStatusSuccess: cannot transition from SUCCESS to SUCCESS",
		},
		{
			name:              "cannot transition ERROR->SUCCESS",
			transactionStatus: TransactionStatusError,
			wantErrContains:   "attempting to transition transaction status to TransactionStatusSuccess: cannot transition from ERROR to SUCCESS",
		},
	}

	unphazedTx1 := CreateTransactionFixtureNew(t, ctx, dbConnectionPool, TransactionFixture{
		ExternalID:         uuid.NewString(),
		TransactionType:    TransactionTypePayment,
		AssetCode:          "USDC",
		AssetIssuer:        "GCBIRB7Q5T53H4L6P5QSI3O6LPD5MBWGM5GHE7A5NY4XT5OT4VCOEZFX",
		DestinationAddress: "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
		Status:             TransactionStatusPending,
		Amount:             1.23,
		TenantID:           uuid.NewString(),
	})

	unphazedTx2 := CreateTransactionFixtureNew(t, ctx, dbConnectionPool, TransactionFixture{
		ExternalID:      uuid.NewString(),
		TransactionType: TransactionTypeWalletCreation,
		PublicKey:       "04f5549c5ef833ab0ade80d9c1f3fb34fb93092503a8ce105773d676288653df384a024a92cc73cb8089c45ed76ed073433b6a72c64a6ed23630b77327beb65f23",
		WasmHash:        "e5da3b9950524b4276ccf2051e6cc8220bb581e869b892a6ff7812d7709c7a50",
		Status:          TransactionStatusPending,
		TenantID:        uuid.NewString(),
	})

	unphazedTx3 := CreateTransactionFixtureNew(t, ctx, dbConnectionPool, TransactionFixture{
		ExternalID:              uuid.NewString(),
		TransactionType:         TransactionTypeSponsored,
		SponsoredAccount:        "CDTY3P6OVY3SMZXR3DZA667NAXFECA6A3AOZXEU33DD2ACBY43CIKDPT",
		SponsoredTransactionXDR: "AAAAAgAAAADSsOMKYK7a1aALie83F4GQDoBdHrW86UX2SYVygRA+VQACywMAAAfhAAAGugAAAAEAAAAAAAAAAAAAAABoSGeWAAAAAAAAAAEAAAABAAAAANKw4wpgrtrVoAuJ7zcXgZAOgF0etbzpRfZJhXKBED5VAAAAGAAAAAAAAAAB542/zq43Jmbx2PIPe+0FykEDwNgdm5Kb2MegCDjmxIUAAAAIdHJhbnNmZXIAAAADAAAAEgAAAAExfHoGm9ws+/5izT6kpJVi5PnJL+ttrXdkxajBGZv6nAAAABIAAAAAAAAAAFvGtEMyXcvbioU2IKCSomxahpl7lUyef7ftEPxWcD4bAAAACgAAAAAAAAAAAAAAAACYloAAAAABAAAAAQAAAAExfHoGm9ws+/5izT6kpJVi5PnJL+ttrXdkxajBGZv6nH0ObdiOTpYlABXcfAAAABAAAAABAAAAAQAAABEAAAABAAAAAgAAAA8AAAAKcHVibGljX2tleQAAAAAADQAAACDSsOMKYK7a1aALie83F4GQDoBdHrW86UX2SYVygRA+VQAAAA8AAAAJc2lnbmF0dXJlAAAAAAAADQAAAEAQ7ioNcH2wRZxLNA2ssu0tNx2K9DBRsh6u0tVrwkkj0sqwsxvYdrm072z5UE9sUSmcbd5s9CHK+MxSRsrc+gUHAAAAAAAAAAHnjb/OrjcmZvHY8g977QXKQQPA2B2bkpvYx6AIOObEhQAAAAh0cmFuc2ZlcgAAAAMAAAASAAAAATF8egab3Cz7/mLNPqSklWLk+ckv622td2TFqMEZm/qcAAAAEgAAAAAAAAAAW8a0QzJdy9uKhTYgoJKibFqGmXuVTJ5/t+0Q/FZwPhsAAAAKAAAAAAAAAAAAAAAAAJiWgAAAAAAAAAABAAAAAAAAAAMAAAAGAAAAATF8egab3Cz7/mLNPqSklWLk+ckv622td2TFqMEZm/qcAAAAFAAAAAEAAAAGAAAAAeeNv86uNyZm8djyD3vtBcpBA8DYHZuSm9jHoAg45sSFAAAAFAAAAAEAAAAHURIgc/sBvxJigzDM+JHIxvPBqRHTG9KFFsMC7294utoAAAADAAAAAQAAAABbxrRDMl3L24qFNiCgkqJsWoaZe5VMnn+37RD8VnA+GwAAAAFVU0RDAAAAAODia2IsqMlWCuY6k734V/dcCafJwfI1Qq7+/0qEd68AAAAABgAAAAExfHoGm9ws+/5izT6kpJVi5PnJL+ttrXdkxajBGZv6nAAAABV9Dm3Yjk6WJQAAAAAAAAAGAAAAAeeNv86uNyZm8djyD3vtBcpBA8DYHZuSm9jHoAg45sSFAAAAEAAAAAEAAAACAAAADwAAAAdCYWxhbmNlAAAAABIAAAABMXx6BpvcLPv+Ys0+pKSVYuT5yS/rba13ZMWowRmb+pwAAAABABlJmAAADWAAAAGcAAAAAAACyp8AAAABgRA+VQAAAEAVqZBB44AhyhyYi3QN51aEvkGw62m+2D2lSGt0bO4lcUNIL10dN1acoBituE9F1Ypxb+mAyqZFhYLg8vn5n5sP",
		Status:                  TransactionStatusPending,
		TenantID:                uuid.NewString(),
	})

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tx := CreateTransactionFixtureNew(t, ctx, dbConnectionPool, TransactionFixture{
				ExternalID:         uuid.NewString(),
				TransactionType:    TransactionTypePayment,
				AssetCode:          "USDC",
				AssetIssuer:        "GCBIRB7Q5T53H4L6P5QSI3O6LPD5MBWGM5GHE7A5NY4XT5OT4VCOEZFX",
				DestinationAddress: "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
				Status:             tc.transactionStatus,
				Amount:             1.23,
				TenantID:           uuid.NewString(),
			})
			if (tc.transactionStatus != TransactionStatusSuccess) && (tc.transactionStatus != TransactionStatusError) {
				assert.Empty(t, tx.CompletedAt)
			} else {
				assert.NotEmpty(t, tx.CompletedAt)
			}

			updatedTx, err := txModel.UpdateStatusToSuccess(ctx, *tx)
			if tc.wantErrContains != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErrContains)
			} else {
				require.NoError(t, err)
				assert.Equal(t, TransactionStatusSuccess, updatedTx.Status)
				assert.NotEmpty(t, updatedTx.CompletedAt)

				// verify that the only fields that changed are updated_at, completed_at, status and status_history:
				tx.UpdatedAt = updatedTx.UpdatedAt
				tx.CompletedAt = updatedTx.CompletedAt
				tx.Status = updatedTx.Status
				tx.StatusHistory = append(TransactionStatusHistory{}, updatedTx.StatusHistory...)
				assert.Equal(t, tx, updatedTx)
			}

			// verify the unphazed payment transaction was not updated
			refreshUnphazedTx1, err := txModel.Get(ctx, unphazedTx1.ID)
			require.NoError(t, err)
			assert.Equal(t, unphazedTx1, refreshUnphazedTx1)

			// verify the unphazed wallet creation transaction was not updated
			refreshedUnphazedTx2, err := txModel.Get(ctx, unphazedTx2.ID)
			require.NoError(t, err)
			assert.Equal(t, unphazedTx2, refreshedUnphazedTx2)

			// verify the unphazed sponsored transaction was not updated
			refreshedUnphazedTx3, err := txModel.Get(ctx, unphazedTx3.ID)
			require.NoError(t, err)
			assert.Equal(t, unphazedTx3, refreshedUnphazedTx3)
		})
	}
}

func Test_TransactionModel_UpdateStatusToError(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	txModel := NewTransactionModel(dbConnectionPool)

	testCases := []struct {
		name              string
		transactionStatus TransactionStatus
		wantErrContains   string
	}{
		{
			name:              "cannot transition PENDING->ERROR",
			transactionStatus: TransactionStatusPending,
			wantErrContains:   "attempting to transition transaction status to TransactionStatusError: cannot transition from PENDING to ERROR",
		},
		{
			name:              "🎉 successfully transition PROCESSING->ERROR",
			transactionStatus: TransactionStatusProcessing,
		},
		{
			name:              "cannot transition SUCCESS->ERROR",
			transactionStatus: TransactionStatusSuccess,
			wantErrContains:   "attempting to transition transaction status to TransactionStatusError: cannot transition from SUCCESS to ERROR",
		},
		{
			name:              "cannot transition ERROR->ERROR",
			transactionStatus: TransactionStatusError,
			wantErrContains:   "attempting to transition transaction status to TransactionStatusError: cannot transition from ERROR to ERROR",
		},
	}

	unphazedTx1 := CreateTransactionFixtureNew(t, ctx, dbConnectionPool, TransactionFixture{
		ExternalID:         uuid.NewString(),
		TransactionType:    TransactionTypePayment,
		AssetCode:          "USDC",
		AssetIssuer:        "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
		DestinationAddress: "GBHNIYGWZUAVZX7KTLVSMILBXJMUACVO6XBEKIN6RW7AABDFH6S7GK2Y",
		Status:             TransactionStatusPending,
		Amount:             1.23,
		TenantID:           uuid.NewString(),
	})

	unphazedTx2 := CreateTransactionFixtureNew(t, ctx, dbConnectionPool, TransactionFixture{
		ExternalID:      uuid.NewString(),
		TransactionType: TransactionTypeWalletCreation,
		PublicKey:       "04f5549c5ef833ab0ade80d9c1f3fb34fb93092503a8ce105773d676288653df384a024a92cc73cb8089c45ed76ed073433b6a72c64a6ed23630b77327beb65f23",
		WasmHash:        "e5da3b9950524b4276ccf2051e6cc8220bb581e869b892a6ff7812d7709c7a50",
		Status:          TransactionStatusPending,
		TenantID:        uuid.NewString(),
	})

	unphazedTx3 := CreateTransactionFixtureNew(t, ctx, dbConnectionPool, TransactionFixture{
		ExternalID:              uuid.NewString(),
		TransactionType:         TransactionTypeSponsored,
		SponsoredAccount:        "CDTY3P6OVY3SMZXR3DZA667NAXFECA6A3AOZXEU33DD2ACBY43CIKDPT",
		SponsoredTransactionXDR: "AAAAAgAAAADSsOMKYK7a1aALie83F4GQDoBdHrW86UX2SYVygRA+VQACywMAAAfhAAAGugAAAAEAAAAAAAAAAAAAAABoSGeWAAAAAAAAAAEAAAABAAAAANKw4wpgrtrVoAuJ7zcXgZAOgF0etbzpRfZJhXKBED5VAAAAGAAAAAAAAAAB542/zq43Jmbx2PIPe+0FykEDwNgdm5Kb2MegCDjmxIUAAAAIdHJhbnNmZXIAAAADAAAAEgAAAAExfHoGm9ws+/5izT6kpJVi5PnJL+ttrXdkxajBGZv6nAAAABIAAAAAAAAAAFvGtEMyXcvbioU2IKCSomxahpl7lUyef7ftEPxWcD4bAAAACgAAAAAAAAAAAAAAAACYloAAAAABAAAAAQAAAAExfHoGm9ws+/5izT6kpJVi5PnJL+ttrXdkxajBGZv6nH0ObdiOTpYlABXcfAAAABAAAAABAAAAAQAAABEAAAABAAAAAgAAAA8AAAAKcHVibGljX2tleQAAAAAADQAAACDSsOMKYK7a1aALie83F4GQDoBdHrW86UX2SYVygRA+VQAAAA8AAAAJc2lnbmF0dXJlAAAAAAAADQAAAEAQ7ioNcH2wRZxLNA2ssu0tNx2K9DBRsh6u0tVrwkkj0sqwsxvYdrm072z5UE9sUSmcbd5s9CHK+MxSRsrc+gUHAAAAAAAAAAHnjb/OrjcmZvHY8g977QXKQQPA2B2bkpvYx6AIOObEhQAAAAh0cmFuc2ZlcgAAAAMAAAASAAAAATF8egab3Cz7/mLNPqSklWLk+ckv622td2TFqMEZm/qcAAAAEgAAAAAAAAAAW8a0QzJdy9uKhTYgoJKibFqGmXuVTJ5/t+0Q/FZwPhsAAAAKAAAAAAAAAAAAAAAAAJiWgAAAAAAAAAABAAAAAAAAAAMAAAAGAAAAATF8egab3Cz7/mLNPqSklWLk+ckv622td2TFqMEZm/qcAAAAFAAAAAEAAAAGAAAAAeeNv86uNyZm8djyD3vtBcpBA8DYHZuSm9jHoAg45sSFAAAAFAAAAAEAAAAHURIgc/sBvxJigzDM+JHIxvPBqRHTG9KFFsMC7294utoAAAADAAAAAQAAAABbxrRDMl3L24qFNiCgkqJsWoaZe5VMnn+37RD8VnA+GwAAAAFVU0RDAAAAAODia2IsqMlWCuY6k734V/dcCafJwfI1Qq7+/0qEd68AAAAABgAAAAExfHoGm9ws+/5izT6kpJVi5PnJL+ttrXdkxajBGZv6nAAAABV9Dm3Yjk6WJQAAAAAAAAAGAAAAAeeNv86uNyZm8djyD3vtBcpBA8DYHZuSm9jHoAg45sSFAAAAEAAAAAEAAAACAAAADwAAAAdCYWxhbmNlAAAAABIAAAABMXx6BpvcLPv+Ys0+pKSVYuT5yS/rba13ZMWowRmb+pwAAAABABlJmAAADWAAAAGcAAAAAAACyp8AAAABgRA+VQAAAEAVqZBB44AhyhyYi3QN51aEvkGw62m+2D2lSGt0bO4lcUNIL10dN1acoBituE9F1Ypxb+mAyqZFhYLg8vn5n5sP",
		Status:                  TransactionStatusPending,
		TenantID:                uuid.NewString(),
	})

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tx := CreateTransactionFixtureNew(t, ctx, dbConnectionPool, TransactionFixture{
				ExternalID:         uuid.NewString(),
				TransactionType:    TransactionTypePayment,
				AssetCode:          "USDC",
				AssetIssuer:        "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
				DestinationAddress: "GBHNIYGWZUAVZX7KTLVSMILBXJMUACVO6XBEKIN6RW7AABDFH6S7GK2Y",
				Status:             tc.transactionStatus,
				Amount:             1.23,
				TenantID:           uuid.NewString(),
			})
			assert.Empty(t, tx.StatusMessage)
			if (tc.transactionStatus != TransactionStatusSuccess) && (tc.transactionStatus != TransactionStatusError) {
				assert.Empty(t, tx.CompletedAt)
			} else {
				assert.NotEmpty(t, tx.CompletedAt)
			}

			const someErrMessage = "some error message"
			updatedTx, err := txModel.UpdateStatusToError(ctx, *tx, someErrMessage)
			if tc.wantErrContains != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErrContains)
			} else {
				require.NoError(t, err)
				assert.Equal(t, TransactionStatusError, updatedTx.Status)
				assert.NotEmpty(t, updatedTx.CompletedAt)

				// verify that the only fields that changed are updated_at, completed_at, status, status_message and status history:
				tx.UpdatedAt = updatedTx.UpdatedAt
				tx.CompletedAt = updatedTx.CompletedAt
				tx.Status = updatedTx.Status
				tx.StatusMessage = sql.NullString{String: someErrMessage, Valid: true}
				tx.StatusHistory = append(TransactionStatusHistory{}, updatedTx.StatusHistory...)
				assert.Equal(t, tx, updatedTx)
			}

			// verify the unphazed payment transaction was not updated
			refreshedUnphazedTx1, err := txModel.Get(ctx, unphazedTx1.ID)
			require.NoError(t, err)
			assert.Equal(t, unphazedTx1, refreshedUnphazedTx1)

			// verify the unphazed wallet creation transaction was not updated
			refreshedUnphazedTx2, err := txModel.Get(ctx, unphazedTx2.ID)
			require.NoError(t, err)
			assert.Equal(t, unphazedTx2, refreshedUnphazedTx2)

			// verify the unphazed sponsored transaction was not updated
			refreshedUnphazedTx3, err := txModel.Get(ctx, unphazedTx3.ID)
			require.NoError(t, err)
			assert.Equal(t, unphazedTx3, refreshedUnphazedTx3)
		})
	}
}

func Test_TransactionModel_UpdateStellarTransactionHashXDRSentAndDistributionAccount(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	txModel := NewTransactionModel(dbConnectionPool)

	const txHash = "3389e9f0f1a65f19736cacf544c2e825313e8447f569233bb8db39aa607c8889"
	const envelopeXDR = "AAAAAGL8HQvQkbK2HA3WVjRrKmjX00fG8sLI7m0ERwJW/AX3AAAACgAAAAAAAAABAAAAAAAAAAAAAAABAAAAAAAAAAAAAAAArqN6LeOagjxMaUP96Bzfs9e0corNZXzBWJkFoK7kvkwAAAAAO5rKAAAAAAAAAAABVvwF9wAAAEAKZ7IPj/46PuWU6ZOtyMosctNAkXRNX9WCAI5RnfRk+AyxDLoDZP/9l3NvsxQtWj9juQOuoBlFLnWu8intgxQA"
	const resultXDR = "AAAAAAAAAGQAAAAAAAAAAQAAAAAAAAAOAAAAAAAAAABw2JZZYIt4n/WXKcnDow3mbTBMPrOnldetgvGUlpTSEQAAAAA="

	testCases := []struct {
		name                string
		transaction         Transaction
		txHash              string
		xdrSent             string
		distributionAccount string
		wantErrContains     string
	}{
		{
			name:            "returns an error if the size of the txHash if invalid",
			txHash:          "invalid-tx-hash",
			wantErrContains: `invalid transaction hash "invalid-tx-hash"`,
		},
		{
			name:                "returns an error if distribution account is invalid",
			txHash:              txHash,
			xdrSent:             envelopeXDR,
			distributionAccount: "invalid-account",
			wantErrContains:     `distribution account "invalid-account" is not a valid ed25519 public key`,
		},
		{
			name:            "returns an error if XDR is empty",
			txHash:          txHash,
			wantErrContains: "invalid XDR envelope: decoding EnvelopeType: decoding EnvelopeType: xdr:DecodeInt: EOF while decoding 4 bytes - read: '[]'",
		},
		{
			name:            "returns an error if XDR is not a valid base64 encoded",
			txHash:          txHash,
			xdrSent:         "not-base-64-encoded",
			wantErrContains: "invalid XDR envelope: decoding EnvelopeType: decoding EnvelopeType: xdr:DecodeInt: illegal base64 data at input byte",
		},
		{
			name:            "returns an error if XDR is not a transaction envelope",
			txHash:          txHash,
			xdrSent:         resultXDR,
			wantErrContains: "invalid XDR envelope: decoding TransactionV0Envelope: decoding TransactionV0: decoding TimeBounds",
		},
		{
			name:                "🎉 successfully validate the tx hash, XDR envelope, distribution account and save them to the DB",
			txHash:              txHash,
			xdrSent:             envelopeXDR,
			distributionAccount: "GCKFBEIYTKP5RDBEJYOR6JKDKYBWB5DBJNXKFRKWKJZKGB3BVGFHBVFS",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// create a new transaction
			tx, err := txModel.Insert(ctx, Transaction{
				ExternalID:      uuid.NewString(),
				TransactionType: TransactionTypePayment,
				Payment: Payment{
					AssetCode:   "USDC",
					AssetIssuer: "GCBIRB7Q5T53H4L6P5QSI3O6LPD5MBWGM5GHE7A5NY4XT5OT4VCOEZFX",
					Amount:      1,
					Destination: "GBHNIYGWZUAVZX7KTLVSMILBXJMUACVO6XBEKIN6RW7AABDFH6S7GK2Y",
				},
				TenantID: uuid.NewString(),
			})
			require.NoError(t, err)
			require.NotNil(t, tx)

			// verify the transaction was created
			originalTx, err := txModel.Get(ctx, tx.ID)
			require.NoError(t, err)

			assert.False(t, originalTx.XDRSent.Valid)
			assert.Equal(t, "", originalTx.XDRSent.String)
			assert.False(t, originalTx.StellarTransactionHash.Valid)
			assert.Equal(t, "", originalTx.StellarTransactionHash.String)
			assert.Nil(t, originalTx.SentAt)
			assert.Len(t, originalTx.StatusHistory, 1)
			initialStatusHistory := originalTx.StatusHistory[0]

			distributionAccount := tc.distributionAccount
			if distributionAccount == "" {
				distributionAccount = "GCKFBEIYTKP5RDBEJYOR6JKDKYBWB5DBJNXKFRKWKJZKGB3BVGFHBVFS"
			}
			updatedTx, err := txModel.UpdateStellarTransactionHashXDRSentAndDistributionAccount(ctx, tx.ID, tc.txHash, tc.xdrSent, distributionAccount)
			if tc.wantErrContains != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErrContains)
				assert.Nil(t, updatedTx)
			} else {
				// check if object has been updated correctly
				require.NoError(t, err)
				assert.True(t, updatedTx.XDRSent.Valid)
				assert.Equal(t, envelopeXDR, updatedTx.XDRSent.String)
				assert.True(t, updatedTx.StellarTransactionHash.Valid)
				assert.Equal(t, txHash, updatedTx.StellarTransactionHash.String)
				assert.True(t, updatedTx.DistributionAccount.Valid)
				assert.Equal(t, distributionAccount, updatedTx.DistributionAccount.String)
				assert.NotNil(t, updatedTx.SentAt)
				assert.Equal(t, originalTx.AttemptsCount+1, updatedTx.AttemptsCount)

				// assert new status history info:
				assert.Len(t, updatedTx.StatusHistory, 2)
				newStatusHist := updatedTx.StatusHistory[1]
				assert.Equal(t, string(updatedTx.Status), newStatusHist.Status)
				assert.Equal(t, updatedTx.StellarTransactionHash.String, newStatusHist.StellarTransactionHash)
				assert.Equal(t, updatedTx.XDRSent.String, newStatusHist.XDRSent)
				assert.Empty(t, updatedTx.XDRReceived)
				wantStatusHistory := TransactionStatusHistory{initialStatusHistory, newStatusHist}

				// retrieve the transaction from the database and check if values are updated
				refreshedTx, err := txModel.Get(ctx, tx.ID)
				require.NoError(t, err)
				assert.Equal(t, updatedTx, refreshedTx)

				// make sure only the expected fields were updated:
				originalTx.XDRSent = refreshedTx.XDRSent
				originalTx.StellarTransactionHash = refreshedTx.StellarTransactionHash
				originalTx.DistributionAccount = refreshedTx.DistributionAccount
				originalTx.SentAt = refreshedTx.SentAt
				originalTx.UpdatedAt = refreshedTx.UpdatedAt
				originalTx.StatusHistory = wantStatusHistory
				originalTx.AttemptsCount += 1
				assert.Equal(t, refreshedTx, originalTx)
			}
		})
	}
}

func Test_TransactionModel_UpdateStellarTransactionXDRReceived(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	txModel := NewTransactionModel(dbConnectionPool)

	const envelopeXDR = "AAAAAGL8HQvQkbK2HA3WVjRrKmjX00fG8sLI7m0ERwJW/AX3AAAACgAAAAAAAAABAAAAAAAAAAAAAAABAAAAAAAAAAAAAAAArqN6LeOagjxMaUP96Bzfs9e0corNZXzBWJkFoK7kvkwAAAAAO5rKAAAAAAAAAAABVvwF9wAAAEAKZ7IPj/46PuWU6ZOtyMosctNAkXRNX9WCAI5RnfRk+AyxDLoDZP/9l3NvsxQtWj9juQOuoBlFLnWu8intgxQA"
	const resultXDR = "AAAAAAAAAGQAAAAAAAAAAQAAAAAAAAAOAAAAAAAAAABw2JZZYIt4n/WXKcnDow3mbTBMPrOnldetgvGUlpTSEQAAAAA="

	testCases := []struct {
		name            string
		transaction     Transaction
		xdrReceived     string
		wantErrContains string
	}{
		{
			name:            "returns an error if XDR is empty",
			xdrReceived:     "",
			wantErrContains: "invalid XDR result: decoding Int64: decoding Hyper: xdr:DecodeHyper: EOF while decoding 8 bytes - read: '[]'",
		},
		{
			name:            "returns an error if XDR is not a valid base64 encoded",
			xdrReceived:     "not-base-64-encoded",
			wantErrContains: "invalid XDR result: decoding Int64: decoding Hyper: xdr:DecodeHyper: illegal base64 data",
		},
		{
			name:            "returns an error if XDR is not a transaction envelope",
			xdrReceived:     envelopeXDR,
			wantErrContains: "invalid XDR result: decoding TransactionResultResult: decoding TransactionResultCode: '-795757898' is not a valid TransactionResultCode enum value",
		},
		{
			name:        "🎉 successfully validate a transaction result and save it in the DB",
			xdrReceived: resultXDR,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// create a new transaction
			tx, err := txModel.Insert(ctx, Transaction{
				ExternalID:      uuid.NewString(),
				TransactionType: TransactionTypePayment,
				Payment: Payment{
					AssetCode:   "USDC",
					AssetIssuer: "GCBIRB7Q5T53H4L6P5QSI3O6LPD5MBWGM5GHE7A5NY4XT5OT4VCOEZFX",
					Amount:      1,
					Destination: "GBHNIYGWZUAVZX7KTLVSMILBXJMUACVO6XBEKIN6RW7AABDFH6S7GK2Y",
				},
				TenantID: uuid.NewString(),
			})
			require.NoError(t, err)
			require.NotNil(t, tx)

			assert.Equal(t, false, tx.XDRReceived.Valid)
			assert.Equal(t, "", tx.XDRReceived.String)

			updatedTx, err := txModel.UpdateStellarTransactionXDRReceived(ctx, tx.ID, tc.xdrReceived)
			if tc.wantErrContains != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErrContains)
			} else {
				// check if object has been updated correctly
				require.NoError(t, err)
				assert.Equal(t, true, updatedTx.XDRReceived.Valid)
				assert.Equal(t, resultXDR, updatedTx.XDRReceived.String)

				// retrieve the transaction from the database and check if values are updated
				refreshedTx, err := txModel.Get(ctx, tx.ID)
				require.NoError(t, err)
				assert.Equal(t, refreshedTx, updatedTx)
			}
		})
	}
}

func Test_Transaction_validate_payment(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()
	require.NoError(t, err)

	testCases := []struct {
		name            string
		transaction     Transaction
		wantErrContains string
	}{
		{
			name: "validate ExternalID",
			transaction: Transaction{
				TransactionType: TransactionTypePayment,
			},
			wantErrContains: "external ID is required",
		},
		{
			name: "validate AssetCode (min size)",
			transaction: Transaction{
				ExternalID:      "123",
				TransactionType: TransactionTypePayment,
				TenantID:        "tenant-id",
			},
			wantErrContains: "asset code must have between 1 and 12 characters",
		},
		{
			name: "validate AssetCode (max size)",
			transaction: Transaction{
				ExternalID:      "123",
				TransactionType: TransactionTypePayment,
				Payment: Payment{
					AssetCode: "1234567890123",
				},
				TenantID: "tenant-id",
			},
			wantErrContains: "asset code must have between 1 and 12 characters",
		},
		{
			name: "validate AssetIssuer (cannot be nil)",
			transaction: Transaction{
				ExternalID:      "123",
				TransactionType: TransactionTypePayment,
				Payment: Payment{
					AssetCode: "USDC",
				},
				TenantID: "tenant-id",
			},
			wantErrContains: "asset issuer is required",
		},
		{
			name: "validate AssetIssuer (not a valid public key)",
			transaction: Transaction{
				ExternalID:      "123",
				TransactionType: TransactionTypePayment,
				Payment: Payment{
					AssetCode:   "USDC",
					AssetIssuer: "invalid-issuer",
				},
				TenantID: "tenant-id",
			},
			wantErrContains: `asset issuer "invalid-issuer" is not a valid ed25519 public key`,
		},
		{
			name: "validate Amount",
			transaction: Transaction{
				ExternalID:      "123",
				TransactionType: TransactionTypePayment,
				Payment: Payment{
					AssetCode:   "USDC",
					AssetIssuer: "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
				},
				TenantID: "tenant-id",
			},
			wantErrContains: "amount must be positive",
		},
		{
			name: "validate Destination",
			transaction: Transaction{
				ExternalID:      "123",
				TransactionType: TransactionTypePayment,
				Payment: Payment{
					AssetCode:   "USDC",
					AssetIssuer: "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
					Amount:      100.0,
					Destination: "invalid-destination",
				},
				TenantID: "tenant-id",
			},
			wantErrContains: `destination "invalid-destination" is not a valid ed25519 public key`,
		},
		{
			name: "validate tenant ID",
			transaction: Transaction{
				ExternalID:      "123",
				TransactionType: TransactionTypePayment,
				Payment: Payment{
					AssetCode:   "USDC",
					AssetIssuer: "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
					Amount:      100.0,
					Destination: "GDUCE34WW5Z34GMCEPURYANUCUP47J6NORJLKC6GJNMDLN4ZI4PMI2MG",
				},
				TenantID: "",
			},
			wantErrContains: `tenant ID is required`,
		},
	}

	validAddresses := []string{
		"GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
		"CAMAMZUOULVWFAB3KRROW5ELPUFHSEKPUALORCFBLFX7XBWWUCUJLR53",
	}

	for _, address := range validAddresses {
		testCases = append(testCases, struct {
			name            string
			transaction     Transaction
			wantErrContains string
		}{
			name: fmt.Sprintf("🎉 successfully validate XLM transaction with (%c) destination", address[0]),
			transaction: Transaction{
				ExternalID:      "123",
				TransactionType: TransactionTypePayment,
				Payment: Payment{
					AssetCode:   "XLM",
					Amount:      100.0,
					Destination: address,
				},
				TenantID: "tenant-id",
			},
		})

		testCases = append(testCases, struct {
			name            string
			transaction     Transaction
			wantErrContains string
		}{
			name: fmt.Sprintf("🎉 successfully validate USDC transaction with (%c) destination", address[0]),
			transaction: Transaction{
				ExternalID:      "123",
				TransactionType: TransactionTypePayment,
				Payment: Payment{
					AssetCode:   "USDC",
					AssetIssuer: "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
					Amount:      100.0,
					Destination: address,
				},
				TenantID: "tenant-id",
			},
		})
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.transaction.validate()
			if tc.wantErrContains == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErrContains)
			}
		})
	}
}

func Test_Transaction_validate_wallet_creation(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()
	require.NoError(t, err)

	testCases := []struct {
		name            string
		transaction     Transaction
		wantErrContains string
	}{
		{
			name: "validate ExternalID",
			transaction: Transaction{
				TransactionType: TransactionTypeWalletCreation,
			},
			wantErrContains: "external ID is required",
		},
		{
			name: "validate PublicKey",
			transaction: Transaction{
				ExternalID:      "123",
				TransactionType: TransactionTypeWalletCreation,
				TenantID:        "tenant-id",
			},
			wantErrContains: "public key is required",
		},
		{
			name: "validate PublicKey (not a valid public key)",
			transaction: Transaction{
				ExternalID:      "123",
				TransactionType: TransactionTypeWalletCreation,
				WalletCreation: WalletCreation{
					PublicKey: "invalid-public-key",
					WasmHash:  "e5da3b9950524b4276ccf2051e6cc8220bb581e869b892a6ff7812d7709c7a50",
				},
				TenantID: "tenant-id",
			},
			wantErrContains: `public key "invalid-public-key" is not a valid hex string`,
		},
		{
			name: "validate WasmHash",
			transaction: Transaction{
				ExternalID:      "123",
				TransactionType: TransactionTypeWalletCreation,
				WalletCreation: WalletCreation{
					PublicKey: "04f5549c5ef833ab0ade80d9c1f3fb34fb93092503a8ce105773d676288653df384a024a92cc73cb8089c45ed76ed073433b6a72c64a6ed23630b77327beb65f23",
					WasmHash:  "",
				},
				TenantID: "tenant-id",
			},
			wantErrContains: "wasm hash is required",
		},
		{
			name: "validate WasmHash (not a valid hash)",
			transaction: Transaction{
				ExternalID:      "123",
				TransactionType: TransactionTypeWalletCreation,
				WalletCreation: WalletCreation{
					PublicKey: "04f5549c5ef833ab0ade80d9c1f3fb34fb93092503a8ce105773d676288653df384a024a92cc73cb8089c45ed76ed073433b6a72c64a6ed23630b77327beb65f23",
					WasmHash:  "invalid-wasm-hash",
				},
				TenantID: "tenant-id",
			},
			wantErrContains: `wasm hash "invalid-wasm-hash" is not a valid hex string`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.transaction.validate()
			if tc.wantErrContains == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErrContains)
			}
		})
	}
}

func Test_Transaction_validate_sponsored(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()
	require.NoError(t, err)

	testCases := []struct {
		name            string
		transaction     Transaction
		wantErrContains string
	}{
		{
			name: "validate ExternalID",
			transaction: Transaction{
				TransactionType: TransactionTypeSponsored,
			},
			wantErrContains: "external ID is required",
		},
		{
			name: "validate SponsoredAccount",
			transaction: Transaction{
				ExternalID:      "123",
				TransactionType: TransactionTypeSponsored,
				TenantID:        "tenant-id",
			},
			wantErrContains: "sponsored account is required",
		},
		{
			name: "validate SponsoredAccount (not a contract address)",
			transaction: Transaction{
				ExternalID:      "123",
				TransactionType: TransactionTypeSponsored,
				Sponsored: Sponsored{
					SponsoredAccount: "invalid-account",
				},
				TenantID: "tenant-id",
			},
			wantErrContains: `sponsored account "invalid-account" is not a valid contract address`,
		},
		{
			name: "validate SponsoredTransactionXDR",
			transaction: Transaction{
				ExternalID:      "123",
				TransactionType: TransactionTypeSponsored,
				Sponsored: Sponsored{
					SponsoredAccount: "CDTY3P6OVY3SMZXR3DZA667NAXFECA6A3AOZXEU33DD2ACBY43CIKDPT",
				},
				TenantID: "tenant-id",
			},
			wantErrContains: "sponsored transaction XDR is required",
		},
		{
			name: "validate TransactionXDR (not a valid hash)",
			transaction: Transaction{
				ExternalID:      "123",
				TransactionType: TransactionTypeSponsored,
				Sponsored: Sponsored{
					SponsoredAccount:        "CDTY3P6OVY3SMZXR3DZA667NAXFECA6A3AOZXEU33DD2ACBY43CIKDPT",
					SponsoredTransactionXDR: "invalid-transaction-xdr",
				},
				TenantID: "tenant-id",
			},
			wantErrContains: `sponsored transaction XDR "invalid-transaction-xdr"`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.transaction.validate()
			if tc.wantErrContains == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErrContains)
			}
		})
	}
}

func Test_TransactionModel_GetTransactionBatchForUpdate(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	txModel := NewTransactionModel(dbConnectionPool)

	testCase := []struct {
		name              string
		transactionStatus TransactionStatus
		shouldBeFound     bool
		batchSize         int
		wantErrContains   string
	}{
		{
			name:              "batchSize must be >= 0",
			transactionStatus: TransactionStatusSuccess,
			batchSize:         0,
			wantErrContains:   "batch size must be greater than 0",
			shouldBeFound:     false,
		},
		{
			name:              "no transactions found (empty database)",
			transactionStatus: "",
			batchSize:         100,
			shouldBeFound:     false,
		},
		{
			name:              "no transactions found (PENDING)",
			transactionStatus: TransactionStatusPending,
			batchSize:         100,
			shouldBeFound:     false,
		},
		{
			name:              "no transactions found (PROCESSING)",
			transactionStatus: TransactionStatusProcessing,
			batchSize:         100,
			shouldBeFound:     false,
		},
		{
			name:              "🎉 transactions successfully found (SUCCESS)",
			transactionStatus: TransactionStatusSuccess,
			batchSize:         100,
			shouldBeFound:     true,
		},
		{
			name:              "🎉 transactions successfully found (ERROR)",
			transactionStatus: TransactionStatusError,
			batchSize:         100,
			shouldBeFound:     true,
		},
	}

	const txCount = 3
	for _, tc := range testCase {
		t.Run(tc.name, func(t *testing.T) {
			dbTx, err := dbConnectionPool.BeginTxx(ctx, nil)
			require.NoError(t, err)
			defer func() {
				err = dbTx.Rollback()
				require.NoError(t, err)
			}()

			tenantID := uuid.NewString()
			var transactions []*Transaction
			if tc.transactionStatus != "" {
				transactions = CreateTransactionFixturesNew(t, ctx, dbTx, txCount, TransactionFixture{
					TransactionType:    TransactionTypePayment,
					AssetCode:          "USDC",
					AssetIssuer:        "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
					DestinationAddress: "GBHNIYGWZUAVZX7KTLVSMILBXJMUACVO6XBEKIN6RW7AABDFH6S7GK2Y",
					Status:             tc.transactionStatus,
					Amount:             1.2,
					TenantID:           tenantID,
				})
			}
			var txIDs []string
			for _, tx := range transactions {
				txIDs = append(txIDs, tx.ID)
			}

			foundTransactions, err := txModel.GetTransactionBatchForUpdate(ctx, dbTx, tc.batchSize, tenantID, TransactionTypePayment)
			if tc.wantErrContains == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErrContains)
			}

			var foundTxIDs []string
			for _, tx := range foundTransactions {
				foundTxIDs = append(foundTxIDs, tx.ID)
			}

			if !tc.shouldBeFound {
				assert.Equal(t, 0, len(foundTransactions))
			} else {
				assert.Equal(t, txCount, len(foundTransactions))
				assert.ElementsMatch(t, txIDs, foundTxIDs)
			}
		})
	}

	DeleteAllTransactionFixtures(t, ctx, dbConnectionPool)
}

func Test_TransactionModel_GetTransactionBatchForUpdate_WithTransactionTypes(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	txModel := NewTransactionModel(dbConnectionPool)

	tenantID := uuid.NewString()

	dbTx, err := dbConnectionPool.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() {
		err = dbTx.Rollback()
		require.NoError(t, err)
	}()

	// Create payment transactions
	paymentTxs := CreateTransactionFixturesNew(t, ctx, dbTx, 2, TransactionFixture{
		TransactionType:    TransactionTypePayment,
		AssetCode:          "USDC",
		AssetIssuer:        "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
		DestinationAddress: "GBHNIYGWZUAVZX7KTLVSMILBXJMUACVO6XBEKIN6RW7AABDFH6S7GK2Y",
		Status:             TransactionStatusSuccess,
		Amount:             1.2,
		TenantID:           tenantID,
	})

	// Create wallet creation transactions
	walletCreationTxs := CreateTransactionFixturesNew(t, ctx, dbTx, 2, TransactionFixture{
		TransactionType: TransactionTypeWalletCreation,
		PublicKey:       "deadbeef",
		WasmHash:        "cafebabe",
		Status:          TransactionStatusSuccess,
		TenantID:        tenantID,
	})

	t.Run("returns only payment transactions when payment type specified", func(t *testing.T) {
		foundTransactions, err := txModel.GetTransactionBatchForUpdate(ctx, dbTx, 10, tenantID, TransactionTypePayment)
		require.NoError(t, err)
		assert.Equal(t, 2, len(foundTransactions))

		var foundTxIDs []string
		for _, tx := range foundTransactions {
			foundTxIDs = append(foundTxIDs, tx.ID)
			assert.Equal(t, TransactionTypePayment, tx.TransactionType)
		}

		var expectedTxIDs []string
		for _, tx := range paymentTxs {
			expectedTxIDs = append(expectedTxIDs, tx.ID)
		}
		assert.ElementsMatch(t, expectedTxIDs, foundTxIDs)
	})

	t.Run("returns only wallet creation transactions when wallet creation type specified", func(t *testing.T) {
		foundTransactions, err := txModel.GetTransactionBatchForUpdate(ctx, dbTx, 10, tenantID, TransactionTypeWalletCreation)
		require.NoError(t, err)
		assert.Equal(t, 2, len(foundTransactions))

		var foundTxIDs []string
		for _, tx := range foundTransactions {
			foundTxIDs = append(foundTxIDs, tx.ID)
			assert.Equal(t, TransactionTypeWalletCreation, tx.TransactionType)
		}

		var expectedTxIDs []string
		for _, tx := range walletCreationTxs {
			expectedTxIDs = append(expectedTxIDs, tx.ID)
		}
		assert.ElementsMatch(t, expectedTxIDs, foundTxIDs)
	})

	t.Run("returns empty when non-existent transaction type specified", func(t *testing.T) {
		foundTransactions, err := txModel.GetTransactionBatchForUpdate(ctx, dbTx, 10, tenantID, TransactionTypeSponsored)
		require.NoError(t, err)
		assert.Equal(t, 0, len(foundTransactions))
	})

	DeleteAllTransactionFixtures(t, ctx, dbConnectionPool)
}

func Test_TransactionModel_GetTransactionPendingUpdateByID(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	txModel := NewTransactionModel(dbConnectionPool)

	testCase := []struct {
		name              string
		transactionStatus TransactionStatus
		shouldBeFound     bool
		wantErr           error
	}{
		{
			name:              "transaction not found (NOT IN DB)",
			transactionStatus: "",
			shouldBeFound:     false,
			wantErr:           ErrRecordNotFound,
		},
		{
			name:              "transaction not found (PENDING)",
			transactionStatus: TransactionStatusPending,
			shouldBeFound:     false,
			wantErr:           ErrRecordNotFound,
		},
		{
			name:              "transaction not found (PROCESSING)",
			transactionStatus: TransactionStatusProcessing,
			shouldBeFound:     false,
			wantErr:           ErrRecordNotFound,
		},
		{
			name:              "🎉 transactions successfully found (SUCCESS)",
			transactionStatus: TransactionStatusSuccess,
			shouldBeFound:     true,
		},
		{
			name:              "🎉 transactions successfully found (ERROR)",
			transactionStatus: TransactionStatusError,
			shouldBeFound:     true,
		},
	}

	for _, tc := range testCase {
		t.Run(tc.name, func(t *testing.T) {
			dbTx, err := dbConnectionPool.BeginTxx(ctx, nil)
			require.NoError(t, err)
			defer func() {
				err = dbTx.Rollback()
				require.NoError(t, err)
			}()

			var tx Transaction
			if tc.transactionStatus != "" {
				tx = *CreateTransactionFixtureNew(t, ctx, dbTx, TransactionFixture{
					TransactionType:    TransactionTypePayment,
					AssetCode:          "USDC",
					AssetIssuer:        "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
					DestinationAddress: "GBHNIYGWZUAVZX7KTLVSMILBXJMUACVO6XBEKIN6RW7AABDFH6S7GK2Y",
					Status:             tc.transactionStatus,
					Amount:             1.2,
					TenantID:           uuid.NewString(),
				})
				defer DeleteAllTransactionFixtures(t, ctx, dbConnectionPool)
			}

			foundTransaction, err := txModel.GetTransactionPendingUpdateByID(ctx, dbTx, tx.ID, TransactionTypePayment)
			if tc.wantErr == nil {
				require.NoError(t, err)
				assert.Equal(t, tx, *foundTransaction)
			} else {
				require.Error(t, err)
				assert.EqualError(t, err, tc.wantErr.Error())
			}
		})
	}
}

func Test_TransactionModel_UpdateSyncedTransactions(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	txModel := NewTransactionModel(dbConnectionPool)

	testCase := []struct {
		name                 string
		shouldSendEmptyIDs   bool
		shouldSendInvalidIDs bool
		transactionStatus    TransactionStatus
		wantErrContains      string
	}{
		{
			name:               "rerturn an error if txIDs is empty",
			shouldSendEmptyIDs: true,
			wantErrContains:    "no transaction IDs provided",
		},
		{
			name:                 "rerturn an error if the IDs sent don't exist",
			shouldSendInvalidIDs: true,
			wantErrContains:      "expected 1 rows to be affected, got 0",
		},
		{
			name:              "rerturn an error if the IDs sent were not ready to be synched (PENDING)",
			transactionStatus: TransactionStatusPending,
			wantErrContains:   "expected 3 rows to be affected, got 0",
		},
		{
			name:              "rerturn an error if the IDs sent were not ready to be synched (PROCESSING)",
			transactionStatus: TransactionStatusProcessing,
			wantErrContains:   "expected 3 rows to be affected, got 0",
		},
		{
			name:              "🎉 successfully set the status of transactions to synched (SUCCESS)",
			transactionStatus: TransactionStatusSuccess,
		},
		{
			name:              "🎉 successfully set the status of transactions to synched (ERROR)",
			transactionStatus: TransactionStatusError,
		},
	}

	const txCount = 3
	for _, tc := range testCase {
		t.Run(tc.name, func(t *testing.T) {
			dbTx, err := dbConnectionPool.BeginTxx(ctx, nil)
			require.NoError(t, err)
			defer func() {
				err = dbTx.Rollback()
				require.NoError(t, err)
			}()

			// create transactions and get their IDs
			var txIDs []string
			if tc.shouldSendEmptyIDs {
				txIDs = []string{}
			} else if tc.shouldSendInvalidIDs {
				txIDs = []string{"invalid-id"}
			} else {
				transactions := CreateTransactionFixturesNew(t, ctx, dbTx, txCount, TransactionFixture{
					TransactionType:    TransactionTypePayment,
					AssetCode:          "USDC",
					AssetIssuer:        "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
					DestinationAddress: "GBHNIYGWZUAVZX7KTLVSMILBXJMUACVO6XBEKIN6RW7AABDFH6S7GK2Y",
					Status:             tc.transactionStatus,
					Amount:             1.2,
					TenantID:           uuid.NewString(),
				})
				for _, tx := range transactions {
					txIDs = append(txIDs, tx.ID)
				}
			}

			err = txModel.UpdateSyncedTransactions(ctx, dbTx, txIDs)

			// count the number of transactions that were synched
			var count int
			countErr := dbTx.GetContext(ctx, &count, "SELECT COUNT(*) FROM submitter_transactions WHERE synced_at IS NOT NULL")
			require.NoError(t, countErr)

			if tc.wantErrContains != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErrContains)
				assert.Equal(t, 0, count)
			} else {
				require.NoError(t, err)
				assert.Equal(t, txCount, count)
			}
		})
	}

	DeleteAllTransactionFixtures(t, ctx, dbConnectionPool)
}

func Test_TransactionModel_queryFilterForLockedState(t *testing.T) {
	txModel := &TransactionModel{}

	testCases := []struct {
		name         string
		locked       bool
		ledgerNumber int32
		wantFilter   string
	}{
		{
			name:         "locked to ledgerNumber=10",
			locked:       true,
			ledgerNumber: 10,
			wantFilter:   "(locked_until_ledger_number >= 10)",
		},
		{
			name:         "unlocked or expired on ledgerNumber=20",
			locked:       false,
			ledgerNumber: 20,
			wantFilter:   "(locked_until_ledger_number IS NULL OR locked_until_ledger_number < 20)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotFilter := txModel.queryFilterForLockedState(tc.locked, tc.ledgerNumber)
			assert.Equal(t, tc.wantFilter, gotFilter)
		})
	}
}

func Test_TransactionModel_Lock(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	transactionModel := TransactionModel{DBConnectionPool: dbConnectionPool}

	const currentLedger int32 = 10
	const nextLedgerLock int32 = 20

	testCases := []struct {
		name                     string
		initialLockedAt          sql.NullTime
		initialSyncedAt          sql.NullTime
		initialStatus            TransactionStatus
		initialLockedUntilLedger sql.NullInt32
		expectedErrContains      string
	}{
		{
			name:          "🎉 successfully locks transaction without any previous lock (PENDING)",
			initialStatus: TransactionStatusPending,
		},
		{
			name:          "🎉 successfully locks transaction without any previous lock (PROCESSING)",
			initialStatus: TransactionStatusProcessing,
		},
		{
			name:                     "🎉 successfully locks transaction with lock expired",
			initialStatus:            TransactionStatusPending,
			initialLockedAt:          sql.NullTime{Time: time.Now().Add(-1 * time.Hour), Valid: true},
			initialLockedUntilLedger: sql.NullInt32{Int32: currentLedger - 1, Valid: true},
		},
		{
			name:                     "🚧 cannot be locked again if still locked",
			initialStatus:            TransactionStatusPending,
			initialLockedAt:          sql.NullTime{Time: time.Now().Add(-1 * time.Hour), Valid: true},
			initialLockedUntilLedger: sql.NullInt32{Int32: currentLedger, Valid: true},
			expectedErrContains:      ErrRecordNotFound.Error(),
		},
		{
			name:                "🚧 cannot be locked if the status is SUCCESS",
			initialStatus:       TransactionStatusSuccess,
			expectedErrContains: ErrRecordNotFound.Error(),
		},
		{
			name:                "🚧 cannot be locked if the status is ERROR",
			initialStatus:       TransactionStatusError,
			expectedErrContains: ErrRecordNotFound.Error(),
		},
		{
			name:                "🚧 cannot be locked if siced_at is not empty",
			initialStatus:       TransactionStatusPending,
			initialSyncedAt:     sql.NullTime{Time: time.Now().Add(-1 * time.Hour), Valid: true},
			expectedErrContains: ErrRecordNotFound.Error(),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tx := CreateTransactionFixtureNew(t, ctx, dbConnectionPool, TransactionFixture{
				AssetCode:          "USDC",
				TransactionType:    TransactionTypePayment,
				AssetIssuer:        "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
				DestinationAddress: "GCBIRB7Q5T53H4L6P5QSI3O6LPD5MBWGM5GHE7A5NY4XT5OT4VCOEZFX",
				Status:             tc.initialStatus,
				Amount:             1,
				TenantID:           uuid.NewString(),
			})
			q := `UPDATE submitter_transactions SET locked_at = $1, locked_until_ledger_number = $2, synced_at = $3, status = $4 WHERE id = $5`
			_, err := dbConnectionPool.ExecContext(ctx, q, tc.initialLockedAt, tc.initialLockedUntilLedger, tc.initialSyncedAt, tc.initialStatus, tx.ID)
			require.NoError(t, err)

			tx, err = transactionModel.Lock(ctx, dbConnectionPool, tx.ID, currentLedger, nextLedgerLock)

			if tc.expectedErrContains == "" {
				require.NoError(t, err)
				tx, err = transactionModel.Get(ctx, tx.ID)
				require.NoError(t, err)
				assert.NotNil(t, tx.LockedAt)
				assert.True(t, tx.LockedUntilLedgerNumber.Valid)
				assert.Equal(t, nextLedgerLock, tx.LockedUntilLedgerNumber.Int32)
				assert.Equal(t, TransactionStatusProcessing, tx.Status)

				var txRefreshed *Transaction
				txRefreshed, err = transactionModel.Get(ctx, tx.ID)
				require.NoError(t, err)
				require.Equal(t, *txRefreshed, *tx)
			} else {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.expectedErrContains)
			}

			DeleteAllTransactionFixtures(t, ctx, dbConnectionPool)
		})
	}
}

func Test_TransactionModel_Unlock(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	transactionModel := TransactionModel{DBConnectionPool: dbConnectionPool}

	const currentLedger int32 = 10

	testCases := []struct {
		name                     string
		initialLockedAt          sql.NullTime
		initialSyncedAt          sql.NullTime
		initialStatus            TransactionStatus
		initialLockedUntilLedger sql.NullInt32
	}{
		{
			name:          "🎉 successfully locks transaction without any previous lock",
			initialStatus: TransactionStatusPending,
		},
		{
			name:                     "🎉 successfully locks transaction with lock expired",
			initialStatus:            TransactionStatusPending,
			initialLockedAt:          sql.NullTime{Time: time.Now().Add(-1 * time.Hour), Valid: true},
			initialLockedUntilLedger: sql.NullInt32{Int32: currentLedger - 1, Valid: true},
		},
		{
			name:                     "🎉 successfully unlocks locked transaction",
			initialStatus:            TransactionStatusPending,
			initialLockedAt:          sql.NullTime{Time: time.Now().Add(-1 * time.Hour), Valid: true},
			initialLockedUntilLedger: sql.NullInt32{Int32: currentLedger, Valid: true},
		},
		{
			name:          "🎉 successfully unlocks transaction with status is SUCCESS",
			initialStatus: TransactionStatusSuccess,
		},
		{
			name:          "🎉 successfully unlocks transaction with status is ERROR",
			initialStatus: TransactionStatusError,
		},
		{
			name:            "🎉 successfully unlocks transaction with siced_at not empty",
			initialStatus:   TransactionStatusPending,
			initialSyncedAt: sql.NullTime{Time: time.Now().Add(-1 * time.Hour), Valid: true},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tx := CreateTransactionFixtureNew(t, ctx, dbConnectionPool, TransactionFixture{
				ExternalID:         uuid.NewString(),
				TransactionType:    TransactionTypePayment,
				AssetCode:          "USDC",
				AssetIssuer:        "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
				DestinationAddress: "GCBIRB7Q5T53H4L6P5QSI3O6LPD5MBWGM5GHE7A5NY4XT5OT4VCOEZFX",
				Status:             tc.initialStatus,
				Amount:             1,
				TenantID:           uuid.NewString(),
			})
			q := `UPDATE submitter_transactions SET locked_at = $1, locked_until_ledger_number = $2, synced_at = $3, status = $4 WHERE id = $5`
			_, err := dbConnectionPool.ExecContext(ctx, q, tc.initialLockedAt, tc.initialLockedUntilLedger, tc.initialSyncedAt, tc.initialStatus, tx.ID)
			require.NoError(t, err)

			tx, err = transactionModel.Unlock(ctx, dbConnectionPool, tx.ID)
			require.NoError(t, err)

			tx, err = transactionModel.Get(ctx, tx.ID)
			require.NoError(t, err)
			assert.Nil(t, tx.LockedAt)
			assert.False(t, tx.LockedUntilLedgerNumber.Valid)

			var txRefreshed *Transaction
			txRefreshed, err = transactionModel.Get(ctx, tx.ID)
			require.NoError(t, err)
			require.Equal(t, *txRefreshed, *tx)

			DeleteAllTransactionFixtures(t, ctx, dbConnectionPool)
		})
	}
}

func Test_TransactionModel_PrepareTransactionForReprocessing(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	transactionModel := NewTransactionModel(dbConnectionPool)

	testCases := []struct {
		name      string
		status    TransactionStatus
		synchedAt sql.NullTime
		wantError error
	}{
		{
			name:      "cannot mark for reporcessing if the status is SUCCESS",
			status:    TransactionStatusSuccess,
			wantError: ErrRecordNotFound,
		},
		{
			name:      "cannot mark for reporcessing if the status is ERROR",
			status:    TransactionStatusError,
			wantError: ErrRecordNotFound,
		},
		{
			name:      "cannot mark for reporcessing if synced_at is not empty",
			status:    TransactionStatusProcessing,
			synchedAt: sql.NullTime{Time: time.Now().Add(-1 * time.Hour), Valid: true},
			wantError: ErrRecordNotFound,
		},
		{
			name:   "🎉 successfully mark as processing if tx is PENDING and not synced transaction",
			status: TransactionStatusPending,
		},
		{
			name:   "🎉 successfully mark as processing if tx is PROCESSING and not synced transaction",
			status: TransactionStatusProcessing,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer DeleteAllTransactionFixtures(t, ctx, dbConnectionPool)
			const lockedUntilLedger = 2

			// create and prepare the transaction:
			tx := CreateTransactionFixtureNew(t, ctx, dbConnectionPool, TransactionFixture{
				ExternalID:         uuid.NewString(),
				TransactionType:    TransactionTypePayment,
				AssetCode:          "USDC",
				AssetIssuer:        "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
				DestinationAddress: "GCBIRB7Q5T53H4L6P5QSI3O6LPD5MBWGM5GHE7A5NY4XT5OT4VCOEZFX",
				Status:             tc.status,
				Amount:             1,
				TenantID:           uuid.NewString(),
			})
			q := `UPDATE submitter_transactions SET status = $1, synced_at = $2, locked_at = NOW(), locked_until_ledger_number=$3 WHERE id = $4`
			_, err = dbConnectionPool.ExecContext(ctx, q, tc.status, tc.synchedAt, lockedUntilLedger, tx.ID)
			require.NoError(t, err)

			// mark the transaction for reprocessing:
			updatedTx, err := transactionModel.PrepareTransactionForReprocessing(ctx, dbConnectionPool, tx.ID)

			// check the result:
			if tc.wantError != nil {
				require.Error(t, err)
				assert.Equal(t, tc.wantError, err)
				assert.Nil(t, updatedTx)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.status, updatedTx.Status)
				assert.Nil(t, updatedTx.SyncedAt)

				// Check if only the expected fields were updated:
				assert.Nil(t, updatedTx.LockedAt)
				assert.False(t, updatedTx.LockedUntilLedgerNumber.Valid)
				assert.False(t, updatedTx.StellarTransactionHash.Valid)
				assert.False(t, updatedTx.XDRSent.Valid)
				assert.False(t, updatedTx.XDRReceived.Valid)

				// Check if the returned transaction is exactly the same as a fresh one from the DB:
				refreshedTx, err := transactionModel.Get(ctx, tx.ID)
				require.NoError(t, err)
				require.Equal(t, refreshedTx, updatedTx)
			}
		})
	}
}
