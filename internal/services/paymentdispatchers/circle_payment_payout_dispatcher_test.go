package paymentdispatchers

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/circle"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

func Test_CirclePaymentPayoutDispatcher_ensureRecipientIsReady_success(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	// Fixtures
	disbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{})
	receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, disbursement.Wallet.ID, data.RegisteredReceiversWalletStatus)

	initialTime := time.Now().Add(-time.Hour)
	recipientInsertTemplate := data.CircleRecipient{
		ReceiverWalletID:  receiverWallet.ID,
		StellarAddress:    receiverWallet.StellarAddress,
		StellarMemo:       receiverWallet.StellarMemo,
		CircleRecipientID: "circle-recipient-id",
		IdempotencyKey:    "idepotency-key",
		CreatedAt:         initialTime,
		UpdatedAt:         initialTime,
		SyncAttempts:      0,
		LastSyncAttemptAt: time.Time{},
		ResponseBody:      nil,
	}
	type TestCase struct {
		name                       string
		populateInitialRecipientFn func(t *testing.T) *data.CircleRecipient
		prepareMocksFn             func(t *testing.T, mCircleService *circle.MockService)
		assertRecipients           func(t *testing.T, initialRecipient, finalRecipient *data.CircleRecipient)
	}
	testCases := []TestCase{
		{
			name: "recipient already exists [status=active]",
			populateInitialRecipientFn: func(t *testing.T) *data.CircleRecipient {
				recipientInsert := recipientInsertTemplate
				recipientInsert.Status = data.CircleRecipientStatusActive
				recipientInsert.SyncAttempts = 1
				recipientInsert.LastSyncAttemptAt = initialTime
				recipientInsert.ResponseBody = []byte(`{"foo": "bar"}`)
				return data.CreateCircleRecipientFixture(t, ctx, dbConnectionPool, recipientInsert)
			},
			assertRecipients: func(t *testing.T, initialRecipient, finalRecipient *data.CircleRecipient) {
				assert.Equal(t, initialRecipient, finalRecipient)
			},
		},
		{
			name: "recipient already exists [status=pending]",
			populateInitialRecipientFn: func(t *testing.T) *data.CircleRecipient {
				recipientInsert := recipientInsertTemplate
				recipientInsert.Status = data.CircleRecipientStatusPending
				recipientInsert.SyncAttempts = 1
				recipientInsert.LastSyncAttemptAt = initialTime
				recipientInsert.ResponseBody = []byte(`{"error": "test"}`)
				return data.CreateCircleRecipientFixture(t, ctx, dbConnectionPool, recipientInsert)
			},
			prepareMocksFn: func(t *testing.T, mCircleService *circle.MockService) {
				mCircleService.
					On("PostRecipient", ctx, mock.Anything).
					Run(func(args mock.Arguments) {
						recipientRequest, ok := args.Get(1).(circle.RecipientRequest)
						require.True(t, ok)
						assert.Equal(t, recipientInsertTemplate.IdempotencyKey, recipientRequest.IdempotencyKey)
						assert.Equal(t, receiverWallet.StellarAddress, recipientRequest.Address)
						assert.Equal(t, circle.StellarChainCode, recipientRequest.Chain)
						assert.Equal(t, receiverWallet.Receiver.PhoneNumber, recipientRequest.Metadata.Nickname)
						assert.Equal(t, receiverWallet.Receiver.Email, recipientRequest.Metadata.Email)
					}).
					Return(&circle.Recipient{ID: "new-circle-recipient-id", Status: "active"}, nil).
					Once()
			},
			assertRecipients: func(t *testing.T, initialRecipient, finalRecipient *data.CircleRecipient) {
				assert.Equal(t, data.CircleRecipientStatusPending, initialRecipient.Status)
				assert.Equal(t, data.CircleRecipientStatusActive, finalRecipient.Status)
				assert.Equal(t, initialRecipient.SyncAttempts+1, finalRecipient.SyncAttempts)
				assert.Greater(t, finalRecipient.LastSyncAttemptAt.Unix(), initialRecipient.LastSyncAttemptAt.Unix())
				assert.NotEqual(t, initialRecipient.ResponseBody, finalRecipient.ResponseBody)
			},
		},
		{
			name: "recipient does not exist in the DB",
			populateInitialRecipientFn: func(t *testing.T) *data.CircleRecipient {
				return nil
			},
			prepareMocksFn: func(t *testing.T, mCircleService *circle.MockService) {
				mCircleService.
					On("PostRecipient", ctx, mock.Anything).
					Run(func(args mock.Arguments) {
						recipientRequest, ok := args.Get(1).(circle.RecipientRequest)
						require.True(t, ok)
						assert.NotEqual(t, recipientInsertTemplate.IdempotencyKey, recipientRequest.IdempotencyKey)
						assert.Equal(t, receiverWallet.StellarAddress, recipientRequest.Address)
						assert.Equal(t, circle.StellarChainCode, recipientRequest.Chain)
						assert.Equal(t, receiverWallet.Receiver.PhoneNumber, recipientRequest.Metadata.Nickname)
						assert.Equal(t, receiverWallet.Receiver.Email, recipientRequest.Metadata.Email)
					}).
					Return(&circle.Recipient{ID: "new-circle-recipient-id", Status: "active"}, nil).
					Once()
			},
			assertRecipients: func(t *testing.T, initialRecipient, finalRecipient *data.CircleRecipient) {
				assert.Nil(t, initialRecipient)
				assert.Equal(t, data.CircleRecipientStatusActive, finalRecipient.Status)
				assert.Equal(t, 1, finalRecipient.SyncAttempts)
				assert.NotEmpty(t, finalRecipient.LastSyncAttemptAt)
			},
		},
	}

	for _, failedStatus := range []data.CircleRecipientStatus{data.CircleRecipientStatusInactive, data.CircleRecipientStatusDenied, data.CircleRecipientStatusFailed} {
		testCases = append(testCases, TestCase{
			name: fmt.Sprintf("recipient already exists [status=%s]", failedStatus),
			populateInitialRecipientFn: func(t *testing.T) *data.CircleRecipient {
				recipientInsert := recipientInsertTemplate
				recipientInsert.Status = failedStatus
				recipientInsert.SyncAttempts = 1
				recipientInsert.LastSyncAttemptAt = initialTime
				recipientInsert.ResponseBody = []byte(`{"error": "test"}`)
				return data.CreateCircleRecipientFixture(t, ctx, dbConnectionPool, recipientInsert)
			},
			prepareMocksFn: func(t *testing.T, mCircleService *circle.MockService) {
				mCircleService.
					On("PostRecipient", ctx, mock.Anything).
					Run(func(args mock.Arguments) {
						recipientRequest, ok := args.Get(1).(circle.RecipientRequest)
						require.True(t, ok)
						assert.NotEqual(t, recipientInsertTemplate.IdempotencyKey, recipientRequest.IdempotencyKey)
						assert.Equal(t, receiverWallet.StellarAddress, recipientRequest.Address)
						assert.Equal(t, circle.StellarChainCode, recipientRequest.Chain)
						assert.Equal(t, receiverWallet.Receiver.PhoneNumber, recipientRequest.Metadata.Nickname)
						assert.Equal(t, receiverWallet.Receiver.Email, recipientRequest.Metadata.Email)
					}).
					Return(&circle.Recipient{ID: "new-circle-recipient-id", Status: "active"}, nil).
					Once()
			},
			assertRecipients: func(t *testing.T, initialRecipient, finalRecipient *data.CircleRecipient) {
				assert.Equal(t, failedStatus, initialRecipient.Status)
				assert.Equal(t, data.CircleRecipientStatusActive, finalRecipient.Status)
				assert.Equal(t, initialRecipient.SyncAttempts+1, finalRecipient.SyncAttempts)
				assert.Greater(t, finalRecipient.LastSyncAttemptAt.Unix(), initialRecipient.LastSyncAttemptAt.Unix())
			},
		})
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer data.DeleteAllCircleRecipientsFixtures(t, ctx, dbConnectionPool)
			initialRecipient := tc.populateInitialRecipientFn(t)

			mDistAccountResolver := mocks.NewMockDistributionAccountResolver(t)
			mCircleService := circle.NewMockService(t)
			if tc.prepareMocksFn != nil {
				tc.prepareMocksFn(t, mCircleService)
			}

			dispatcher := NewCirclePaymentPayoutDispatcher(models, mCircleService, mDistAccountResolver)

			finalRecipient, err := dispatcher.ensureRecipientIsReady(ctx, *receiverWallet)
			require.NoError(t, err)
			tc.assertRecipients(t, initialRecipient, finalRecipient)
		})
	}
}

