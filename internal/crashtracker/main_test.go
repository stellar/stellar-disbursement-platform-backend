package crashtracker

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_ParseCrashTrackerType(t *testing.T) {
	testCases := []struct {
		metricTypeStr            string
		expectedCrashTrackerType CrashTrackerType
		wantErr                  error
	}{
		{wantErr: fmt.Errorf("invalid crash tracker type \"\"")},
		{metricTypeStr: "MOCKCRASHTRACKERTYPE", wantErr: fmt.Errorf("invalid crash tracker type \"MOCKCRASHTRACKERTYPE\"")},
		{metricTypeStr: "sentry", expectedCrashTrackerType: CrashTrackerTypeSentry},
		{metricTypeStr: "SENtry", expectedCrashTrackerType: CrashTrackerTypeSentry},
		{metricTypeStr: "DRY_run", expectedCrashTrackerType: CrashTrackerTypeDryRun},
		{metricTypeStr: "dry_run", expectedCrashTrackerType: CrashTrackerTypeDryRun},
	}
	for _, tc := range testCases {
		t.Run("crashTrackerType: "+tc.metricTypeStr, func(t *testing.T) {
			crashTrackerType, err := ParseCrashTrackerType(tc.metricTypeStr)
			assert.Equal(t, tc.expectedCrashTrackerType, crashTrackerType)
			assert.Equal(t, tc.wantErr, err)
		})
	}
}

func Test_GetClient(t *testing.T) {
	ctx := context.Background()
	crashTrackerOptions := CrashTrackerOptions{}

	t.Run("get sentry crash tracker client", func(t *testing.T) {
		crashTrackerOptions.CrashTrackerType = CrashTrackerTypeSentry

		gotClient, err := GetClient(ctx, crashTrackerOptions)
		assert.NoError(t, err)
		assert.IsType(t, &sentryClient{}, gotClient)
	})

	t.Run("get dry run crash tracker client", func(t *testing.T) {
		crashTrackerOptions.CrashTrackerType = CrashTrackerTypeDryRun

		gotClient, err := GetClient(ctx, crashTrackerOptions)
		assert.NoError(t, err)
		assert.IsType(t, &dryRunClient{}, gotClient)
	})

	t.Run("error metric passed is invalid", func(t *testing.T) {
		crashTrackerOptions.CrashTrackerType = "MOCKCRASHTRACKERTYPE"

		gotClient, err := GetClient(ctx, crashTrackerOptions)
		assert.Nil(t, gotClient)
		assert.EqualError(t, err, "unknown crash tracker type: \"MOCKCRASHTRACKERTYPE\"")
	})
}
