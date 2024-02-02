package events

import (
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

const MaxBackoffExponent = 8

type ConsumerBackoffManager struct {
	backoffCounter int
	backoff        time.Duration
	backoffChan    chan<- struct{}
}

func NewBackoffManager(backoffChan chan<- struct{}) *ConsumerBackoffManager {
	return &ConsumerBackoffManager{
		backoffChan: backoffChan,
	}
}

func (bm *ConsumerBackoffManager) TriggerBackoff() {
	bm.backoffCounter++
	if bm.backoffCounter > MaxBackoffExponent {
		bm.backoffCounter = MaxBackoffExponent
	}
	// No need to handle this error since it only returns error when retry > 32, < 0
	bm.backoff, _ = utils.ExponentialBackoffInSeconds(bm.backoffCounter)
	bm.backoffChan <- struct{}{}
}

func (bm *ConsumerBackoffManager) GetBackoffDuration() time.Duration {
	return bm.backoff
}

func (bm *ConsumerBackoffManager) ResetBackoff() {
	bm.backoffCounter = 0
	bm.backoff = 0
}
