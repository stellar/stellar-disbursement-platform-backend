package jobs

import (
	"context"
	"time"
)

const DefaultMinimumJobIntervalSeconds = 1

type Job interface {
	Execute(context.Context) error
	GetInterval() time.Duration
	GetName() string
	IsJobMultiTenant() bool
}
