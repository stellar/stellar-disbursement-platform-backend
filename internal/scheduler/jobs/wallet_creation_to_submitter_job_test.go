package jobs

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/mocks"
	sigMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

func Test_WalletCreationToSubmitterJob_GetInterval(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	interval := 5
	job := NewWalletCreationToSubmitterJob(WalletCreationToSubmitterJobOptions{
		JobIntervalSeconds:  interval,
		Models:              models,
		TSSDBConnectionPool: dbConnectionPool,
		DistAccountResolver: sigMocks.NewMockDistributionAccountResolver(t),
	})

	require.NotNil(t, job)
	assert.Equal(t, time.Duration(interval)*time.Second, job.GetInterval())
}

func Test_WalletCreationToSubmitterJob_GetName(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	job := NewWalletCreationToSubmitterJob(WalletCreationToSubmitterJobOptions{
		JobIntervalSeconds:  5,
		Models:              models,
		TSSDBConnectionPool: dbConnectionPool,
		DistAccountResolver: sigMocks.NewMockDistributionAccountResolver(t),
	})

	assert.Equal(t, walletCreationToSubmitterJobName, job.GetName())
}

func Test_WalletCreationToSubmitterJob_IsJobMultiTenant(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	job := NewWalletCreationToSubmitterJob(WalletCreationToSubmitterJobOptions{
		JobIntervalSeconds:  5,
		Models:              models,
		TSSDBConnectionPool: dbConnectionPool,
		DistAccountResolver: sigMocks.NewMockDistributionAccountResolver(t),
	})

	assert.True(t, job.IsJobMultiTenant())
}

func Test_WalletCreationToSubmitterJob_Execute(t *testing.T) {
	tests := []struct {
		name              string
		resolveAccount    schema.TransactionAccount
		resolveErr        error
		serviceErr        error
		expectServiceCall bool
		wantErr           string
	}{
		{
			name:              "submits wallet creations successfully",
			resolveAccount:    schema.TransactionAccount{Type: schema.DistributionAccountStellarDBVault},
			expectServiceCall: true,
		},
		{
			name:              "returns error when service fails",
			resolveAccount:    schema.TransactionAccount{Type: schema.DistributionAccountStellarDBVault},
			serviceErr:        fmt.Errorf("service error"),
			expectServiceCall: true,
			wantErr:           "service error",
		},
		{
			name:              "propagates distribution account resolver error",
			resolveErr:        fmt.Errorf("resolver error"),
			expectServiceCall: false,
			wantErr:           "resolver error",
		},
		{
			name:              "skips when distribution account is not stellar",
			resolveAccount:    schema.TransactionAccount{Type: schema.DistributionAccountCircleDBVault},
			expectServiceCall: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mService := mocks.NewMockWalletCreationToSubmitterService(t)
			if tt.expectServiceCall {
				mService.
					On("SendBatchWalletCreations", mock.Anything, walletCreationToSubmitterBatchSize).
					Return(tt.serviceErr).
					Once()
			}

			distResolver := sigMocks.NewMockDistributionAccountResolver(t)
			distResolver.
				On("DistributionAccountFromContext", mock.Anything).
				Return(tt.resolveAccount, tt.resolveErr).
				Maybe()

			job := walletCreationToSubmitterJob{
				service:             mService,
				jobIntervalSeconds:  5,
				distAccountResolver: distResolver,
			}

			err := job.Execute(context.Background())
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				assert.NoError(t, err)
			}

			mService.AssertExpectations(t)
		})
	}
}
