package events

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_EventConsumer_Consume(t *testing.T) {
	// setup mocks
	consumerMock := &MockConsumer{}
	crashTrackerMock := &crashtracker.MockCrashTrackerClient{}
	producerMock := &MockProducer{}

	msg := &Message{Key: "key-1", Topic: "test.test_topic"}
	unexpectedErr := errors.New("unexpected error")

	ec := NewEventConsumer(consumerMock, producerMock, crashTrackerMock)

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Second*8))
	defer cancel()

	crashTrackerMock.
		On("LogAndReportErrors", ctx, unexpectedErr, "consuming messages for topic test.test_topic").
		Return()

	consumerMock.
		On("Topic").
		Return("test.test_topic").
		On("ReadMessage", ctx).
		Return(nil, unexpectedErr).
		Twice().
		On("ReadMessage", ctx).
		Return(msg, nil).
		Once().
		On("ReadMessage", ctx).
		Return(nil, unexpectedErr).
		Once().
		On("ReadMessage", ctx).
		Return(msg, nil).
		On("Handlers").
		Return([]EventHandler{})

	getEntries := log.DefaultLogger.StartTest(log.WarnLevel)

	ec.Consume(ctx)

	entries := getEntries()
	require.Len(t, entries, 3)
	assert.Equal(t, "Waiting 2s before retrying reading new messages", entries[0].Message)
	assert.Equal(t, "Waiting 4s before retrying reading new messages", entries[1].Message)
	assert.Equal(t, "Waiting 2s before retrying reading new messages", entries[2].Message) // backoffManager.ResetBackoff() was called

	consumerMock.AssertExpectations(t)
	crashTrackerMock.AssertExpectations(t)
}

func Test_EventConsumer_Consume_SendDLQ(t *testing.T) {
	// setup mocks
	consumerMock := &MockConsumer{}
	crashTrackerMock := &crashtracker.MockCrashTrackerClient{}
	producerMock := &MockProducer{}

	handlingErr := errors.New("handling message for topic test.test_topic")
	msg := &Message{Key: "key-1", Topic: "test.test_topic"}

	ec := NewEventConsumer(consumerMock, producerMock, crashTrackerMock)
	ec.maxBackoff = 1

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Second*3))
	defer cancel()

	crashTrackerMock.
		On("LogAndReportErrors", mock.Anything, mock.Anything, handlingErr.Error()).
		Return()

	consumerMock.
		On("Topic").
		Return("test.test_topic").
		On("ReadMessage", ctx).
		Return(msg, nil).
		On("Handlers").
		Return([]EventHandler{&FailEventHandler{}})

	producerMock.
		On("WriteMessages", ctx, mock.Anything).
		Return(nil)

	getEntries := log.DefaultLogger.StartTest(log.WarnLevel)

	ec.Consume(ctx)

	entries := getEntries()
	assert.Equal(t, "Waiting 2s before retrying reading new messages", entries[0].Message)
	assert.Equal(t, "Max backoff reached for topic test.test_topic.", entries[1].Message)
	assert.Equal(t, "Sending message with key key-1 to DLQ for topic test.test_topic", entries[2].Message) // backoffManager.ResetBackoff() was called

	consumerMock.AssertExpectations(t)
	crashTrackerMock.AssertExpectations(t)
	producerMock.AssertExpectations(t)
}

func Test_EventConsumer_Consume_FinalizeConsumer(t *testing.T) {
	// setup mocks
	consumerMock := &MockConsumer{}
	crashTrackerMock := &crashtracker.MockCrashTrackerClient{}
	producerMock := &MockProducer{}

	handlingErr := errors.New("handling message for topic test.test_topic")
	msg := &Message{Key: "key-1", Topic: "test.test_topic"}

	ec := NewEventConsumer(consumerMock, producerMock, crashTrackerMock)

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Second*1))
	defer cancel()

	crashTrackerMock.
		On("LogAndReportErrors", mock.Anything, mock.Anything, handlingErr.Error()).
		Return()

	consumerMock.
		On("Topic").
		Return("test.test_topic").
		On("ReadMessage", ctx).
		Return(msg, nil).
		On("Handlers").
		Return([]EventHandler{&FailEventHandler{}})

	producerMock.
		On("WriteMessages", mock.Anything, mock.MatchedBy(func(m []Message) bool {
			if len(m) != 1 {
				return false
			}
			message := m[0]
			// Verify that the message being re-broadcasted is the same as the one that failed
			if len(message.Errors) != 1 && message.Errors[0].ErrorMessage != handlingErr.Error() {
				return false
			}
			return message.Key == msg.Key && message.Topic == msg.Topic
		})).
		Return(nil)

	getEntries := log.DefaultLogger.StartTest(log.WarnLevel)

	ec.Consume(ctx)

	entries := getEntries()
	assert.Equal(t, "Waiting 2s before retrying reading new messages", entries[0].Message)
	assert.Equal(t, "Replaying message with key key-1 to topic test.test_topic", entries[1].Message)

	consumerMock.AssertExpectations(t)
	crashTrackerMock.AssertExpectations(t)
	producerMock.AssertExpectations(t)
}

// Always fail event handler
type FailEventHandler struct{}

func (h *FailEventHandler) Handle(ctx context.Context, msg *Message) error {
	return errors.New("handler failed")
}

func (h *FailEventHandler) CanHandleMessage(ctx context.Context, msg *Message) bool {
	return true
}

func (h *FailEventHandler) Name() string {
	return "FailEventHandler"
}
