package events

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConsume(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Second*8))
	defer cancel()
	consumer := &MockConsumer{}
	crashTracker := &crashtracker.MockCrashTrackerClient{}

	unexpectedErr := errors.New("unexpected error")
	consumer.
		On("Topic").
		Return("test.test_topic").
		On("ReadMessage", ctx).
		Return(unexpectedErr).
		Twice().
		On("ReadMessage", ctx).
		Return(nil).
		Once().
		On("ReadMessage", ctx).
		Return(unexpectedErr).
		Once().
		On("ReadMessage", ctx).
		Return(nil)

	crashTracker.
		On("LogAndReportErrors", ctx, unexpectedErr, "consuming messages for topic test.test_topic").
		Return().
		Times(3)

	t.Log("calling Consume function...")
	getEntries := log.DefaultLogger.StartTest(log.WarnLevel)
	Consume(ctx, consumer, crashTracker)

	entries := getEntries()
	require.Len(t, entries, 3)
	assert.Equal(t, "Waiting 2s before retrying reading new messages", entries[0].Message)
	assert.Equal(t, "Waiting 4s before retrying reading new messages", entries[1].Message)
	assert.Equal(t, "Waiting 2s before retrying reading new messages", entries[2].Message) // backoffManager.ResetBackoff() was called

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