func Test_CirclePaymentPayoutDispatcher_ensureRecipientIsReady_success_assertMemos(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	tenantID := "tenant-id"
	tnt := schema.Tenant{
		ID:      tenantID,
		BaseURL: utils.Ptr("https://example.com"),
	}

	ctx := context.Background()
	ctx = sdpcontext.SetTenantInContext(ctx, &tnt)
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	// Wallets
	walletA := data.CreateWalletFixture(t, ctx, dbConnectionPool, "walletA", "https://www.a.com", "www.a.com", "a://")
	walletB := data.CreateWalletFixture(t, ctx, dbConnectionPool, "walletB", "https://www.b.com", "www.b.com", "b://")
	// Disbursements
	disbursementA := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{Wallet: walletA})
	disbursementB := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{Wallet: walletB})
	// Receiver
	receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	// Receiver Wallets
	rwWithMemo := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, disbursementA.Wallet.ID, data.RegisteredReceiversWalletStatus)
	rwWithoutMemo := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, disbursementB.Wallet.ID, data.RegisteredReceiversWalletStatus)
	err = models.ReceiverWallet.Update(ctx, rwWithoutMemo.ID, data.ReceiverWalletUpdate{
		StellarMemo:     utils.Ptr(""),
		StellarMemoType: utils.Ptr(schema.MemoType("")),
	}, dbConnectionPool)
	require.NoError(t, err)
	rwWithoutMemo.StellarMemo = ""
	rwWithoutMemo.StellarMemoType = schema.MemoType("")

	// Successful test cases
	testCases := []struct {
		name                 string
		isMemoTracingEnabled bool
		receiverWallet       *data.ReceiverWallet
		assertMemo           func(t *testing.T, rw *data.ReceiverWallet, recipientRequest circle.RecipientRequest)
	}{
		{
			name:           "recipient created with ReceiverWallet memo",
			receiverWallet: rwWithMemo,
			assertMemo: func(t *testing.T, rw *data.ReceiverWallet, recipientRequest circle.RecipientRequest) {
				assert.Equal(t, rw.StellarMemo, recipientRequest.AddressTag)
				assert.NotEmpty(t, recipientRequest.AddressTag)
			},
		},
		{
			name:                 "recipient created with no memo",
			isMemoTracingEnabled: false,
			receiverWallet:       rwWithoutMemo,
			assertMemo: func(t *testing.T, rw *data.ReceiverWallet, recipientRequest circle.RecipientRequest) {
				assert.Empty(t, recipientRequest.AddressTag)
			},
		},
		{
			name:                 "recipient created with Organization memo",
			isMemoTracingEnabled: true,
			receiverWallet:       rwWithoutMemo,
			assertMemo: func(t *testing.T, rw *data.ReceiverWallet, recipientRequest circle.RecipientRequest) {
				assert.Equal(t, tenant.GenerateHashFromBaseURL(*tnt.BaseURL), recipientRequest.AddressTag)
				assert.NotEmpty(t, recipientRequest.AddressTag)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err = models.Organizations.Update(ctx, &data.OrganizationUpdate{IsMemoTracingEnabled: utils.Ptr(tc.isMemoTracingEnabled)})
			require.NoError(t, err)
			rw := tc.receiverWallet

			defer data.DeleteAllCircleRecipientsFixtures(t, ctx, dbConnectionPool)

			mDistAccountResolver := mocks.NewMockDistributionAccountResolver(t)
			mCircleService := circle.NewMockService(t)
			mCircleService.
				On("PostRecipient", ctx, mock.Anything).
				Run(func(args mock.Arguments) {
					recipientRequest, ok := args.Get(1).(circle.RecipientRequest)
					require.True(t, ok)
					assert.NotEmpty(t, recipientRequest.IdempotencyKey)
					assert.Equal(t, rw.StellarAddress, recipientRequest.Address)
					assert.Equal(t, circle.StellarChainCode, recipientRequest.Chain)
					assert.Equal(t, rw.Receiver.PhoneNumber, recipientRequest.Metadata.Nickname)
					assert.Equal(t, rw.Receiver.Email, recipientRequest.Metadata.Email)

					tc.assertMemo(t, rw, recipientRequest)
				}).
				Return(&circle.Recipient{ID: "new-circle-recipient-id", Status: "active"}, nil).
				Once()

			dispatcher := NewCirclePaymentPayoutDispatcher(models, mCircleService, mDistAccountResolver)

			finalRecipient, err := dispatcher.ensureRecipientIsReady(ctx, *tc.receiverWallet)
			require.NoError(t, err)
			assert.Equal(t, data.CircleRecipientStatusActive, finalRecipient.Status)
			assert.Equal(t, 1, finalRecipient.SyncAttempts)
			assert.NotEmpty(t, finalRecipient.LastSyncAttemptAt)
		})
	}
}

