package events

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_BackoffManager_TriggerBackoff(t *testing.T) {
	backoffChan := make(chan struct{}, 1)
	maxBackoff := DefaultMaxBackoffExponent
	backoffManager := NewBackoffManager(backoffChan, maxBackoff)

	backoffManager.TriggerBackoff()
	<-backoffChan
	assert.Equal(t, time.Second*2, backoffManager.backoff)
	assert.Equal(t, time.Second*2, backoffManager.GetBackoffDuration())
	assert.Equal(t, 1, backoffManager.backoffCounter)

	backoffManager.ResetBackoff()
	assert.Equal(t, time.Duration(0), backoffManager.backoff)
	assert.Equal(t, time.Duration(0), backoffManager.GetBackoffDuration())
	assert.Equal(t, 0, backoffManager.backoffCounter)

	// Checking the DefaultMaxBackoffExponent constraint
	for i := 1; i <= maxBackoff+1; i++ {
		backoffManager.TriggerBackoff()
		<-backoffChan
		if i > maxBackoff {
			// It should the same of DefaultMaxBackoffExponent
			assert.Equal(t, time.Second*(1<<maxBackoff), backoffManager.GetBackoffDuration())
			assert.Equal(t, maxBackoff, backoffManager.backoffCounter)
		} else {
			assert.Equal(t, time.Second*(1<<i), backoffManager.GetBackoffDuration())
			assert.Equal(t, i, backoffManager.backoffCounter)
		}
	}
}

func Test_BackoffManager_TriggerBackoffWithMessage(t *testing.T) {
	backoffChan := make(chan struct{}, 1)
	maxBackoff := DefaultMaxBackoffExponent
	backoffManager := NewBackoffManager(backoffChan, maxBackoff)

	msg := &Message{}
	backoffError := errors.New("temporary network failure")

	// Trigger backoff with the message.
	msg.RecordError("test-handler", backoffError)
	backoffManager.TriggerBackoffWithMessage(msg)
	<-backoffChan

	// checking backoff behaviour.
	assert.Equal(t, 1, backoffManager.backoffCounter)
	assert.Equal(t, time.Second*2, backoffManager.GetBackoffDuration())
	assert.Equal(t, 1, len(backoffManager.GetMessage().Errors))
	assert.Equal(t, backoffError.Error(), backoffManager.GetMessage().Errors[0].ErrorMessage)
	assert.Equal(t, "test-handler", backoffManager.GetMessage().Errors[0].HandlerName)
	assert.Equal(t, backoffError, backoffManager.GetMessage().Errors[0].Err)

	// checking reset backoff.
	backoffManager.ResetBackoff()
	assert.Equal(t, time.Duration(0), backoffManager.backoff)
	assert.Equal(t, time.Duration(0), backoffManager.GetBackoffDuration())
	assert.Equal(t, 0, backoffManager.backoffCounter)
	assert.Nil(t, backoffManager.GetMessage())

	// checking backoff calculations and errors
	for i := 1; i <= maxBackoff+1; i++ {
		msg.RecordError("test-handler", backoffError)
		backoffManager.TriggerBackoffWithMessage(msg)
		<-backoffChan
		if i >= maxBackoff {
			assert.Equal(t, time.Second*(1<<maxBackoff), backoffManager.GetBackoffDuration())
			assert.Equal(t, maxBackoff, backoffManager.backoffCounter)
			assert.True(t, backoffManager.IsMaxBackoffReached())
		} else {
			assert.Equal(t, time.Second*(1<<i), backoffManager.GetBackoffDuration())
			assert.Equal(t, i, backoffManager.backoffCounter)
			assert.False(t, backoffManager.IsMaxBackoffReached())
		}
		assert.Equal(t, i+1, len(backoffManager.GetMessage().Errors))
	}
}
