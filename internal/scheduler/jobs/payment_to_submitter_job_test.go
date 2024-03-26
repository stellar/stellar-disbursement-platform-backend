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

func Test_PaymentToSubmitterJob_GetInterval(t *testing.T) {
	interval := 5
	p := NewPaymentToSubmitterJob(interval, &data.Models{}, nil)
	require.Equal(t, time.Duration(interval)*time.Second, p.GetInterval())
}

func Test_PaymentToSubmitterJob_GetName(t *testing.T) {
	p := NewPaymentToSubmitterJob(5, &data.Models{}, nil)
	require.Equal(t, paymentToSubmitterJobName, p.GetName())
}

func Test_PaymentToSubmitterJob_IsJobMultiTenant(t *testing.T) {
	p := NewPaymentToSubmitterJob(5, &data.Models{}, nil)
	require.Equal(t, true, p.IsJobMultiTenant())
}

func Test_PaymentToSubmitterJob_Execute(t *testing.T) {
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
			mockPaymentToSubmitterService.On("SendBatchPayments", mock.Anything, paymentToSubmitterBatchSize).
				Return(tt.sendPayments(nil, paymentToSubmitterBatchSize))

			p := paymentToSubmitterJob{
				paymentToSubmitterSvc: mockPaymentToSubmitterService,
			}

			err := p.Execute(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("paymentToSubmitterJob.Execute() error = %v, wantErr %v", err, tt.wantErr)
			}

			mockPaymentToSubmitterService.AssertExpectations(t)
		})
	}
}