func Test_CirclePaymentPayoutDispatcher_ensureRecipientIsReady_success_CircleRecipientRecreatedWhenMemoChanges(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	// Disbursement
	disbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{})
	// Receiver
	receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	// Receiver Wallets
	rw := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, disbursement.Wallet.ID, data.RegisteredReceiversWalletStatus)
	initialMemo := rw.StellarMemo
	require.NotEmpty(t, initialMemo)

	defer data.DeleteAllCircleRecipientsFixtures(t, ctx, dbConnectionPool)

	// The first Circle recipient should be crated with the initial ReceiverWallet memo
	mDistAccountResolver := mocks.NewMockDistributionAccountResolver(t)
	mCircleService := circle.NewMockService(t)
	mCircleService.
		On("PostRecipient", ctx, mock.Anything).
		Run(func(args mock.Arguments) {
			recipientRequest, ok := args.Get(1).(circle.RecipientRequest)
			require.True(t, ok)
			assert.NotEmpty(t, recipientRequest.IdempotencyKey)
			assert.Equal(t, rw.StellarAddress, recipientRequest.Address)
			assert.Equal(t, circle.StellarChainCode, recipientRequest.Chain)
			assert.Equal(t, rw.Receiver.PhoneNumber, recipientRequest.Metadata.Nickname)
			assert.Equal(t, rw.Receiver.Email, recipientRequest.Metadata.Email)

			assert.Equal(t, initialMemo, recipientRequest.AddressTag)
			assert.NotEmpty(t, recipientRequest.AddressTag)
		}).
		Return(&circle.Recipient{ID: "new-circle-recipient-id", Status: "active"}, nil).
		Once()

	dispatcher := NewCirclePaymentPayoutDispatcher(models, mCircleService, mDistAccountResolver)

	initialRecipient, err := dispatcher.ensureRecipientIsReady(ctx, *rw)
	require.NoError(t, err)
	assert.Equal(t, data.CircleRecipientStatusActive, initialRecipient.Status)
	assert.Equal(t, 1, initialRecipient.SyncAttempts)
	assert.NotEmpty(t, initialRecipient.LastSyncAttemptAt)

	// Update the ReceiverWallet memo
	finalMemo := "my-final-memo"
	err = models.ReceiverWallet.Update(ctx, rw.ID, data.ReceiverWalletUpdate{
		StellarMemo:     &finalMemo,
		StellarMemoType: utils.Ptr(schema.MemoTypeText),
	}, dbConnectionPool)
	require.NoError(t, err)
	rw.StellarMemo = finalMemo
	rw.StellarMemoType = schema.MemoTypeText

	// The final version of the Circle recipient should have its memo (AddressTag) updated to the final ReceiverWallet memo
	mCircleService.
		On("PostRecipient", ctx, mock.Anything).
		Run(func(args mock.Arguments) {
			recipientRequest, ok := args.Get(1).(circle.RecipientRequest)
			require.True(t, ok)
			assert.NotEmpty(t, recipientRequest.IdempotencyKey)
			assert.Equal(t, rw.StellarAddress, recipientRequest.Address)
			assert.Equal(t, circle.StellarChainCode, recipientRequest.Chain)
			assert.Equal(t, rw.Receiver.PhoneNumber, recipientRequest.Metadata.Nickname)
			assert.Equal(t, rw.Receiver.Email, recipientRequest.Metadata.Email)

			assert.Equal(t, finalMemo, recipientRequest.AddressTag)
			assert.NotEmpty(t, recipientRequest.AddressTag)
		}).
		Return(&circle.Recipient{ID: "new-circle-recipient-id", Status: "active"}, nil).
		Once()

	finalRecipient, err := dispatcher.ensureRecipientIsReady(ctx, *rw)
	require.NoError(t, err)
	assert.Equal(t, data.CircleRecipientStatusActive, finalRecipient.Status)
	assert.Equal(t, 2, finalRecipient.SyncAttempts)
	assert.NotEmpty(t, finalRecipient.LastSyncAttemptAt)
	assert.NotEqual(t, initialRecipient.IdempotencyKey, finalRecipient.IdempotencyKey)
	assert.Equal(t, finalMemo, finalRecipient.StellarMemo)
}

