package dependencyinjection

import (
	"context"
	"fmt"

	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
)

const kafkaEventManagerInstanceName = "kafka_event_manager_instance_name"

func NewKafkaEventManager(ctx context.Context, brokers []string, consumerTopics []string, consumerGroupID string, eventHandlers ...events.EventHandler) (*events.KafkaEventManager, error) {
	if len(brokers) == 0 {
		return nil, fmt.Errorf("brokers cannot be empty")
	}

	if len(consumerTopics) == 0 {
		return nil, fmt.Errorf("consumer topics cannot be empty")
	}

	if consumerGroupID == "" {
		return nil, fmt.Errorf("consumer group ID cannot be empty")
	}

	if instance, ok := dependenciesStoreMap[kafkaEventManagerInstanceName]; ok {
		if kafkaEventManager, ok := instance.(*events.KafkaEventManager); ok {
			return kafkaEventManager, nil
		}
		return nil, fmt.Errorf("trying to cast pre-existing Kafka Event Manager for dependency injection")
	}

	// Setup Kafka Event Manager
	log.Infof("⚙️ Setting Kafka Event Manager")
	kafkaEventManager, err := events.NewKafkaEventManager(brokers, consumerTopics, consumerGroupID)
	if err != nil {
		return nil, fmt.Errorf("creating Kafka Event Manager: %w", err)
	}

	err = kafkaEventManager.RegisterEventHandler(ctx, eventHandlers...)
	if err != nil {
		return nil, fmt.Errorf("registering event handlers: %w", err)
	}

	setInstance(kafkaEventManagerInstanceName, kafkaEventManager)

	return kafkaEventManager, nil
}
