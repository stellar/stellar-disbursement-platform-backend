package dependencyinjection

import (
	"context"
	"fmt"

	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
)

const kafkaProducerInstanceName = "kafka_producer_instance_name"

func NewKafkaProducer(ctx context.Context, brokers []string) (*events.KafkaProducer, error) {
	if len(brokers) == 0 {
		return nil, fmt.Errorf("brokers cannot be empty")
	}

	if instance, ok := dependenciesStoreMap[kafkaProducerInstanceName]; ok {
		if kafkaProducer, ok := instance.(*events.KafkaProducer); ok {
			return kafkaProducer, nil
		}
		return nil, fmt.Errorf("trying to cast pre-existing Kafka Producer for dependency injection")
	}

	// Setup Kafka Event Manager
	log.Infof("⚙️ Setting Kafka Producer")
	kafkaProducer := events.NewKafkaProducer(brokers)
	setInstance(kafkaProducerInstanceName, kafkaProducer)
	return kafkaProducer, nil
}
