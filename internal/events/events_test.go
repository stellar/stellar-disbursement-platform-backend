package events

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stretchr/testify/assert"
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

func (c *MockConsumer) Close() error {
	args := c.Called()
	return args.Error(0)
}

func TestConsume(t *testing.T) {
	ctx := context.Background()
	consumer := &MockConsumer{}
	crashTracker := &crashtracker.MockCrashTrackerClient{}

	unexpectedErr := errors.New("unexpected error")
	consumer.
		On("ReadMessage", ctx).
		Return(unexpectedErr).
		Once().
		On("ReadMessage", ctx).
		Return(io.EOF).
		Once().
		On("ReadMessage", ctx).
		Return(nil)

	crashTracker.
		On("LogAndReportErrors", ctx, unexpectedErr, "consuming messages").
		Return().
		Once()

	Consume(ctx, consumer, crashTracker)

	tick := time.Tick(time.Second * 1)
	go func() {
		Consume(ctx, consumer, crashTracker)
	}()

	<-tick

	consumer.AssertExpectations(t)
	crashTracker.AssertExpectations(t)
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
