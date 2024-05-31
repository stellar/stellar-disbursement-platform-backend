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
		On("LogAndReportErrors", ctx, unexpectedErr, "consuming messages for topic test.test_topic").Return()

	consumerMock.
		On("Topic").Return("test.test_topic").
		On("ReadMessage", ctx).Return(nil, unexpectedErr).Twice().
		On("ReadMessage", ctx).Return(msg, nil).Once().
		On("ReadMessage", ctx).Return(nil, unexpectedErr).Once().
		On("ReadMessage", ctx).Return(msg, nil).
		On("Handlers").Return([]EventHandler{})

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
	failedEventHandlerMock := &MockEventHandler{}

	handlingErr := errors.New("handling message for topic test.test_topic")
	msg := &Message{Key: "key-1", Topic: "test.test_topic"}

	ec := NewEventConsumer(consumerMock, producerMock, crashTrackerMock)
	ec.maxBackoff = 1

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Second*3))
	defer cancel()

	crashTrackerMock.
		On("LogAndReportErrors", mock.Anything, mock.Anything, handlingErr.Error()).Return()

	consumerMock.
		On("Topic").Return("test.test_topic").
		On("ReadMessage", ctx).Return(msg, nil).
		On("Handlers").Return([]EventHandler{failedEventHandlerMock})

	failedEventHandlerMock.
		On("Handle", ctx, msg).Return(handlingErr).
		On("CanHandleMessage", ctx, msg).Return(true).
		On("Name").Return("FailedEventHandler")

	producerMock.
		On("WriteMessages", ctx, mock.Anything).Return(nil)

	getEntries := log.DefaultLogger.StartTest(log.WarnLevel)

	ec.Consume(ctx)

	entries := getEntries()
	assert.Equal(t, "Waiting 2s before retrying handling message with key key-1", entries[0].Message)
	assert.Equal(t, "Max backoff reached for topic test.test_topic.", entries[1].Message)
	assert.Equal(t, "Sending message with key key-1 to DLQ for topic test.test_topic", entries[2].Message) // backoffManager.ResetBackoff() was called

	consumerMock.AssertExpectations(t)
	crashTrackerMock.AssertExpectations(t)
	producerMock.AssertExpectations(t)
	failedEventHandlerMock.AssertExpectations(t)
}

