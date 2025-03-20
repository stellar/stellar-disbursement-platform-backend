package paymentdispatchers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

func Test_MemoResolver_GetMemo(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	tnt := tenant.Tenant{
		ID:      "tenant-id",
		BaseURL: utils.Ptr("https://example.com"),
	}

	testCases := []struct {
		name            string
		getCtxFn        func(t *testing.T) context.Context
		receiverWallet  data.ReceiverWallet
		orgMemoEnabled  bool
		expectedMemo    schema.Memo
		wantErrContains string
	}{
		{
			name: "🟢 return receiver wallet memo when present (missing memo type)",
			getCtxFn: func(t *testing.T) context.Context {
				return context.Background()
			},
			receiverWallet: data.ReceiverWallet{StellarMemo: "1234567890"},
			expectedMemo: schema.Memo{
				Value: "1234567890",
				Type:  schema.MemoTypeID,
			},
			wantErrContains: "",
		},
		{
			name: "🟢 return receiver wallet memo when present (MEMO_ID)",
			getCtxFn: func(t *testing.T) context.Context {
				return context.Background()
			},
			receiverWallet: data.ReceiverWallet{StellarMemo: "1234567890", StellarMemoType: schema.MemoTypeID},
			expectedMemo: schema.Memo{
				Value: "1234567890",
				Type:  schema.MemoTypeID,
			},
			wantErrContains: "",
		},
		{
			name: "🟢 return receiver wallet memo when present (MEMO_TEXT)",
			getCtxFn: func(t *testing.T) context.Context {
				return context.Background()
			},
			receiverWallet: data.ReceiverWallet{StellarMemo: "memo-text-1", StellarMemoType: schema.MemoTypeText},
			expectedMemo: schema.Memo{
				Value: "memo-text-1",
				Type:  schema.MemoTypeText,
			},
			wantErrContains: "",
		},
		{
			name: "🟢 return receiver wallet memo when present (MEMO_HASH)",
			getCtxFn: func(t *testing.T) context.Context {
				return context.Background()
			},
			receiverWallet: data.ReceiverWallet{StellarMemo: "12f37f82eb6708daa0ac372a1a67a0f33efa6a9cd213ed430517e45fefb51577", StellarMemoType: schema.MemoTypeHash},
			expectedMemo: schema.Memo{
				Value: "12f37f82eb6708daa0ac372a1a67a0f33efa6a9cd213ed430517e45fefb51577",
				Type:  schema.MemoTypeHash,
			},
			wantErrContains: "",
		},
		{
			name: "🟢 return receiver wallet memo when present (MEMO_RETURN)",
			getCtxFn: func(t *testing.T) context.Context {
				return context.Background()
			},
			receiverWallet: data.ReceiverWallet{StellarMemo: "12f37f82eb6708daa0ac372a1a67a0f33efa6a9cd213ed430517e45fefb51577", StellarMemoType: schema.MemoTypeReturn},
			expectedMemo: schema.Memo{
				Value: "12f37f82eb6708daa0ac372a1a67a0f33efa6a9cd213ed430517e45fefb51577",
				Type:  schema.MemoTypeReturn,
			},
			wantErrContains: "",
		},
		{
			name: "🟢 return nil when memo is not enabled",
			getCtxFn: func(t *testing.T) context.Context {
				return context.Background()
			},
			receiverWallet:  data.ReceiverWallet{},
			orgMemoEnabled:  false,
			expectedMemo:    schema.Memo{},
			wantErrContains: "",
		},
		{
			name: "🔴 error when tenant is not in the context",
			getCtxFn: func(t *testing.T) context.Context {
				return context.Background()
			},
			receiverWallet:  data.ReceiverWallet{},
			orgMemoEnabled:  true,
			expectedMemo:    schema.Memo{},
			wantErrContains: "getting tenant: tenant not found in context",
		},
		{
			name: "🟢 return tenant memo when enabled",
			getCtxFn: func(t *testing.T) context.Context {
				ctx := context.Background()
				return tenant.SaveTenantInContext(ctx, &tnt)
			},
			receiverWallet: data.ReceiverWallet{},
			orgMemoEnabled: true,
			expectedMemo: schema.Memo{
				Value: "sdp-100680ad546c",
				Type:  schema.MemoTypeText,
			},
			wantErrContains: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			models, err := data.NewModels(dbConnectionPool)
			require.NoError(t, err)
			memoResolver := MemoResolver{Organizations: models.Organizations}

			ctx := tc.getCtxFn(t)
			err = models.Organizations.Update(ctx, &data.OrganizationUpdate{IsMemoTracingEnabled: &tc.orgMemoEnabled})
			require.NoError(t, err)

			memo, err := memoResolver.GetMemo(ctx, tc.receiverWallet)

			if tc.wantErrContains != "" {
				assert.ErrorContains(t, err, tc.wantErrContains)
				assert.Empty(t, memo)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedMemo, memo)
			}
		})
	}
}

func Test_generateHashFromBaseURL(t *testing.T) {
	testCases := []struct {
		baseURL      string
		expectedHash string
	}{
		{
			baseURL:      "https://example.com",
			expectedHash: "sdp-100680ad546c",
		},
		{
			baseURL:      "   https://example.com   ",
			expectedHash: "sdp-100680ad546c",
		},
		{
			baseURL:      "https://example.com/",
			expectedHash: "sdp-100680ad546c",
		},
		{
			baseURL:      "  https://example.com/  ",
			expectedHash: "sdp-100680ad546c",
		},
		{
			baseURL:      "https://example.com/path?query=param",
			expectedHash: "sdp-58821f845568",
		},
		{
			baseURL:      "https://test.com",
			expectedHash: "sdp-396936bd0bf0",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.baseURL, func(t *testing.T) {
			hash := GenerateHashFromBaseURL(tc.baseURL)
			assert.Equal(t, tc.expectedHash, hash)
		})
	}
}
