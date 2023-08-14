package utils

import (
	"sync"
	"testing"
	"time"
)

// WaitUntilWaitGroupIsDoneOrTimeout is a helper function that waits for a wait group to finish or times out after a
// given duration. This is used for test purposes.
func WaitUntilWaitGroupIsDoneOrTimeout(t *testing.T, wg *sync.WaitGroup, timeout time.Duration, shouldTimeout bool, assertFn func()) {
	t.Helper()

	ch := make(chan struct{})
	go func() {
		wg.Wait()
		close(ch)
	}()

	select {
	case <-ch:
		if shouldTimeout {
			t.Fatal("wait group finished, but we expected it to timeout")
		} else {
			t.Log("wait group finished as expected")
		}
	case <-time.After(timeout):
		if shouldTimeout {
			t.Log("wait group correctly timed out")
		} else {
			t.Fatal("wait group did not finish within the expected time")
		}
	}

	if assertFn != nil {
		assertFn()
	}
}
