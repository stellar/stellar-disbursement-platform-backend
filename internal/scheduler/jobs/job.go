package jobs

import (
	"context"
	"time"
)

type Job interface {
	Execute(context.Context) error
	GetInterval() time.Duration
	GetName() string
}
