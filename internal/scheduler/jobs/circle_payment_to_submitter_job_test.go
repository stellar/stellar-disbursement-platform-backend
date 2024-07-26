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

func Test_PaymentToSubmitterJob_GetInterval(t *testing.T) {
	interval := 5
	p := NewCirclePaymentToSubmitterJob(CirclePaymentToSubmitterJobOptions{JobIntervalSeconds: interval})
	require.Equal(t, time.Duration(interval)*time.Second, p.GetInterval())
}

func Test_PaymentToSubmitterJob_GetName(t *testing.T) {
	p := NewCirclePaymentToSubmitterJob(CirclePaymentToSubmitterJobOptions{JobIntervalSeconds: 5})
	require.Equal(t, circlePaymentToSubmitterJobName, p.GetName())
}

func Test_CirclePaymentToSubmitterJob_IsJobMultiTenant(t *testing.T) {
	p := NewCirclePaymentToSubmitterJob(CirclePaymentToSubmitterJobOptions{JobIntervalSeconds: 5})
	require.Equal(t, true, p.IsJobMultiTenant())
}

func Test_CirclePaymentToSubmitterJob_Execute(t *testing.T) {
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
			mockCirclePaymentToSubmitterService := &mocks.MockPaymentToSubmitterService{}
			mockCirclePaymentToSubmitterService.On("SendBatchPayments", mock.Anything, circlePaymentToSubmitterBatchSize).
				Return(tt.sendPayments(nil, circlePaymentToSubmitterBatchSize))
			mDistAccResolver := sigMocks.NewMockDistributionAccountResolver(t)
			mDistAccResolver.
				On("DistributionAccountFromContext", mock.Anything).
				Return(schema.TransactionAccount{Type: schema.DistributionAccountCircleDBVault}, nil).
				Maybe()

			p := circlePaymentToSubmitterJob{
				paymentToSubmitterSvc: mockCirclePaymentToSubmitterService,
				distAccountResolver:   mDistAccResolver,
			}

			err := p.Execute(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("circlePaymentToSubmitterJob.Execute() error = %v, wantErr %v", err, tt.wantErr)
			}

			mockCirclePaymentToSubmitterService.AssertExpectations(t)
		})
	}
}
