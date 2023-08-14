package crashtracker

import (
	"context"
	"time"
)

type CrashTrackerClient interface {
	LogAndReportErrors(ctx context.Context, err error, msg string)
	LogAndReportMessages(ctx context.Context, msg string)
	FlushEvents(waitTime time.Duration) bool
	Recover()
	Clone() CrashTrackerClient
}
