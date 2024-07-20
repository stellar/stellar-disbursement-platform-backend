package jobs

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/mocks"
)

func Test_circleReconciliationJob_GetInterval(t *testing.T) {
	job := NewCircleReconciliationJob(CircleReconciliationJobOptions{})
	require.Equal(t, circleReconciliationJobIntervalSeconds*time.Second, job.GetInterval())
}

func Test_circleReconciliationJob_GetName(t *testing.T) {
	job := NewCircleReconciliationJob(CircleReconciliationJobOptions{})
	require.Equal(t, circleReconciliationJobName, job.GetName())
}

func Test_circleReconciliationJob_IsJobMultiTenant(t *testing.T) {
	job := NewCircleReconciliationJob(CircleReconciliationJobOptions{})
	require.Equal(t, true, job.IsJobMultiTenant())
}

func Test_circleReconciliationJob_Execute(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name            string
		prepareMocksFn  func(mReconciliationService *mocks.MockCircleReconciliationService)
		wantErrContains string
	}{
		{
			name: "🔴 execution fails",
			prepareMocksFn: func(mReconciliationService *mocks.MockCircleReconciliationService) {
				mReconciliationService.
					On("Reconcile", ctx).
					Return(assert.AnError).
					Once()
			},
			wantErrContains: "executing Job",
		},
		{
			name: "🟢 execution succeeds",
			prepareMocksFn: func(mReconciliationService *mocks.MockCircleReconciliationService) {
				mReconciliationService.
					On("Reconcile", ctx).
					Return(nil).
					Once()
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mReconciliationService := mocks.NewMockCircleReconciliationService(t)
			tc.prepareMocksFn(mReconciliationService)
			job := circleReconciliationJob{
				jobIntervalSeconds:    5,
				reconciliationService: mReconciliationService,
			}

			err := job.Execute(ctx)
			if tc.wantErrContains != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErrContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
