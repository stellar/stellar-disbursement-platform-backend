package jobs

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

func Test_WalletCreationFromSubmitterJob_GetInterval(t *testing.T) {
	interval := 5
	j := NewWalletCreationFromSubmitterJob(interval, &data.Models{}, nil, "Test SDF Network ; September 2015")
	require.Equal(t, time.Duration(interval)*time.Second, j.GetInterval())
}

func Test_WalletCreationFromSubmitterJob_GetName(t *testing.T) {
	j := NewWalletCreationFromSubmitterJob(5, &data.Models{}, nil, "Test SDF Network ; September 2015")
	require.Equal(t, walletCreationFromSubmitterJobName, j.GetName())
}

func Test_WalletCreationFromSubmitterJob_IsJobMultiTenant(t *testing.T) {
	j := NewWalletCreationFromSubmitterJob(5, &data.Models{}, nil, "Test SDF Network ; September 2015")
	require.Equal(t, true, j.IsJobMultiTenant())
}

func Test_WalletCreationFromSubmitterJob_Execute(t *testing.T) {
	tests := []struct {
		name                   string
		tenantDistributionType schema.AccountType
		syncBatchTransactions  func(ctx context.Context, batchSize int, tenantID string) error
		wantErr                bool
		expectServiceCall      bool
	}{
		{
			name:                   "SyncBatchTransactions success with Stellar distribution account",
			tenantDistributionType: schema.DistributionAccountStellarEnv,
			syncBatchTransactions: func(ctx context.Context, batchSize int, tenantID string) error {
				return nil
			},
			wantErr:           false,
			expectServiceCall: true,
		},
		{
			name:                   "SyncBatchTransactions returns error with Stellar distribution account",
			tenantDistributionType: schema.DistributionAccountStellarEnv,
			syncBatchTransactions: func(ctx context.Context, batchSize int, tenantID string) error {
				return fmt.Errorf("sync error")
			},
			wantErr:           true,
			expectServiceCall: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			tenantInfo := &tenant.Tenant{
				ID:                      "95e788b6-c80e-4975-9d12-141001fe6e44",
				Name:                    "aid-org-1",
				DistributionAccountType: tc.tenantDistributionType,
			}
			ctx = tenant.SaveTenantInContext(ctx, tenantInfo)

			mockService := &mocks.MockWalletCreationFromSubmitterService{}

			if tc.expectServiceCall {
				mockService.On("SyncBatchTransactions", mock.Anything, walletCreationFromSubmitterBatchSize, tenantInfo.ID).
					Return(tc.syncBatchTransactions(nil, walletCreationFromSubmitterBatchSize, tenantInfo.ID))
			}

			j := walletCreationFromSubmitterJob{
				service: mockService,
			}

			err := j.Execute(ctx)
			if (err != nil) != tc.wantErr {
				t.Errorf("walletCreationFromSubmitterJob.Execute() error = %v, wantErr %v", err, tc.wantErr)
			}

			mockService.AssertExpectations(t)
		})
	}
}

func Test_WalletCreationFromSubmitterJob_Execute_NoTenantInContext(t *testing.T) {
	ctx := context.Background()

	mockService := &mocks.MockWalletCreationFromSubmitterService{}

	j := walletCreationFromSubmitterJob{
		service: mockService,
	}

	err := j.Execute(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "error getting tenant from context")

	mockService.AssertExpectations(t)
}
