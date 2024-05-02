package events

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
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

func Consume(ctx context.Context, consumer Consumer, dlqProducer Producer, crashTracker crashtracker.CrashTrackerClient) {
	log.Ctx(ctx).Infof("starting consuming messages for topic %s...", consumer.Topic())

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)

	backoffChan := make(chan struct{}, 1)
	defer close(backoffChan)
	backoffManager := NewBackoffManager(backoffChan)

	for {
		select {
		case <-ctx.Done():
			log.Ctx(ctx).Infof("Stopping consuming messages for topic %s due to context cancellation...", consumer.Topic())
			return

		case sig := <-signalChan:
			log.Ctx(ctx).Infof("Stopping consuming messages for topic %s due to OS signal '%+v'", consumer.Topic(), sig)
			return

		case <-backoffChan:
			backoff := backoffManager.GetBackoffDuration()
			log.Ctx(ctx).Warnf("Waiting %s before retrying reading new messages", backoff)
			time.Sleep(backoff)

		default:

			// 1. Attempt fetching msg from backoff manager in case it was already Read from Consumer.
			msg := backoffManager.GetMessage()

			// 2. If Backoff max reached, send message to DLQ and reset backoff.
			if backoffManager.IsMaxBackoffReached() {
				log.Ctx(ctx).Warnf("Max backoff reached for topic %s.", consumer.Topic())
				if msg != nil {
					err := sendMessageToDLQ(ctx, dlqProducer, *msg)
					if err != nil {
						crashTracker.LogAndReportErrors(ctx, err, fmt.Sprintf("sending message to DLQ for topic %s", consumer.Topic()))
					}
				}
				backoffManager.ResetBackoff()
				continue
			}

			// 3. If no message in backoff manager, read message from Kafka.
			if msg == nil {
				var consumeErr error
				msg, consumeErr = consumer.ReadMessage(ctx)
				if consumeErr != nil {
					crashTracker.LogAndReportErrors(ctx, consumeErr, fmt.Sprintf("consuming messages for topic %s", consumer.Topic()))
					backoffManager.TriggerBackoff()
					continue
				}
			}

			// 4. Run the message through the handler chain.
			if handleErr := handleMessage(ctx, consumer, msg); handleErr != nil {
				backoffManager.TriggerBackoffWithMessage(msg, handleErr)
				crashTracker.LogAndReportErrors(ctx, handleErr, fmt.Sprintf("handling message for topic %s", consumer.Topic()))
				continue
			}

			// 5. Message handled successfully, reset backoff.
			backoffManager.ResetBackoff()
		}
	}
}

func sendMessageToDLQ(ctx context.Context, dlqProducer Producer, msg Message) error {
	log.Ctx(ctx).Warnf("Sending message with key %s to DLQ for topic %s", msg.Key, msg.Topic)

	msg.Topic = msg.Topic + ".dlq"
	err := dlqProducer.WriteMessages(ctx, msg)
	if err != nil {
		return fmt.Errorf("sending message %s to DLQ for topic %s: %w", msg.String(), msg.Topic, err)
	}
	return nil
}

// handleMessage handles the message by the handler chain of the consumer.
func handleMessage(ctx context.Context, consumer Consumer, msg *Message) error {
	for _, handler := range consumer.Handlers() {
		if handler.CanHandleMessage(ctx, msg) {
			handleErr := handler.Handle(ctx, msg)
			if handleErr != nil {
				return fmt.Errorf("handling message: %w", handleErr)
			}
		}
	}
	return nil
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
