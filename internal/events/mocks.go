package events

import (
	"context"

	"github.com/stellar/go/support/log"

	"github.com/stretchr/testify/mock"
)

// MockConsumer is a mock implementation of Consumer
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

func (c *MockConsumer) BrokerType() EventBrokerType {
	return c.Called().Get(0).(EventBrokerType)
}

// MockProducer is a mock implementation of Producer
type MockProducer struct {
	mock.Mock
}

var _ Producer = new(MockProducer)

func (c *MockProducer) WriteMessages(ctx context.Context, messages ...Message) error {
	args := c.Called(ctx, messages)
	return args.Error(0)
}

func (c *MockProducer) Close(ctx context.Context) {
	c.Called(ctx)
}

func (c *MockProducer) Ping(ctx context.Context) error {
	args := c.Called(ctx)
	return args.Error(0)
}

func (c *MockProducer) BrokerType() EventBrokerType {
	return c.Called().Get(0).(EventBrokerType)
}

type testInterface interface {
	mock.TestingT
	Cleanup(func())
}

// NewMockProducer creates a new instance of MockProducer. It also registers a testing interface on the mock and a
// cleanup function to assert the mocks expectations.
func NewMockProducer(t testInterface) *MockProducer {
	mock := &MockProducer{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}

// MockEventHandler is a mock implementation of EventHandler
type MockEventHandler struct {
	mock.Mock
}

func NewMockEventHandler(t testInterface) *MockEventHandler {
	mock := MockEventHandler{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return &mock
}

func (h *MockEventHandler) Handle(ctx context.Context, msg *Message) error {
	log.Ctx(ctx).Infof("Handling message with key %s by handler %s", msg.Key, h.Name())
	args := h.Called(ctx, msg)
	return args.Error(0)
}

func (h *MockEventHandler) CanHandleMessage(ctx context.Context, msg *Message) bool {
	args := h.Called(ctx, msg)
	return args.Bool(0)
}

func (h *MockEventHandler) Name() string {
	args := h.Called()
	return args.String(0)
}