func Test_CirclePaymentPayoutDispatcher_ensureRecipientIsReady_failure(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	// Fixtures
	disbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{})
	receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, disbursement.Wallet.ID, data.RegisteredReceiversWalletStatus)

	now := time.Now()
	initialTime := now.Add(-time.Hour)
	recipientInsertTemplate := data.CircleRecipient{
		ReceiverWalletID:  receiverWallet.ID,
		CircleRecipientID: "circle-recipient-id",
		IdempotencyKey:    "idepotency-key",
		CreatedAt:         initialTime,
		UpdatedAt:         initialTime,
		SyncAttempts:      0,
		LastSyncAttemptAt: initialTime,
	}
	type TestCase struct {
		name                       string
		populateInitialRecipientFn func(t *testing.T) *data.CircleRecipient
		prepareMocksFn             func(t *testing.T, mCircleService *circle.MockService)
		assertRecipients           func(t *testing.T, initialRecipient, finalRecipient data.CircleRecipient)
		wantErrContains            string
	}
	testCases := []TestCase{
		{
			name: "PostRecipient returns an error [status=pending]",
			populateInitialRecipientFn: func(t *testing.T) *data.CircleRecipient {
				recipientInsert := recipientInsertTemplate
				recipientInsert.Status = data.CircleRecipientStatusPending
				recipientInsert.ResponseBody = []byte(`{"foo": "bar"}`)
				return data.CreateCircleRecipientFixture(t, ctx, dbConnectionPool, recipientInsert)
			},
			prepareMocksFn: func(t *testing.T, mCircleService *circle.MockService) {
				mCircleService.
					On("PostRecipient", ctx, mock.Anything).
					Run(func(args mock.Arguments) {
						recipientRequest, ok := args.Get(1).(circle.RecipientRequest)
						require.True(t, ok)
						assert.Equal(t, recipientInsertTemplate.IdempotencyKey, recipientRequest.IdempotencyKey)
						assert.Equal(t, receiverWallet.StellarAddress, recipientRequest.Address)
						assert.Equal(t, circle.StellarChainCode, recipientRequest.Chain)
						assert.Equal(t, receiverWallet.Receiver.PhoneNumber, recipientRequest.Metadata.Nickname)
						assert.Equal(t, receiverWallet.Receiver.Email, recipientRequest.Metadata.Email)
					}).
					Return(nil, errors.New("got 400 from vendor's API")).
					Once()
			},
			assertRecipients: func(t *testing.T, initialRecipient, finalRecipient data.CircleRecipient) {
				assert.Equal(t, initialRecipient.SyncAttempts+1, finalRecipient.SyncAttempts)
				assert.Greater(t, finalRecipient.LastSyncAttemptAt.Unix(), initialRecipient.LastSyncAttemptAt.Unix())
				assert.JSONEq(t, `{"error": "got 400 from vendor's API"}`, string(finalRecipient.ResponseBody))
			},
			wantErrContains: "creating Circle recipient: got 400 from vendor's API",
		},
	}

	for _, failedStatus := range []data.CircleRecipientStatus{
		data.CircleRecipientStatusInactive,
		data.CircleRecipientStatusDenied,
		data.CircleRecipientStatusFailed,
		"",
	} {
		testCases = append(testCases, TestCase{
			name: fmt.Sprintf("recipient has reached maxCircleRecipientCreationAttempts [status=%s]", failedStatus),
			populateInitialRecipientFn: func(t *testing.T) *data.CircleRecipient {
				recipientInsert := recipientInsertTemplate
				recipientInsert.Status = failedStatus
				recipientInsert.ResponseBody = []byte(`{"error": "test"}`)
				recipientInsert.SyncAttempts = maxCircleRecipientCreationAttempts
				recipientInsert.LastSyncAttemptAt = now
				return data.CreateCircleRecipientFixture(t, ctx, dbConnectionPool, recipientInsert)
			},
			assertRecipients: func(t *testing.T, initialRecipient, finalRecipient data.CircleRecipient) {
				assert.Equal(t, initialRecipient, finalRecipient)
			},
			wantErrContains: ErrCircleRecipientCreationFailedTooManyTimes.Error(),
		})
	}

	for _, failedStatus := range []data.CircleRecipientStatus{
		data.CircleRecipientStatusInactive,
		data.CircleRecipientStatusDenied,
		data.CircleRecipientStatusFailed,
		"",
	} {
		testCases = append(testCases, TestCase{
			name: fmt.Sprintf("recover failure if recipient can still retry [status=%s]", failedStatus),
			populateInitialRecipientFn: func(t *testing.T) *data.CircleRecipient {
				recipientInsert := recipientInsertTemplate
				recipientInsert.Status = failedStatus
				recipientInsert.ResponseBody = []byte(`{"error": "test"}`)
				recipientInsert.SyncAttempts = 1
				return data.CreateCircleRecipientFixture(t, ctx, dbConnectionPool, recipientInsert)
			},
			prepareMocksFn: func(t *testing.T, mCircleService *circle.MockService) {
				mCircleService.
					On("PostRecipient", ctx, mock.Anything).
					Run(func(args mock.Arguments) {
						recipientRequest, ok := args.Get(1).(circle.RecipientRequest)
						require.True(t, ok)
						assert.Equal(t, receiverWallet.StellarAddress, recipientRequest.Address)
						assert.Equal(t, circle.StellarChainCode, recipientRequest.Chain)
						assert.Equal(t, receiverWallet.Receiver.PhoneNumber, recipientRequest.Metadata.Nickname)
						assert.Equal(t, receiverWallet.Receiver.Email, recipientRequest.Metadata.Email)
					}).
					Return(&circle.Recipient{ID: "new-circle-recipient-id", Status: "active"}, nil).
					Once()
			},
			assertRecipients: func(t *testing.T, initialRecipient, finalRecipient data.CircleRecipient) {
				if failedStatus == "" {
					assert.Equal(t, initialRecipient.IdempotencyKey, finalRecipient.IdempotencyKey)
				} else {
					assert.NotEqual(t, initialRecipient.IdempotencyKey, finalRecipient.IdempotencyKey)
				}
				assert.Equal(t, initialRecipient.SyncAttempts+1, finalRecipient.SyncAttempts)
				assert.Greater(t, finalRecipient.LastSyncAttemptAt.Unix(), initialRecipient.LastSyncAttemptAt.Unix())
				assert.Equal(t, "new-circle-recipient-id", finalRecipient.CircleRecipientID)
				assert.Equal(t, data.CircleRecipientStatusActive, finalRecipient.Status)
			},
		})
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer data.DeleteAllCircleRecipientsFixtures(t, ctx, dbConnectionPool)
			initialRecipient := tc.populateInitialRecipientFn(t)

			mDistAccountResolver := mocks.NewMockDistributionAccountResolver(t)
			mCircleService := circle.NewMockService(t)
			if tc.prepareMocksFn != nil {
				tc.prepareMocksFn(t, mCircleService)
			}

			dispatcher := NewCirclePaymentPayoutDispatcher(models, mCircleService, mDistAccountResolver)

			finalRecipient, err := dispatcher.ensureRecipientIsReady(ctx, *receiverWallet)

			if tc.wantErrContains != "" {
				assert.ErrorContains(t, err, tc.wantErrContains)
				assert.Nil(t, finalRecipient)
			}

			finalRecipient, err = models.CircleRecipient.GetByReceiverWalletID(ctx, receiverWallet.ID)
			require.NoError(t, err)
			tc.assertRecipients(t, *initialRecipient, *finalRecipient)
		})
	}
}

