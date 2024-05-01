package events

import (
	"context"

	"github.com/stretchr/testify/mock"
)

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

type testInterface interface {
	mock.TestingT
	Cleanup(func())
}

// NewMockProducer creates a new instance of MockProducer. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
func NewMockProducer(t testInterface) *MockProducer {
	mock := &MockProducer{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
