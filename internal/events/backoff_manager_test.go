package events

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_BackoffManager(t *testing.T) {
	backoffChan := make(chan struct{}, 1)
	backoffManager := NewBackoffManager(backoffChan)

	backoffManager.TriggerBackoff()
	<-backoffChan
	assert.Equal(t, time.Second*2, backoffManager.backoff)
	assert.Equal(t, time.Second*2, backoffManager.GetBackoffDuration())
	assert.Equal(t, 1, backoffManager.backoffCounter)

	backoffManager.ResetBackoff()
	assert.Equal(t, time.Duration(0), backoffManager.backoff)
	assert.Equal(t, time.Duration(0), backoffManager.GetBackoffDuration())
	assert.Equal(t, 0, backoffManager.backoffCounter)

	// Checking the MaxBackoffExponent constraint
	for i := 1; i <= MaxBackoffExponent+1; i++ {
		backoffManager.TriggerBackoff()
		<-backoffChan
		if i > MaxBackoffExponent {
			// It should the same of MaxBackoffExponent
			assert.Equal(t, time.Second*(1<<MaxBackoffExponent), backoffManager.GetBackoffDuration())
			assert.Equal(t, MaxBackoffExponent, backoffManager.backoffCounter)
		} else {
			assert.Equal(t, time.Second*(1<<i), backoffManager.GetBackoffDuration())
			assert.Equal(t, i, backoffManager.backoffCounter)
		}
	}
}
