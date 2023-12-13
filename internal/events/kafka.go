package events

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/segmentio/kafka-go"
	"github.com/stellar/go/support/log"
	"golang.org/x/exp/maps"
)

type KafkaProducer struct {
	writer *kafka.Writer
}

// Implements Producer interface
var _ Producer = new(KafkaProducer)

func NewKafkaProducer(brokers []string) *KafkaProducer {
	k := KafkaProducer{}

	k.writer = &kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		Balancer:     &kafka.RoundRobin{},
		RequiredAcks: -1,
	}

	return &k
}

func (k *KafkaProducer) WriteMessages(ctx context.Context, messages ...Message) error {
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

func (k *KafkaProducer) Close() error {
	log.Info("closing kafka producer")
	return k.writer.Close()
}

type KafkaConsumer struct {
	handlers []EventHandler
	reader   *kafka.Reader
}

// Implements Consumer interface
var _ Consumer = new(KafkaConsumer)

func NewKafkaConsumer(brokers []string, topic string, consumerGroupID string) *KafkaConsumer {
	k := KafkaConsumer{}

	k.reader = kafka.NewReader(kafka.ReaderConfig{
		Brokers: brokers,
		Topic:   topic,
		GroupID: consumerGroupID,
	})

	return &k
}

func (k *KafkaConsumer) RegisterEventHandler(ctx context.Context, handlers ...EventHandler) error {
	ehMap := make(map[string]EventHandler, len(handlers))
	for _, handler := range handlers {
		log.Ctx(ctx).Infof("registering event handler %s", handler.Name())
		ehMap[handler.Name()] = handler
	}
	k.handlers = maps.Values(ehMap)
	return nil
}

func (k *KafkaConsumer) ReadMessage(ctx context.Context) error {
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

func (k *KafkaConsumer) Close() error {
	log.Info("closing kafka consumer")
	return k.reader.Close()
}
