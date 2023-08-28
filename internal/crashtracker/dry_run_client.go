package crashtracker

import (
	"context"
	"fmt"
	"time"

	"github.com/stellar/go/support/log"
)

type dryRunClient struct{}

func (s *dryRunClient) LogAndReportErrors(ctx context.Context, err error, msg string) {
	if msg != "" {
		err = fmt.Errorf("%s: %w", msg, err)
	}
	log.Ctx(ctx).Errorf("[DRY_RUN Crash Reporter] %+v", err)
}

func (s *dryRunClient) LogAndReportMessages(ctx context.Context, msg string) {
	log.Ctx(ctx).Infof("[DRY_RUN Crash Reporter] %s", msg)
}

func (s *dryRunClient) FlushEvents(waitTime time.Duration) bool {
	return false
}

func (s *dryRunClient) Recover() {}

func (s *dryRunClient) Clone() CrashTrackerClient {
	return &dryRunClient{}
}

func NewDryRunClient() (*dryRunClient, error) {
	return &dryRunClient{}, nil
}

// Ensuring that dryRunClient is implementing CrashTrackerClient interface
var _ CrashTrackerClient = (*dryRunClient)(nil)