func Test_EventConsumer_Consume_HandleFailedOnly(t *testing.T) {
	// setup mocks
	consumerMock := &MockConsumer{}
	crashTrackerMock := &crashtracker.MockCrashTrackerClient{}
	producerMock := &MockProducer{}
	failedEventHandlerMock := &MockEventHandler{}
	successfulEventHandlerMock := &MockEventHandler{}

	handlingErr := errors.New("handling message for topic test.test_topic")
	msg := &Message{Key: "key-1", Topic: "test.test_topic"}

	ec := NewEventConsumer(consumerMock, producerMock, crashTrackerMock)

	// Kill the consumer after 3 seconds.
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Second*3))
	defer cancel()

	crashTrackerMock.
		On("LogAndReportErrors", mock.Anything, mock.Anything, handlingErr.Error()).
		Return()

	// In the 3 seconds runtime, we're expecting call `ReadMessage` once.
	consumerMock.
		On("ReadMessage", ctx).Return(msg, nil).Once().
		On("Topic").Return("test.test_topic").
		On("Handlers").Return([]EventHandler{failedEventHandlerMock, successfulEventHandlerMock})

	// In the 3 seconds runtime, we're expecting to call `Handle` twice for `FailedEventHandler`.
	// 1 call triggered by the original message and one backoff.
	failedEventHandlerMock.
		On("Handle", ctx, msg).Return(handlingErr).Twice().
		On("CanHandleMessage", ctx, msg).Return(true).
		On("Name").Return("FailedEventHandler")

	// In the 3 seconds runtime, we're expecting to call `Handle` once for `SuccessfulEventHandler`.
	//  This is triggered by the original message. The backoff shouldn't trigger this handler.
	successfulEventHandlerMock.
		On("Handle", ctx, msg).Return(nil).Once().
		On("CanHandleMessage", ctx, msg).Return(true).
		On("Name").Return("SuccessfulEventHandler")

	// We expect the message to be re-broadcasted to the same topic when consumer gets interrupted by context deadline.
	producerMock.
		On("WriteMessages", ctx, mock.MatchedBy(func(m []Message) bool {
			if len(m) != 1 {
				return false
			}
			message := m[0]
			// Verify that there is only 1 successful message
			if len(message.SuccessfulExecutions) != 1 {
				return false
			}

			// Verify that one error is recorded correctly. in 3 seconds there should be one backoff.
			if len(message.Errors) != 1 && message.Errors[0].ErrorMessage != handlingErr.Error() {
				return false
			}

			// Verify that re-broadcasted message is the same as the one that failed
			return message.Key == msg.Key && message.Topic == msg.Topic
		})).
		Return(nil)

	getEntries := log.DefaultLogger.StartTest(log.InfoLevel)

	ec.Consume(ctx)

	entries := getEntries()
	assert.Equal(t, "Starting consuming messages for topic test.test_topic...", entries[0].Message)
	assert.Equal(t, log.InfoLevel, entries[0].Level)

	assert.Equal(t, "Reading message from topic test.test_topic...", entries[1].Message)
	assert.Equal(t, log.InfoLevel, entries[1].Level)

	// logged by mock handler
	assert.Equal(t, "Handling message with key key-1 by handler FailedEventHandler", entries[2].Message)
	assert.Equal(t, log.InfoLevel, entries[2].Level)

	// logged by mock handler
	assert.Equal(t, "Handling message with key key-1 by handler SuccessfulEventHandler", entries[3].Message)
	assert.Equal(t, log.InfoLevel, entries[3].Level)

	assert.Equal(t, "Waiting 2s before retrying handling message with key key-1", entries[4].Message)
	assert.Equal(t, log.WarnLevel, entries[4].Level)

	assert.Equal(t, "Retrying handling message with key key-1", entries[5].Message)
	assert.Equal(t, log.WarnLevel, entries[5].Level)
	assert.Equal(t, entries[4].Time.Truncate(time.Second).Add(time.Second*2), entries[5].Time.Truncate(time.Second))

	assert.Equal(t, "Handling message with key key-1 by handler FailedEventHandler", entries[6].Message)
	assert.Equal(t, log.InfoLevel, entries[6].Level)

	assert.Equal(t, "Handler SuccessfulEventHandler has already been executed for message with key key-1. Skipping...", entries[7].Message)
	assert.Equal(t, log.InfoLevel, entries[7].Level)

	assert.Equal(t, "Waiting 4s before retrying handling message with key key-1", entries[8].Message)
	assert.Equal(t, log.WarnLevel, entries[8].Level)

	assert.Equal(t, "Stopping consuming messages for topic test.test_topic due to context cancellation...", entries[9].Message)
	assert.Equal(t, log.InfoLevel, entries[9].Level)

	assert.Equal(t, "Replaying message with key key-1 to topic test.test_topic", entries[10].Message)
	assert.Equal(t, log.WarnLevel, entries[10].Level)

	consumerMock.AssertExpectations(t)
	crashTrackerMock.AssertExpectations(t)
	producerMock.AssertExpectations(t)
	successfulEventHandlerMock.AssertExpectations(t)
	failedEventHandlerMock.AssertExpectations(t)
}

func Test_EventConsumer_Consume_FinalizeConsumer(t *testing.T) {
	// setup mocks
	consumerMock := &MockConsumer{}
	crashTrackerMock := &crashtracker.MockCrashTrackerClient{}
	producerMock := &MockProducer{}
	failedEventHandlerMock := &MockEventHandler{}

	handlingErr := errors.New("handling message for topic test.test_topic")
	msg := &Message{Key: "key-1", Topic: "test.test_topic"}

	ec := NewEventConsumer(consumerMock, producerMock, crashTrackerMock)

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Second*1))
	defer cancel()

	crashTrackerMock.
		On("LogAndReportErrors", mock.Anything, mock.Anything, handlingErr.Error()).
		Return()

	consumerMock.
		On("Topic").Return("test.test_topic").
		On("ReadMessage", ctx).Return(msg, nil).
		On("Handlers").Return([]EventHandler{failedEventHandlerMock})

	failedEventHandlerMock.
		On("Handle", ctx, msg).Return(handlingErr).
		On("CanHandleMessage", ctx, msg).Return(true).
		On("Name").Return("FailedEventHandler")

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
	assert.Equal(t, "Waiting 2s before retrying handling message with key key-1", entries[0].Message)
	assert.Equal(t, "Replaying message with key key-1 to topic test.test_topic", entries[1].Message)

	consumerMock.AssertExpectations(t)
	crashTrackerMock.AssertExpectations(t)
	producerMock.AssertExpectations(t)
	failedEventHandlerMock.AssertExpectations(t)
}
