package crashtracker

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stellar/go/support/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_DryRun_LogAndReportErrors(t *testing.T) {
	mDryRunClient := &dryRunClient{}
	mMsgError := "error"
	mError := fmt.Errorf("mock error")
	ctx := context.Background()

	t.Run("LogAndReportErrors without message", func(t *testing.T) {
		// set the logger to a buffer so we can check the error message
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)

		mDryRunClient.LogAndReportErrors(ctx, mError, mMsgError)

		// validate logs
		require.Contains(t, buf.String(), "error: mock error")
	})

	t.Run("LogAndReportErrors with message", func(t *testing.T) {
		// set the logger to a buffer so we can check the error message
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)

		mDryRunClient.LogAndReportErrors(ctx, mError, mMsgError)

		// validate logs
		require.Contains(t, buf.String(), "mock error")
	})
}

func Test_DryRun_LogAndReportMessages(t *testing.T) {
	mDryRunClient := &dryRunClient{}
	mMsg := "mock message"

	t.Run("LogAndReportMessages without message", func(t *testing.T) {
		// set the logger to a buffer so we can check the error message
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)
		log.DefaultLogger.SetLevel(log.InfoLevel)

		mDryRunClient.LogAndReportMessages(context.Background(), mMsg)

		// validate logs
		require.Contains(t, buf.String(), "mock message")
	})
}

func Test_DryRun_FlushEvents(t *testing.T) {
	mDryRunClient := &dryRunClient{}

	waitTimeout := time.Second
	valid := mDryRunClient.FlushEvents(waitTimeout)

	assert.Equal(t, false, valid)
}

func Test_DryRun_Clone(t *testing.T) {
	mDryRunClient := &dryRunClient{}

	waitTimeout := time.Second
	valid := mDryRunClient.FlushEvents(waitTimeout)

	assert.Equal(t, false, valid)

	cloneClient := mDryRunClient.Clone()

	assert.IsType(t, &dryRunClient{}, cloneClient)
	assert.NotEqual(t, mDryRunClient, &cloneClient)
}
