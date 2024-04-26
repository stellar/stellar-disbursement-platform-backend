package jobs

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockReadyPaymentsCancellation struct {
	mock.Mock
}

func (s *mockReadyPaymentsCancellation) CancelReadyPayments(ctx context.Context) error {
	args := s.Called(ctx)
	return args.Error(0)
}

func Test_ReadyPaymentsCancellationJob(t *testing.T) {
	j := readyPaymentsCancellationJob{}

	assert.Equal(t, readyPaymentsCancellationJobName, j.GetName())
	assert.Equal(t, readyPaymentsCancellationJobInterval*time.Minute, j.GetInterval())
	assert.True(t, j.IsJobMultiTenant())
}

func Test_ReadyPaymentsCancellationJob_Execute(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	mockService := mockReadyPaymentsCancellation{}
	j := &readyPaymentsCancellationJob{
		service: &mockService,
	}

	t.Run("returns error when cancellation service fails", func(t *testing.T) {
		getEntries := log.DefaultLogger.StartTest(log.ErrorLevel)
		mockService.On("CancelReadyPayments", ctx).Return(errors.New("Unexpected error")).Once()

		err := j.Execute(ctx)
		assert.EqualError(t, err, "error cancelling ready payments: Unexpected error")

		entries := getEntries()
		require.Len(t, entries, 1)
		assert.Equal(t, entries[0].Message, "error cancelling ready payments: Unexpected error")
	})

	t.Run("executes successfully", func(t *testing.T) {
		mockService.On("CancelReadyPayments", ctx).Return(nil).Once()

		err := j.Execute(ctx)
		assert.NoError(t, err)
	})
}

func Test_NewReadyPaymentsCancellationJob(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	j := NewReadyPaymentsCancellationJob(models)
	assert.NotNil(t, j)
}