func Test_CirclePaymentPayoutDispatcher_ensureRecipientIsReadyWithRetry(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	// Fixtures
	disbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{})
	receiver1 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver1.ID, disbursement.Wallet.ID, data.RegisteredReceiversWalletStatus)

	now := time.Now()
	initialTime := now.Add(-time.Hour)
	recipientInsertTemplate := data.CircleRecipient{
		ReceiverWalletID:  receiverWallet.ID,
		CircleRecipientID: "circle-recipient-id",
		IdempotencyKey:    "idepotency-key",
		CreatedAt:         initialTime,
		UpdatedAt:         initialTime,
		SyncAttempts:      0,
		LastSyncAttemptAt: initialTime,
	}
	type TestCase struct {
		name                       string
		populateInitialRecipientFn func(t *testing.T) *data.CircleRecipient
		prepareMocksFn             func(t *testing.T, mCircleService *circle.MockService)
		assertRecipients           func(t *testing.T, initialRecipient, finalRecipient data.CircleRecipient)
		wantErrContains            string
	}
	testCases := []TestCase{
		{
			name: "tries 5 times (error returned)",
			populateInitialRecipientFn: func(t *testing.T) *data.CircleRecipient {
				recipientInsert := recipientInsertTemplate
				recipientInsert.Status = data.CircleRecipientStatusPending
				recipientInsert.ResponseBody = []byte(`{"foo": "bar"}`)
				return data.CreateCircleRecipientFixture(t, ctx, dbConnectionPool, recipientInsert)
			},
			prepareMocksFn: func(t *testing.T, mCircleService *circle.MockService) {
				mCircleService.
					On("PostRecipient", ctx, mock.Anything).
					Run(func(args mock.Arguments) {
						recipientRequest, ok := args.Get(1).(circle.RecipientRequest)
						require.True(t, ok)
						assert.Equal(t, recipientInsertTemplate.IdempotencyKey, recipientRequest.IdempotencyKey)
						assert.Equal(t, receiverWallet.StellarAddress, recipientRequest.Address)
						assert.Equal(t, circle.StellarChainCode, recipientRequest.Chain)
						assert.Equal(t, receiverWallet.Receiver.PhoneNumber, recipientRequest.Metadata.Nickname)
						assert.Equal(t, receiverWallet.Receiver.Email, recipientRequest.Metadata.Email)
					}).
					Return(nil, errors.New("got 400 from vendor's API")).
					Times(maxCircleRecipientCreationAttempts)
			},
			assertRecipients: func(t *testing.T, initialRecipient, finalRecipient data.CircleRecipient) {
				assert.Equal(t, initialRecipient.SyncAttempts+maxCircleRecipientCreationAttempts, finalRecipient.SyncAttempts)
				assert.Greater(t, finalRecipient.LastSyncAttemptAt.Unix(), initialRecipient.LastSyncAttemptAt.Unix())
			},
			wantErrContains: "failed to ensure recipient is ready: All attempts fail:\n#1: submitting recipient to Circle: creating Circle recipient: got 400 from vendor's API",
		},
		{
			name: "gives up if maxCircleRecipientCreationAttempts is reached",
			populateInitialRecipientFn: func(t *testing.T) *data.CircleRecipient {
				recipientInsert := recipientInsertTemplate
				recipientInsert.Status = data.CircleRecipientStatusPending
				recipientInsert.ResponseBody = []byte(`{"foo": "bar"}`)
				recipientInsert.SyncAttempts = maxCircleRecipientCreationAttempts
				return data.CreateCircleRecipientFixture(t, ctx, dbConnectionPool, recipientInsert)
			},
			assertRecipients: func(t *testing.T, initialRecipient, finalRecipient data.CircleRecipient) {
				assert.Equal(t, initialRecipient.SyncAttempts, finalRecipient.SyncAttempts)
				assert.Equal(t, finalRecipient.LastSyncAttemptAt.Unix(), initialRecipient.LastSyncAttemptAt.Unix())
			},
			wantErrContains: ErrCircleRecipientCreationFailedTooManyTimes.Error(),
		},
	}

	for _, nonSuccessfulState := range []data.CircleRecipientStatus{data.CircleRecipientStatusInactive, data.CircleRecipientStatusDenied, data.CircleRecipientStatusFailed, data.CircleRecipientStatusPending} {
		testCases = append(testCases, TestCase{
			name: fmt.Sprintf("tries 5 times [status=%s]", nonSuccessfulState),
			populateInitialRecipientFn: func(t *testing.T) *data.CircleRecipient {
				recipientInsert := recipientInsertTemplate
				recipientInsert.Status = data.CircleRecipientStatusPending
				recipientInsert.ResponseBody = []byte(`{"foo": "bar"}`)
				return data.CreateCircleRecipientFixture(t, ctx, dbConnectionPool, recipientInsert)
			},
			prepareMocksFn: func(t *testing.T, mCircleService *circle.MockService) {
				mCircleService.
					On("PostRecipient", ctx, mock.Anything).
					Run(func(args mock.Arguments) {
						recipientRequest, ok := args.Get(1).(circle.RecipientRequest)
						require.True(t, ok)
						assert.Equal(t, receiverWallet.StellarAddress, recipientRequest.Address)
						assert.Equal(t, circle.StellarChainCode, recipientRequest.Chain)
						assert.Equal(t, receiverWallet.Receiver.PhoneNumber, recipientRequest.Metadata.Nickname)
						assert.Equal(t, receiverWallet.Receiver.Email, recipientRequest.Metadata.Email)
					}).
					Return(&circle.Recipient{ID: "recipient-id", Status: string(nonSuccessfulState)}, nil).
					Times(maxCircleRecipientCreationAttempts)
			},
			assertRecipients: func(t *testing.T, initialRecipient, finalRecipient data.CircleRecipient) {
				assert.Equal(t, nonSuccessfulState, finalRecipient.Status)
				assert.Equal(t, initialRecipient.SyncAttempts+maxCircleRecipientCreationAttempts, finalRecipient.SyncAttempts)
				assert.Greater(t, finalRecipient.LastSyncAttemptAt.Unix(), initialRecipient.LastSyncAttemptAt.Unix())
			},
		})
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer data.DeleteAllCircleRecipientsFixtures(t, ctx, dbConnectionPool)
			initialRecipient := tc.populateInitialRecipientFn(t)

			mDistAccountResolver := mocks.NewMockDistributionAccountResolver(t)
			mCircleService := circle.NewMockService(t)
			if tc.prepareMocksFn != nil {
				tc.prepareMocksFn(t, mCircleService)
			}

			dispatcher := NewCirclePaymentPayoutDispatcher(models, mCircleService, mDistAccountResolver)

			finalRecipient, err := dispatcher.ensureRecipientIsReadyWithRetry(ctx, *receiverWallet, 1*time.Millisecond)

			if tc.wantErrContains != "" {
				assert.ErrorContains(t, err, tc.wantErrContains)
				assert.Nil(t, finalRecipient)
			}

			finalRecipient, err = models.CircleRecipient.GetByReceiverWalletID(ctx, receiverWallet.ID)
			require.NoError(t, err)
			tc.assertRecipients(t, *initialRecipient, *finalRecipient)
		})
	}
}

