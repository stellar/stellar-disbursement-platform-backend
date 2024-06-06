package jobs

import (
	"context"
	"time"
)

const DefaultMinimumJobIntervalSeconds = 5

type Job interface {
	Execute(context.Context) error
	GetInterval() time.Duration
	GetName() string
	IsJobMultiTenant() bool
}
