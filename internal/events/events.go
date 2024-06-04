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
	log.Ctx(ctx).Debugf("NoopProducer: These messages will be discarded and handled by the scheduler: %+v", messages)
	return nil
}

func (p NoopProducer) Close() error {
	return nil
}

var _ Producer = NoopProducer{}

func ProduceEvents(ctx context.Context, producer Producer, messages ...*Message) error {
	if producer == nil {
		log.Ctx(ctx).Errorf("event producer is nil, could not publish messages %+v", messages)
		return nil
	}

	var messagesToProduce []Message
	for i, msg := range messages {
		if msg == nil {
			log.Ctx(ctx).Warnf("message at index %d is nil, not producing event", i)
			continue
		} else {
			messagesToProduce = append(messagesToProduce, *msg)
		}
	}
	if len(messagesToProduce) == 0 {
		log.Ctx(ctx).Warn("not producing events, since there are zero not-nil messages to produce")
		return nil
	}

	log.Ctx(ctx).Debugf("writing %d messages on the event producer", len(messagesToProduce))
	err := producer.WriteMessages(ctx, messagesToProduce...)
	if err != nil {
		return fmt.Errorf("writing messages %+v on event producer: %w", messagesToProduce, err)
	}

	return nil
}
