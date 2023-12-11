package events

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
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

func (c *MockConsumer) Close() error {
	args := c.Called()
	return args.Error(0)
}

func TestConsume(t *testing.T) {
	ctx := context.Background()
	consumer := &MockConsumer{}

	consumer.
		On("ReadMessage", ctx).
		Return(errors.New("unexpected error")).
		Once().
		On("ReadMessage", ctx).
		Return(nil)

	err := Consume(ctx, consumer)
	assert.EqualError(t, err, "consuming messages: unexpected error")

	tick := time.Tick(time.Second * 1)
	go func() {
		err := Consume(ctx, consumer)
		require.NoError(t, err)
	}()

	<-tick

	consumer.AssertExpectations(t)
}

func Test_Message_Validate(t *testing.T) {
	m := Message{}

	err := m.Validate()
	assert.ErrorIs(t, err, ErrTopicRequired)

	m.Topic = "test-topic"
	err = m.Validate()
	assert.ErrorIs(t, err, ErrKeyRequired)

	m.Key = "test-key"
	err = m.Validate()
	assert.ErrorIs(t, err, ErrTenantIDRequired)

	m.TenantID = "tenant-ID"
	err = m.Validate()
	assert.ErrorIs(t, err, ErrTypeRequired)

	m.Type = "test-type"
	err = m.Validate()
	assert.ErrorIs(t, err, ErrDataRequired)

	m.Data = "test"
	err = m.Validate()
	assert.NoError(t, err)

	m.Data = nil
	m.Data = map[string]string{"test": "test"}
	err = m.Validate()
	assert.NoError(t, err)

	m.Data = nil
	m.Data = struct{ Name string }{Name: "test"}
	err = m.Validate()
	assert.NoError(t, err)
}
