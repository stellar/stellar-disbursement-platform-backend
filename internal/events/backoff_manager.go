package events

import (
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

const DefaultMaxBackoffExponent = 8

type ConsumerBackoffManager struct {
	maxBackoff     int
	backoffCounter int
	backoff        time.Duration
	backoffChan    chan<- struct{}
	message        *Message
}

func NewBackoffManager(backoffChan chan<- struct{}, maxBackoff int) *ConsumerBackoffManager {
	return &ConsumerBackoffManager{
		backoffChan: backoffChan,
		maxBackoff:  maxBackoff,
	}
}

func (bm *ConsumerBackoffManager) TriggerBackoffWithMessage(msg *Message, hErr *HandlerError) {
	if msg != nil {
		if hErr != nil {
			msg.RecordError(hErr)
		}
		bm.message = msg
	}
	bm.TriggerBackoff()
}

func (bm *ConsumerBackoffManager) TriggerBackoff() {
	bm.backoffCounter++
	if bm.backoffCounter > bm.maxBackoff {
		bm.backoffCounter = bm.maxBackoff
	}

	// No need to handle this error since it only returns error when retry > 32, < 0
	bm.backoff, _ = utils.ExponentialBackoffInSeconds(bm.backoffCounter)

	bm.backoffChan <- struct{}{}
}

func (bm *ConsumerBackoffManager) IsMaxBackoffReached() bool {
	return bm.backoffCounter >= bm.maxBackoff
}

func (bm *ConsumerBackoffManager) GetBackoffDuration() time.Duration {
	return bm.backoff
}

func (bm *ConsumerBackoffManager) ResetBackoff() {
	bm.backoffCounter = 0
	bm.backoff = 0
	bm.message = nil
}

func (bm *ConsumerBackoffManager) GetMessage() *Message {
	return bm.message
}
