package jobs

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/mocks"
	sigMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

func Test_StellarPaymentToSubmitterJob_GetInterval(t *testing.T) {
	interval := 5
	p := NewStellarPaymentToSubmitterJob(StellarPaymentToSubmitterJobOptions{JobIntervalSeconds: interval})
	require.Equal(t, time.Duration(interval)*time.Second, p.GetInterval())
}

func Test_StellarPaymentToSubmitterJob_GetName(t *testing.T) {
	p := NewStellarPaymentToSubmitterJob(StellarPaymentToSubmitterJobOptions{JobIntervalSeconds: 5})
	require.Equal(t, stellarPaymentToSubmitterJobName, p.GetName())
}

func Test_StellarPaymentToSubmitterJob_IsJobMultiTenant(t *testing.T) {
	p := NewStellarPaymentToSubmitterJob(StellarPaymentToSubmitterJobOptions{JobIntervalSeconds: 5})
	require.Equal(t, true, p.IsJobMultiTenant())
}

func Test_StellarPaymentToSubmitterJob_Execute(t *testing.T) {
	tests := []struct {
		name         string
		sendPayments func(ctx context.Context, batchSize int) error
		wantErr      bool
	}{
		{
			name: "SendBatchPayments success",
			sendPayments: func(ctx context.Context, batchSize int) error {
				return nil
			},
			wantErr: false,
		},
		{
			name: "SendBatchPayments returns error",
			sendPayments: func(ctx context.Context, batchSize int) error {
				return fmt.Errorf("error")
			},
			wantErr: true,
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
			if (err != nil) != tt.wantErr {
				t.Errorf("stellarPaymentToSubmitterJob.Execute() error = %v, wantErr %v", err, tt.wantErr)
			}

			mockPaymentToSubmitterService.AssertExpectations(t)
		})
	}
}
