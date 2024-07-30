package jobs

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/mocks"
	sigMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

func Test_CirclePaymentToSubmitterJob_GetInterval(t *testing.T) {
	interval := 5
	p := NewCirclePaymentToSubmitterJob(CirclePaymentToSubmitterJobOptions{JobIntervalSeconds: interval})
	require.Equal(t, time.Duration(interval)*time.Second, p.GetInterval())
}

func Test_CirclePaymentToSubmitterJob_GetName(t *testing.T) {
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
			mockPaymentToSubmitterService.On("SendBatchPayments", mock.Anything, circlePaymentToSubmitterBatchSize).
				Return(tt.sendPayments(nil, circlePaymentToSubmitterBatchSize))
			mDistAccResolver := sigMocks.NewMockDistributionAccountResolver(t)
			mDistAccResolver.
				On("DistributionAccountFromContext", mock.Anything).
				Return(schema.TransactionAccount{Type: schema.DistributionAccountCircleDBVault}, nil).
				Maybe()

			p := circlePaymentToSubmitterJob{
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
