package events

import (
	"context"

	"github.com/stellar/go/support/log"
)

// Producer is an interface that defines the methods that a producer should implement.
type Producer interface {
	WriteMessages(ctx context.Context, messages ...Message) error
	Close() error
}

// Consumer is an interface that defines the methods that a consumer should implement.
type Consumer interface {
	ReadMessage(ctx context.Context) (*Message, error)
	Topic() string
	Handlers() []EventHandler
	Close() error
}

// NoopProducer is a producer used to log messages instead of sending them to a real producer.
type NoopProducer struct{}

func (p NoopProducer) WriteMessages(ctx context.Context, messages ...Message) error {
	log.Ctx(ctx).Debugf("NoopProducer: These messages will be discarded and handled by the scheduler: %+v", messages)
	return nil
}

func (p NoopProducer) Close() error {
	return nil
}

var _ Producer = NoopProducer{}
