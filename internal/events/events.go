package events

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
	"github.com/stretchr/testify/mock"
)

const MAX_BACKOFF_DURATION = 8

var (
	ErrTopicRequired    = errors.New("message topic is required")
	ErrKeyRequired      = errors.New("message key is required")
	ErrTenantIDRequired = errors.New("message tenant ID is required")
	ErrTypeRequired     = errors.New("message type is required")
	ErrDataRequired     = errors.New("message data is required")
)

type Message struct {
	Topic    string `json:"topic"`
	Key      string `json:"key"`
	TenantID string `json:"tenant_id"`
	Type     string `json:"type"`
	Data     any    `json:"data"`
}

func (m Message) String() string {
	return fmt.Sprintf("Message{Topic: %s, Key: %s, Type: %s, TenantID: %s, Data: %v}", m.Topic, m.Key, m.Type, m.TenantID, m.Data)
}

func (m Message) Validate() error {
	if m.Topic == "" {
		return ErrTopicRequired
	}

	if m.Key == "" {
		return ErrKeyRequired
	}

	if m.TenantID == "" {
		return ErrTenantIDRequired
	}

	if m.Type == "" {
		return ErrTypeRequired
	}

	if m.Data == nil {
		return ErrDataRequired
	}

	return nil
}

// NewMessage returns a new message with values passed by parameters. It also parses the `TenantID` from the context and inject it into the message.
// Returns error if the tenant is not found in the context.
func NewMessage(ctx context.Context, topic, key, messageType string, data any) (*Message, error) {
	tnt, err := tenant.GetTenantFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting tenant from context: %w", err)
	}

	return &Message{
		Topic:    topic,
		Key:      key,
		TenantID: tnt.ID,
		Type:     messageType,
		Data:     data,
	}, nil
}

type Producer interface {
	WriteMessages(ctx context.Context, messages ...Message) error
	Close() error
}

type Consumer interface {
	ReadMessage(ctx context.Context) error
	Topic() string
	Close() error
}

func Consume(ctx context.Context, consumer Consumer, crashTracker crashtracker.CrashTrackerClient) {
	log.Ctx(ctx).Info("starting consuming messages...")

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)

	var backoff time.Duration
	var backoffCounter int
	backoffChan := make(chan struct{}, 1)
	defer close(backoffChan)

	for {
		select {
		case <-ctx.Done():
			log.Ctx(ctx).Infof("Stopping consuming messages for topic %s due to context cancellation...", consumer.Topic())
			return

		case sig := <-signalChan:
			log.Ctx(ctx).Infof("Stopping consuming messages for topic %s due to OS signal '%+v'", consumer.Topic(), sig)
			return

		case <-backoffChan:
			log.Ctx(ctx).Warnf("Waiting %s before retrying reading new messages", backoff)
			time.Sleep(backoff)

		default:
			if err := consumer.ReadMessage(ctx); err != nil {
				crashTracker.LogAndReportErrors(ctx, err, fmt.Sprintf("consuming messages for topic %s", consumer.Topic()))

				backoffCounter++
				if backoffCounter > MAX_BACKOFF_DURATION {
					backoffCounter = MAX_BACKOFF_DURATION
				}

				// No need to handle this error since it only returns error when retry > 32, < 0
				backoff, _ = utils.ExponentialBackoffInSeconds(backoffCounter)
				backoffChan <- struct{}{}
				continue
			}
			backoff = 0
			backoffCounter = 0
		}
	}
}

type MockConsumer struct {
	mock.Mock
}

var _ Consumer = new(MockConsumer)

func (c *MockConsumer) ReadMessage(ctx context.Context) error {
	args := c.Called(ctx)
	return args.Error(0)
}

func (c *MockConsumer) RegisterEventHandler(ctx context.Context, eventHandlers ...EventHandler) error {
	args := c.Called(ctx, eventHandlers)
	return args.Error(0)
}

func (c *MockConsumer) Topic() string {
	return c.Called().String(0)
}

func (c *MockConsumer) Close() error {
	args := c.Called()
	return args.Error(0)
}

type MockProducer struct {
	mock.Mock
}

var _ Producer = new(MockProducer)

func (c *MockProducer) WriteMessages(ctx context.Context, messages ...Message) error {
	args := c.Called(ctx, messages)
	return args.Error(0)
}

func (c *MockProducer) Close() error {
	args := c.Called()
	return args.Error(0)
}
