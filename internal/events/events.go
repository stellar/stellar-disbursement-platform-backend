package events

import (
	"context"
	"fmt"

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
	log.Ctx(ctx).Debugf("[NoopProducer] the following messages are not being published, please make sure to rely on the scheduler for them: %+v", messages)
	return nil
}

func (p NoopProducer) Close() error {
	return nil
}

var _ Producer = NoopProducer{}

func ProduceEvent(ctx context.Context, producer Producer, msg *Message) error {
	if msg == nil {
		log.Ctx(ctx).Warn("message is nil, not producing event")
		return nil
	}

	if producer == nil {
		log.Ctx(ctx).Errorf("event producer is nil, could not publish message %+v", msg)
		return nil
	}

	err := producer.WriteMessages(ctx, *msg)
	if err != nil {
		return fmt.Errorf("writing message %+v on event producer: %w", msg, err)
	}

	return nil
}
