package events

import (
	"fmt"
	"strings"
)

type EventBrokerType string

const (
	KafkaEventBrokerType EventBrokerType = "KAFKA"
	// NoneEventBrokerType means that no event broker was chosen.
	NoneEventBrokerType      EventBrokerType = "NONE" // Deprecated: use SCHEDULER instead
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
