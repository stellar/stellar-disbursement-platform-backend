package jobs

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/mocks"
)

func Test_PaymentFromSubmitterJob_GetInterval(t *testing.T) {
	interval := 5
	p := NewPaymentFromSubmitterJob(interval, &data.Models{}, nil)
	require.Equal(t, time.Duration(interval)*time.Second, p.GetInterval())
}

func Test_PaymentFromSubmitterJob_GetName(t *testing.T) {
	p := NewPaymentFromSubmitterJob(5, &data.Models{}, nil)
	require.Equal(t, paymentFromSubmitterJobName, p.GetName())
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
			ctx := context.Background()
			tenantInfo := &tenant.Tenant{
				ID:                      "95e788b6-c80e-4975-9d12-141001fe6e44",
				Name:                    "aid-org-1",
				DistributionAccountType: schema.DistributionAccountStellarEnv,
			}
			ctx = tenant.SaveTenantInContext(ctx, tenantInfo)

			mockPaymentFromSubmitterService := &mocks.MockPaymentFromSubmitterService{}
			mockPaymentFromSubmitterService.On("SyncBatchTransactions", mock.Anything, paymentFromSubmitterBatchSize, tenantInfo.ID).
				Return(tt.syncTransactions(nil, paymentFromSubmitterBatchSize))

			p := paymentFromSubmitterJob{
				service: mockPaymentFromSubmitterService,
			}

			err := p.Execute(ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("paymentFromSubmitterJob.Execute() error = %v, wantErr %v", err, tt.wantErr)
			}

			mockPaymentFromSubmitterService.AssertExpectations(t)
		})
	}
}
