package jobs

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/anchorplatform"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewAnchorPlatformAuthMonitoringJob(t *testing.T) {
	apService := &anchorplatform.AnchorPlatformAPIService{}
	monitorService := &monitor.MockMonitorService{}
	crashTrackerClient := &crashtracker.MockCrashTrackerClient{}

	testCases := []struct {
		name               string
		apService          anchorplatform.AnchorPlatformAPIServiceInterface
		monitorService     monitor.MonitorServiceInterface
		crashTrackerClient crashtracker.CrashTrackerClient
		wantErrContains    string
	}{
		{
			name:               "return an error if apService is nil",
			apService:          nil,
			monitorService:     nil,
			crashTrackerClient: nil,
			wantErrContains:    "apService cannot be nil",
		},
		{
			name:               "return an error if monitorService is nil",
			apService:          apService,
			monitorService:     nil,
			crashTrackerClient: nil,
			wantErrContains:    "monitorService cannot be nil",
		},
		{
			name:               "return an error if crashTrackerClient is nil",
			apService:          apService,
			monitorService:     monitorService,
			crashTrackerClient: nil,
			wantErrContains:    "crashTrackerClient cannot be nil",
		},
		{
			name:               "🎉 successfully creates a new instance",
			apService:          apService,
			monitorService:     monitorService,
			crashTrackerClient: crashTrackerClient,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			apAuthMonitoringJob, err := NewAnchorPlatformAuthMonitoringJob(tc.apService, tc.monitorService, tc.crashTrackerClient)
			if tc.wantErrContains != "" {
				require.ErrorContains(t, err, tc.wantErrContains)
				require.Nil(t, apAuthMonitoringJob)
			} else {
				require.NoError(t, err)
				require.NotNil(t, apAuthMonitoringJob)
				assert.Equal(t, tc.apService, apAuthMonitoringJob.apService)
				assert.Equal(t, tc.monitorService, apAuthMonitoringJob.monitorService)
				assert.Equal(t, tc.crashTrackerClient, apAuthMonitoringJob.crashTrackerClient)
			}
		})
	}
}

func Test_AnchorPlatformAuthMonitoringJob_GetInterval(t *testing.T) {
	apAuthMonitoringJob := &AnchorPlatformAuthMonitoringJob{}
	gotInterval := apAuthMonitoringJob.GetInterval()
	assert.Equal(t, apAuthMonitoringJobIntervalSeconds*time.Second, gotInterval)
}

func Test_AnchorPlatformAuthMonitoringJob_GetName(t *testing.T) {
	apAuthMonitoringJob := &AnchorPlatformAuthMonitoringJob{}
	gotName := apAuthMonitoringJob.GetName()
	assert.Equal(t, apAuthMonitoringJobName, gotName)
}

func Test_AnchorPlatformAuthMonitoringJob_Execute(t *testing.T) {
	apService := &anchorplatform.AnchorPlatformAPIServiceMock{}
	monitorService := &monitor.MockMonitorService{}
	crashTrackerClient := &crashtracker.MockCrashTrackerClient{}

	apAuthMonitoringJob, err := NewAnchorPlatformAuthMonitoringJob(apService, monitorService, crashTrackerClient)
	require.NoError(t, err)

	ctx := context.Background()
	var nilMap map[string]string

	t.Run("handle error from apService.IsAnchorProtectedByAuth", func(t *testing.T) {
		// prepare mocks
		apService.On("IsAnchorProtectedByAuth", ctx).Return(false, fmt.Errorf("apService error")).Once()

		// execute and assert result
		err := apAuthMonitoringJob.Execute(ctx)
		require.EqualError(t, err, "executing AnchorPlatformAuthMonitoringJob: apService error")

		// assert mocks
		apService.AssertExpectations(t)
		monitorService.AssertExpectations(t)
		crashTrackerClient.AssertExpectations(t)
	})

	t.Run("handle 'isProtected==false' with error from monitorService.MonitorCounters", func(t *testing.T) {
		// prepare mocks
		apService.On("IsAnchorProtectedByAuth", ctx).Return(false, nil).Once()
		crashTrackerClient.On("LogAndReportMessages", ctx, "Anchor platform is not enforcing authentication").Once()
		monitorService.On("MonitorCounters", monitor.AnchorPlatformAuthProtectionMissingCounterTag, nilMap).Return(fmt.Errorf("monitorService error")).Once()

		// execute and assert result
		err := apAuthMonitoringJob.Execute(ctx)
		require.NoError(t, err)

		// assert mocks
		apService.AssertExpectations(t)
		monitorService.AssertExpectations(t)
		crashTrackerClient.AssertExpectations(t)
	})

	t.Run("handle 'isProtected==false' without error from monitorService.MonitorCounters", func(t *testing.T) {
		// prepare mocks
		apService.On("IsAnchorProtectedByAuth", ctx).Return(false, nil).Once()
		crashTrackerClient.On("LogAndReportMessages", ctx, "Anchor platform is not enforcing authentication").Once()
		monitorService.On("MonitorCounters", monitor.AnchorPlatformAuthProtectionMissingCounterTag, nilMap).Return(nil).Once()

		// execute and assert result
		err := apAuthMonitoringJob.Execute(ctx)
		require.NoError(t, err)

		// assert mocks
		apService.AssertExpectations(t)
		monitorService.AssertExpectations(t)
		crashTrackerClient.AssertExpectations(t)
	})

	t.Run("handle 'isProtected==true' with error from monitorService.MonitorCounters", func(t *testing.T) {
		// prepare mocks
		apService.On("IsAnchorProtectedByAuth", ctx).Return(true, nil).Once()
		monitorService.On("MonitorCounters", monitor.AnchorPlatformAuthProtectionEnsuredCounterTag, nilMap).Return(fmt.Errorf("monitorService error")).Once()

		// execute and assert result
		err := apAuthMonitoringJob.Execute(ctx)
		require.NoError(t, err)

		// assert mocks
		apService.AssertExpectations(t)
		monitorService.AssertExpectations(t)
		crashTrackerClient.AssertExpectations(t)
	})

	t.Run("handle 'isProtected==true' without error from monitorService.MonitorCounters", func(t *testing.T) {
		// prepare mocks
		apService.On("IsAnchorProtectedByAuth", ctx).Return(true, nil).Once()
		monitorService.On("MonitorCounters", monitor.AnchorPlatformAuthProtectionEnsuredCounterTag, nilMap).Return(nil).Once()

		// execute and assert result
		err := apAuthMonitoringJob.Execute(ctx)
		require.NoError(t, err)

		// assert mocks
		apService.AssertExpectations(t)
		monitorService.AssertExpectations(t)
		crashTrackerClient.AssertExpectations(t)
	})
}
