package events

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/segmentio/kafka-go"
	"github.com/stellar/go/support/log"
	"golang.org/x/exp/maps"
)

type KafkaEventManager struct {
	handlers []EventHandler
	writer   *kafka.Writer
	reader   *kafka.Reader
}

func NewKafkaEventManager(brokers []string, consumerTopics []string, consumerGroupID string) (*KafkaEventManager, error) {
	k := KafkaEventManager{}

	writer := kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		Balancer:     &kafka.RoundRobin{},
		RequiredAcks: -1,
	}

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     brokers,
		GroupID:     consumerGroupID,
		GroupTopics: consumerTopics,
	})

	k.writer = &writer
	k.reader = reader

	return &k, nil
}

// Implements Producer interface
var _ Producer = new(KafkaEventManager)

// Implements Consumer interface
var _ Consumer = new(KafkaEventManager)

func (k *KafkaEventManager) WriteMessages(ctx context.Context, messages ...Message) error {
	kafkaMessages := make([]kafka.Message, 0, len(messages))
	for _, msg := range messages {
		msgJSON, err := json.Marshal(msg)
		if err != nil {
			return fmt.Errorf("marshalling message: %w", err)
		}

		kafkaMessages = append(kafkaMessages, kafka.Message{
			Topic: msg.Topic,
			Key:   []byte(msg.Key),
			Value: msgJSON,
		})
	}

	if err := k.writer.WriteMessages(ctx, kafkaMessages...); err != nil {
		log.Ctx(ctx).Errorf("writing message on kafka: %s", err.Error())
		return fmt.Errorf("writing message on kafka: %w", err)
	}

	return nil
}

func (k *KafkaEventManager) RegisterEventHandler(ctx context.Context, handlers ...EventHandler) error {
	ehMap := make(map[string]EventHandler, len(handlers))
	for _, handler := range handlers {
		log.Ctx(ctx).Infof("registering event handler %s", handler.Name())
		ehMap[handler.Name()] = handler
	}
	k.handlers = maps.Values(ehMap)
	return nil
}

func (k *KafkaEventManager) ReadMessage(ctx context.Context) error {
	log.Ctx(ctx).Info("fetching messages from kafka")
	kafkaMessage, err := k.reader.FetchMessage(ctx)
	if err != nil {
		return fmt.Errorf("fetching message from kafka: %w", err)
	}

	log.Ctx(ctx).Info("unmarshalling new message")
	var msg Message
	if err = json.Unmarshal(kafkaMessage.Value, &msg); err != nil {
		return fmt.Errorf("unmarshaling message: %w", err)
	}

	log.Ctx(ctx).Infof("new message being processed: %s", msg.String())
	for _, handler := range k.handlers {
		if handler.CanHandleMessage(ctx, &msg) {
			handler.Handle(ctx, &msg)
		}
	}

	// Acknowledgement
	if err = k.reader.CommitMessages(ctx, kafkaMessage); err != nil {
		return fmt.Errorf("committing message: %w", err)
	}

	return nil
}

func (k *KafkaEventManager) Close() error {
	log.Info("closing kafka producer and consumer")
	defer k.writer.Close()
	defer k.reader.Close()
	return nil
}
