package jobs

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_PaymentFromSubmitterJob_GetInterval(t *testing.T) {
	interval := 5
	p := NewPaymentFromSubmitterJob(interval, &data.Models{}, nil)
	require.Equal(t, time.Duration(interval)*time.Second, p.GetInterval())
}

func Test_PaymentFromSubmitterJob_GetName(t *testing.T) {
	p := NewPaymentFromSubmitterJob(5, &data.Models{}, nil)
	require.Equal(t, PaymentFromSubmitterJobName, p.GetName())
}

func Test_PaymentFromSubmitterJob_IsJobMultiTenant(t *testing.T) {
	p := NewPaymentFromSubmitterJob(5, &data.Models{}, nil)
	require.Equal(t, true, p.IsJobMultiTenant())
}

func Test_PaymentFromSubmitterJob_Execute(t *testing.T) {
	tests := []struct {
		name             string
		syncTransactions func(ctx context.Context, batchSize int) error
		wantErr          bool
	}{
		{
			name: "SyncBatchTransactions success",
			syncTransactions: func(ctx context.Context, batchSize int) error {
				return nil
			},
			wantErr: false,
		},
		{
			name: "SyncBatchTransactions returns error",
			syncTransactions: func(ctx context.Context, batchSize int) error {
				return fmt.Errorf("error")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockPaymentFromSubmitterService := &mocks.MockPaymentFromSubmitterService{}
			mockPaymentFromSubmitterService.On("SyncBatchTransactions", mock.Anything, PaymentFromSubmitterBatchSize).
				Return(tt.syncTransactions(nil, PaymentFromSubmitterBatchSize))

			p := PaymentFromSubmitterJob{
				service: mockPaymentFromSubmitterService,
			}

			err := p.Execute(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("PaymentFromSubmitterJob.Execute() error = %v, wantErr %v", err, tt.wantErr)
			}

			mockPaymentFromSubmitterService.AssertExpectations(t)
		})
	}
}
