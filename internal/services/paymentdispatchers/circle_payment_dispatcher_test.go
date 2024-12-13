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
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

func Test_CirclePaymentDispatcher_ensureRecipientIsReady_success(t *testing.T) {
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

	initialTime := time.Now().Add(-time.Hour)
	recipientInsertTemplate := data.CircleRecipient{
		ReceiverWalletID:  receiverWallet.ID,
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

	for _, failedStatus := range []data.CircleRecipientStatus{data.CircleRecipientStatusInactive, data.CircleRecipientStatusDenied} {
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

			dispatcher := NewCirclePaymentDispatcher(models, mCircleService, mDistAccountResolver)

			finalRecipient, err := dispatcher.ensureRecipientIsReady(ctx, *receiverWallet)
			require.NoError(t, err)
			tc.assertRecipients(t, initialRecipient, finalRecipient)
		})
	}
}

func Test_CirclePaymentDispatcher_ensureRecipientIsReady_failure(t *testing.T) {
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

			dispatcher := NewCirclePaymentDispatcher(models, mCircleService, mDistAccountResolver)

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

func Test_CirclePaymentDispatcher_ensureRecipientIsReadyWithRetry(t *testing.T) {
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
					Times(5)
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

	for _, nonSuccessfulState := range []data.CircleRecipientStatus{data.CircleRecipientStatusInactive, data.CircleRecipientStatusDenied, data.CircleRecipientStatusPending} {
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
					Times(5)
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

			dispatcher := NewCirclePaymentDispatcher(models, mCircleService, mDistAccountResolver)

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

func Test_CirclePaymentDispatcher_DispatchPayments(t *testing.T) {
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
	receiver1 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})

	// Receiver Wallets
	rw1Registered := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver1.ID, disbursement.Wallet.ID, data.RegisteredReceiversWalletStatus)

	// Circle Recipient
	circleRecipient := data.CreateCircleRecipientFixture(t, ctx, dbConnectionPool, data.CircleRecipient{
		ReceiverWalletID:  rw1Registered.ID,
		Status:            data.CircleRecipientStatusActive,
		CircleRecipientID: uuid.NewString(),
	})

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
		fnSetup            func(*testing.T, *circle.MockService)
		fnAsserts          func(*testing.T, db.SQLExecuter)
	}{
		{
			name: "failure validating payment ready for sending",
			paymentsToDispatch: []*data.Payment{
				{
					ID:             "123",
					ReceiverWallet: rw1Registered,
				},
			},
			wantErr: fmt.Errorf("payment with ID 123 does not exist"),
		},
		{
			name:               "payment marked as failed when posting circle transfer fails",
			paymentsToDispatch: []*data.Payment{payment1},
			wantErr:            nil,
			fnSetup: func(t *testing.T, m *circle.MockService) {
				transferRequest, setupErr := models.CircleTransferRequests.Insert(ctx, payment1.ID)
				require.NoError(t, setupErr)

				m.On("SendPayment", ctx, circle.PaymentRequest{
					SourceWalletID:   circleWalletID,
					RecipientID:      circleRecipient.CircleRecipientID,
					Amount:           payment1.Amount,
					StellarAssetCode: payment1.Asset.Code,
					IdempotencyKey:   transferRequest.IdempotencyKey,
				}).
					Return(nil, fmt.Errorf("error posting transfer to Circle")).
					Once()
			},
			fnAsserts: func(t *testing.T, sqlExecuter db.SQLExecuter) {
				// Payment should be marked as failed
				payment, assertErr := models.Payment.Get(ctx, payment1.ID, sqlExecuter)
				require.NoError(t, assertErr)
				assert.Equal(t, data.FailedPaymentStatus, payment.Status)
			},
		},
		{
			name:               "error updating circle transfer request",
			paymentsToDispatch: []*data.Payment{payment1},
			wantErr:            fmt.Errorf("updating circle transfer request: payout cannot be nil"),
			fnSetup: func(t *testing.T, m *circle.MockService) {
				m.On("SendPayment", ctx, mock.AnythingOfType("circle.PaymentRequest")).
					Return(nil, nil).
					Once()
			},
		},
		{
			name:               "error updating payment status for completed request",
			paymentsToDispatch: []*data.Payment{payment1},
			wantErr:            fmt.Errorf("invalid input value for enum circle_transfer_status"),
			fnSetup: func(t *testing.T, m *circle.MockService) {
				m.On("SendPayment", ctx, mock.AnythingOfType("circle.PaymentRequest")).
					Return(&circle.Payout{
						ID:     "payout_id",
						Status: "wrong-status",
					}, nil).
					Once()
			},
		},
		{
			name:               "success posting transfer to Circle",
			paymentsToDispatch: []*data.Payment{payment1},
			wantErr:            nil,
			fnSetup: func(t *testing.T, m *circle.MockService) {
				m.On("SendPayment", ctx, mock.AnythingOfType("circle.PaymentRequest")).
					Return(&circle.Payout{
						ID:     circlePayoutID,
						Status: circle.TransferStatusPending,
						Amount: circle.Balance{
							Amount:   payment1.Amount,
							Currency: "USD",
						},
					}, nil).
					Once()
			},
			fnAsserts: func(t *testing.T, sqlExecuter db.SQLExecuter) {
				// Payment should be marked as pending
				payment, assertErr := models.Payment.Get(ctx, payment1.ID, sqlExecuter)
				require.NoError(t, assertErr)
				assert.Equal(t, data.PendingPaymentStatus, payment.Status)

				// Transfer request is still not updated for the main connection pool
				var transferRequest data.CircleTransferRequest
				assertErr = dbConnectionPool.GetContext(ctx, &transferRequest, "SELECT * FROM circle_transfer_requests WHERE payment_id = $1", payment1.ID)
				require.NoError(t, assertErr)
				assert.Nil(t, transferRequest.CirclePayoutID)
				assert.Nil(t, transferRequest.SourceWalletID)

				// Transfer request is updated for the transaction
				assertErr = sqlExecuter.GetContext(ctx, &transferRequest, "SELECT * FROM circle_transfer_requests WHERE payment_id = $1", payment1.ID)
				require.NoError(t, assertErr)
				assert.Equal(t, circlePayoutID, *transferRequest.CirclePayoutID)
				assert.Equal(t, circleWalletID, *transferRequest.SourceWalletID)
				assert.Equal(t, data.CircleTransferStatusPending, *transferRequest.Status)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dbtx, runErr := dbConnectionPool.BeginTxx(ctx, nil)
			require.NoError(t, runErr)

			// Teardown
			defer func() {
				err = dbtx.Rollback()
				require.NoError(t, err)

				_, err = dbConnectionPool.ExecContext(ctx, "DELETE FROM circle_transfer_requests")
				require.NoError(t, err)
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

			dispatcher := NewCirclePaymentDispatcher(models, mCircleService, mDistAccountResolver)

			if tt.fnSetup != nil {
				tt.fnSetup(t, mCircleService)
			}
			runErr = dispatcher.DispatchPayments(ctx, dbtx, tenantID, tt.paymentsToDispatch)
			if tt.wantErr != nil {
				assert.ErrorContains(t, runErr, tt.wantErr.Error())
			} else {
				assert.NoError(t, runErr)
			}

			if tt.fnAsserts != nil {
				tt.fnAsserts(t, dbtx)
			}
		})
	}
}

func Test_CirclePaymentDispatcher_SupportedPlatform(t *testing.T) {
	dispatcher := CirclePaymentDispatcher{}
	assert.Equal(t, schema.CirclePlatform, dispatcher.SupportedPlatform())
}