func Test_CirclePaymentPayoutDispatcher_DispatchPayments(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	circleWalletID := "22322112"
	circlePayoutID := "dce3a913-9043-4d20-ba6c-fe27f630f2a0"

	tenantID := "tenant-id"

	// Disbursement
	disbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{})

	// Receivers
	receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})

	type TestCase struct {
		name            string
		wantErrContains []string
		fnSetup         func(*testing.T, *circle.MockService, *data.Payment, *data.CircleRecipient)
		fnAsserts       func(*testing.T, db.SQLExecuter, *data.Payment)
	}
	tests := []TestCase{
		{
			name: "🔴 if payment does not exist return error",
			fnSetup: func(*testing.T, *circle.MockService, *data.Payment, *data.CircleRecipient) {
				// By deleting all payments, the function will return an error
				data.DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
			},
			wantErrContains: []string{"payment with ID", "does not exist"},
		},
		{
			name: "🔴 if SendPayout fails return error",
			fnSetup: func(t *testing.T, m *circle.MockService, payment *data.Payment, circleRecipient *data.CircleRecipient) {
				transferRequest, setupErr := models.CircleTransferRequests.Insert(ctx, dbConnectionPool, payment.ID)
				require.NoError(t, setupErr)

				m.On("SendPayout", ctx, circle.PaymentRequest{
					APIType:          circle.APITypePayouts,
					SourceWalletID:   circleWalletID,
					RecipientID:      circleRecipient.CircleRecipientID,
					Amount:           payment.Amount,
					StellarAssetCode: payment.Asset.Code,
					IdempotencyKey:   transferRequest.IdempotencyKey,
				}).
					Return(nil, fmt.Errorf("error posting transfer to Circle")).
					Once()
			},
			fnAsserts: func(t *testing.T, sqlExecuter db.SQLExecuter, payment *data.Payment) {
				// Payment should be marked as failed
				payment, assertErr := models.Payment.Get(ctx, payment.ID, sqlExecuter)
				require.NoError(t, assertErr)
				assert.Equal(t, data.FailedPaymentStatus, payment.Status)
			},
		},
		{
			name: "🔴 if the payout is unexpectedly nil return an error",
			fnSetup: func(t *testing.T, m *circle.MockService, payment *data.Payment, circleRecipient *data.CircleRecipient) {
				m.
					On("SendPayout", ctx, mock.AnythingOfType("circle.PaymentRequest")).
					Return(nil, nil).
					Once()
			},
			wantErrContains: []string{"updating circle transfer request: payout cannot be nil"},
		},
		{
			name: "🔴 if the payout status is unsupported return an error",
			fnSetup: func(t *testing.T, m *circle.MockService, payment *data.Payment, circleRecipient *data.CircleRecipient) {
				m.
					On("SendPayout", ctx, mock.AnythingOfType("circle.PaymentRequest")).
					Return(&circle.Payout{ID: "payout_id", Status: "unsupported-status"}, nil).
					Once()
			},
			wantErrContains: []string{"invalid input value for enum circle_transfer_status"},
		},
		{
			name: "🟢 successful SendPayout",
			fnSetup: func(t *testing.T, m *circle.MockService, payment *data.Payment, circleRecipient *data.CircleRecipient) {
				m.
					On("SendPayout", ctx, mock.AnythingOfType("circle.PaymentRequest")).
					Return(&circle.Payout{
						ID:     circlePayoutID,
						Status: circle.TransferStatusPending,
						Amount: circle.Balance{
							Amount:   payment.Amount,
							Currency: "USD",
						},
					}, nil).
					Once()
			},
			fnAsserts: func(t *testing.T, sqlExecuter db.SQLExecuter, payment *data.Payment) {
				// Payment should be marked as pending
				var assertErr error
				payment, assertErr = models.Payment.Get(ctx, payment.ID, sqlExecuter)
				require.NoError(t, assertErr)
				assert.Equal(t, data.PendingPaymentStatus, payment.Status)

				// Transfer request is still not updated for the main connection pool
				var transferRequest data.CircleTransferRequest
				assertErr = dbConnectionPool.GetContext(ctx, &transferRequest, "SELECT * FROM circle_transfer_requests WHERE payment_id = $1", payment.ID)
				require.NoError(t, assertErr)
				assert.Nil(t, transferRequest.CirclePayoutID)
				assert.Nil(t, transferRequest.SourceWalletID)

				// Transfer request is updated for the transaction
				assertErr = sqlExecuter.GetContext(ctx, &transferRequest, "SELECT * FROM circle_transfer_requests WHERE payment_id = $1", payment.ID)
				require.NoError(t, assertErr)
				assert.Equal(t, circlePayoutID, *transferRequest.CirclePayoutID)
				assert.Equal(t, circleWalletID, *transferRequest.SourceWalletID)
				assert.Equal(t, data.CircleTransferStatusPending, *transferRequest.Status)
			},
		},
	}

	// Errors that invalidate the Circle recipient:
	for _, circleErrCode := range circle.DestinationAddressErrorCodes {
		tests = append(tests, TestCase{
			name: fmt.Sprintf("🟠[CircleAPI.error.code=%d] should move the CircleRecipient to status=denied", circleErrCode),
			fnSetup: func(t *testing.T, m *circle.MockService, payment *data.Payment, circleRecipient *data.CircleRecipient) {
				transferRequest, setupErr := models.CircleTransferRequests.Insert(ctx, dbConnectionPool, payment.ID)
				require.NoError(t, setupErr)

				m.On("SendPayout", ctx, circle.PaymentRequest{
					APIType:          circle.APITypePayouts,
					SourceWalletID:   circleWalletID,
					RecipientID:      circleRecipient.CircleRecipientID,
					Amount:           payment.Amount,
					StellarAssetCode: payment.Asset.Code,
					IdempotencyKey:   transferRequest.IdempotencyKey,
				}).
					Return(nil, &circle.APIError{Code: circleErrCode}).
					Once()
			},
			fnAsserts: func(t *testing.T, sqlExecuter db.SQLExecuter, payment *data.Payment) {
				// Payment should be marked as failed
				var assertErr error
				payment, assertErr = models.Payment.Get(ctx, payment.ID, sqlExecuter)
				require.NoError(t, assertErr)
				assert.Equal(t, data.FailedPaymentStatus, payment.Status)

				// Circle recipient should be marked as Denied
				circleRecipient, assertErr := models.CircleRecipient.GetByReceiverWalletID(ctx, payment.ReceiverWallet.ID)
				require.NoError(t, assertErr)
				assert.Equal(t, data.CircleRecipientStatusDenied, circleRecipient.Status)
			},
		})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Receiver Wallets
			rwRegistered := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, disbursement.Wallet.ID, data.RegisteredReceiversWalletStatus)
			circleRecipient := data.CreateCircleRecipientFixture(t, ctx, dbConnectionPool, data.CircleRecipient{
				ReceiverWalletID:  rwRegistered.ID,
				Status:            data.CircleRecipientStatusActive,
				StellarAddress:    rwRegistered.StellarAddress,
				StellarMemo:       rwRegistered.StellarMemo,
				CircleRecipientID: uuid.NewString(),
			})
			payment := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
				ReceiverWallet: rwRegistered,
				Disbursement:   disbursement,
				Asset:          *disbursement.Asset,
				Amount:         "100",
				Status:         data.ReadyPaymentStatus,
			})

			dbTx, runErr := dbConnectionPool.BeginTxx(ctx, nil)
			require.NoError(t, runErr)

			// Teardown
			defer func() {
				err = dbTx.Rollback()
				require.NoError(t, err)

				data.DeleteAllCircleTransferRequests(t, ctx, dbConnectionPool)
				data.DeleteAllCircleRecipientsFixtures(t, ctx, dbConnectionPool)
				data.DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
				data.DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
			}()

			mCircleService := circle.NewMockService(t)

			mDistAccountResolver := mocks.NewMockDistributionAccountResolver(t)
			mDistAccountResolver.
				On("DistributionAccountFromContext", ctx).
				Return(schema.TransactionAccount{
					Type:           schema.DistributionAccountCircleDBVault,
					CircleWalletID: circleWalletID,
					Status:         schema.AccountStatusActive,
				}, nil).Maybe()

			dispatcher := NewCirclePaymentPayoutDispatcher(models, mCircleService, mDistAccountResolver)

			if tt.fnSetup != nil {
				tt.fnSetup(t, mCircleService, payment, circleRecipient)
			}
			runErr = dispatcher.DispatchPayments(ctx, dbTx, tenantID, []*data.Payment{payment})
			if tt.wantErrContains != nil {
				for _, wantErr := range tt.wantErrContains {
					assert.ErrorContains(t, runErr, wantErr)
				}
			} else {
				assert.NoError(t, runErr)
			}

			if tt.fnAsserts != nil {
				tt.fnAsserts(t, dbTx, payment)
			}
		})
	}
}

func Test_CirclePaymentPayoutDispatcher_SupportedPlatform(t *testing.T) {
	dispatcher := CirclePaymentPayoutDispatcher{}
	assert.Equal(t, schema.CirclePlatform, dispatcher.SupportedPlatform())
}
