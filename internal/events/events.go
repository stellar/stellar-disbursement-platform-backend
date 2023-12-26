package events

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stretchr/testify/mock"
)

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
	return fmt.Sprintf("Topic: %s - Key: %s - Type: %s - Tenant ID: %s", m.Topic, m.Key, m.Type, m.TenantID)
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

type Producer interface {
	WriteMessages(ctx context.Context, messages ...Message) error
	Close() error
}

type Consumer interface {
	ReadMessage(ctx context.Context) error
	Close() error
}

func Consume(ctx context.Context, consumer Consumer, crashTracker crashtracker.CrashTrackerClient) {
	log.Ctx(ctx).Info("starting consuming messages...")

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)

	for {
		select {
		case <-ctx.Done():
			log.Ctx(ctx).Infof("Stopping consuming messages due to context cancellation...")
			return

		case sig := <-signalChan:
			log.Ctx(ctx).Infof("Stopping consuming messages due to OS signal '%+v'", sig)
			return

		default:
			if err := consumer.ReadMessage(ctx); err != nil {
				if errors.Is(err, io.EOF) {
					log.Ctx(ctx).Warn("message broker returned EOF") // This is an end state
					return
				}
				crashTracker.LogAndReportErrors(ctx, err, "consuming messages")
			}
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
