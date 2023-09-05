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

func Test_PaymentsProcessorJob_GetInterval(t *testing.T) {
	p := NewPaymentsProcessorJob(&data.Models{})
	require.Equal(t, PaymentsJobIntervalSeconds*time.Second, p.GetInterval())
}

func Test_PaymentsProcessorJob_GetName(t *testing.T) {
	p := NewPaymentsProcessorJob(&data.Models{})
	require.Equal(t, PaymentJobName, p.GetName())
}

func Test_PaymentsProcessorJob_Execute(t *testing.T) {
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
			mockPaymentToSubmitterService.On("SendBatchPayments", mock.Anything, PaymentsBatchSize).
				Return(tt.sendPayments(nil, PaymentsBatchSize))

			p := PaymentsProcessorJob{
				service: mockPaymentToSubmitterService,
			}

			err := p.Execute(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("PaymentsProcessorJob.Execute() error = %v, wantErr %v", err, tt.wantErr)
			}

			mockPaymentToSubmitterService.AssertExpectations(t)
		})
	}
}
