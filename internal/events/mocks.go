package events

import (
	"context"

	"github.com/stretchr/testify/mock"
)

type MockConsumer struct {
	mock.Mock
}

var _ Consumer = new(MockConsumer)

func (c *MockConsumer) ReadMessage(ctx context.Context) (*Message, error) {
	args := c.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Message), args.Error(1)
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

func (c *MockConsumer) Handlers() []EventHandler {
	args := c.Called()
	return args.Get(0).([]EventHandler)
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
