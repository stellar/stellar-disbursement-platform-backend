package events

import (
	"fmt"
	"strings"
)

type EventBrokerType string

const (
	KafkaEventBrokerType EventBrokerType = "KAFKA"
	// NoneEventBrokerType means that no event broker was chosen.
	// Deprecated: use SCHEDULER instead
	NoneEventBrokerType      EventBrokerType = "NONE"
	SchedulerEventBrokerType EventBrokerType = "SCHEDULER"
)

func ParseEventBrokerType(ebType string) (EventBrokerType, error) {
	switch EventBrokerType(strings.ToUpper(ebType)) {
	case KafkaEventBrokerType:
		return KafkaEventBrokerType, nil
	case NoneEventBrokerType:
		return NoneEventBrokerType, nil
	case SchedulerEventBrokerType:
		return SchedulerEventBrokerType, nil
	default:
		return "", fmt.Errorf("invalid event broker type")
	}
}

type EventReceiverWalletSMSInvitationData struct {
	ReceiverWalletID string `json:"id"`
}

type EventPatchAnchorPlatformTransactionCompletionData struct {
	PaymentID string `json:"payment_id"`
}
