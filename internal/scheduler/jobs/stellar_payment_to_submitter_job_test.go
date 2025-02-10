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

func Test_StellarPaymentToSubmitterJob_GetInterval(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	interval := 5
	p := NewStellarPaymentToSubmitterJob(StellarPaymentToSubmitterJobOptions{JobIntervalSeconds: interval, Models: models})
	require.Equal(t, time.Duration(interval)*time.Second, p.GetInterval())
}

func Test_StellarPaymentToSubmitterJob_GetName(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	p := NewStellarPaymentToSubmitterJob(StellarPaymentToSubmitterJobOptions{JobIntervalSeconds: 5, Models: models})
	require.Equal(t, stellarPaymentToSubmitterJobName, p.GetName())
}

func Test_StellarPaymentToSubmitterJob_IsJobMultiTenant(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	p := NewStellarPaymentToSubmitterJob(StellarPaymentToSubmitterJobOptions{JobIntervalSeconds: 5, Models: models})
	require.Equal(t, true, p.IsJobMultiTenant())
}

func Test_StellarPaymentToSubmitterJob_Execute(t *testing.T) {
	tests := []struct {
		name         string
		sendPayments func(ctx context.Context, batchSize int) error
		wantErr      error
	}{
		{
			name: "SendBatchPayments success",
			sendPayments: func(ctx context.Context, batchSize int) error {
				return nil
			},
			wantErr: nil,
		},
		{
			name: "SendBatchPayments returns error",
			sendPayments: func(ctx context.Context, batchSize int) error {
				return fmt.Errorf("error")
			},
			wantErr: fmt.Errorf("error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockPaymentToSubmitterService := &mocks.MockPaymentToSubmitterService{}
			mockPaymentToSubmitterService.On("SendBatchPayments", mock.Anything, stellarPaymentToSubmitterBatchSize).
				Return(tt.sendPayments(nil, stellarPaymentToSubmitterBatchSize))
			mDistAccResolver := sigMocks.NewMockDistributionAccountResolver(t)
			mDistAccResolver.
				On("DistributionAccountFromContext", mock.Anything).
				Return(schema.TransactionAccount{Type: schema.DistributionAccountStellarDBVault}, nil).
				Maybe()

			p := stellarPaymentToSubmitterJob{
				paymentToSubmitterSvc: mockPaymentToSubmitterService,
				distAccountResolver:   mDistAccResolver,
			}

			err := p.Execute(context.Background())
			if tt.wantErr != nil {
				assert.NotNil(t, err)
				assert.ErrorContains(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
			}

			mockPaymentToSubmitterService.AssertExpectations(t)
		})
	}
}
