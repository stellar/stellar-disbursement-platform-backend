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

type EventConsumer struct {
	consumer     Consumer
	producer     Producer
	crashTracker crashtracker.CrashTrackerClient
	maxBackoff   int
}

func NewEventConsumer(consumer Consumer, producer Producer, crashTracker crashtracker.CrashTrackerClient) *EventConsumer {
	return &EventConsumer{
		consumer:     consumer,
		producer:     producer,
		crashTracker: crashTracker,
		maxBackoff:   DefaultMaxBackoffExponent,
	}
}

func (ec *EventConsumer) Consume(ctx context.Context) {
	log.Ctx(ctx).Infof("starting consuming messages for topic %s...", ec.consumer.Topic())

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)

	backoffChan := make(chan struct{}, 1)
	defer close(backoffChan)
	backoffManager := NewBackoffManager(backoffChan, ec.maxBackoff)

	for {
		select {
		case <-ctx.Done():
			log.Ctx(ctx).Infof("Stopping consuming messages for topic %s due to context cancellation...", ec.consumer.Topic())
			ec.finalizeConsumer(ctx, backoffManager.GetMessage())
			return

		case sig := <-signalChan:
			log.Ctx(ctx).Infof("Stopping consuming messages for topic %s due to OS signal '%+v'", ec.consumer.Topic(), sig)
			ec.finalizeConsumer(ctx, backoffManager.GetMessage())
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
				log.Ctx(ctx).Warnf("Max backoff reached for topic %s.", ec.consumer.Topic())
				if msg != nil {
					err := ec.sendMessageToDLQ(ctx, *msg)
					if err != nil {
						ec.crashTracker.LogAndReportErrors(ctx, err, fmt.Sprintf("sending message to DLQ for topic %s", ec.consumer.Topic()))
					}
				}
				backoffManager.ResetBackoff()
				continue
			}

			// 3. If no message in backoff manager, read message from Kafka.
			if msg == nil {
				var readErr error
				msg, readErr = ec.consumer.ReadMessage(ctx)
				if readErr != nil {
					ec.crashTracker.LogAndReportErrors(ctx, readErr, fmt.Sprintf("consuming messages for topic %s", ec.consumer.Topic()))
					backoffManager.TriggerBackoff()
					continue
				}
			}

			// 4. Run the message through the handler chain.
			if handleErr := ec.handleMessage(ctx, msg); handleErr != nil {
				backoffManager.TriggerBackoffWithMessage(msg, handleErr)
				ec.crashTracker.LogAndReportErrors(ctx, handleErr, fmt.Sprintf("handling message for topic %s", ec.consumer.Topic()))
				continue
			}

			// 5. Message handled successfully, reset backoff.
			backoffManager.ResetBackoff()
		}
	}
}

// finalizeConsumer replays the message back to the original topic in case of a failure.
func (ec *EventConsumer) finalizeConsumer(ctx context.Context, msg *Message) {
	if msg == nil {
		log.Ctx(ctx).Infof("No message to finalize for topic %s", ec.consumer.Topic())
		return
	}
	log.Ctx(ctx).Warnf("Replaying message with key %s to topic %s", msg.Key, msg.Topic)
	err := ec.producer.WriteMessages(ctx, *msg)
	if err != nil {
		ec.crashTracker.LogAndReportErrors(ctx, err, fmt.Sprintf("replaying message to topic %s", msg.Topic))
		return
	}
}

// sendMessageToDLQ sends the message to the DLQ.
func (ec *EventConsumer) sendMessageToDLQ(ctx context.Context, msg Message) error {
	log.Ctx(ctx).Warnf("Sending message with key %s to DLQ for topic %s", msg.Key, msg.Topic)

	msg.Topic = msg.Topic + ".dlq"
	err := ec.producer.WriteMessages(ctx, msg)
	if err != nil {
		return fmt.Errorf("sending message %s to DLQ for topic %s: %w", msg, msg.Topic, err)
	}
	return nil
}

// handleMessage handles the message by the handler chain of the consumer.
func (ec *EventConsumer) handleMessage(ctx context.Context, msg *Message) error {
	for _, handler := range ec.consumer.Handlers() {
		if handler.CanHandleMessage(ctx, msg) {
			handleErr := handler.Handle(ctx, msg)
			if handleErr != nil {
				return fmt.Errorf("handling message: %w", handleErr)
			}
		}
	}
	return nil
}
