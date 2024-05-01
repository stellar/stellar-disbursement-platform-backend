package events

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl/plain"
	"github.com/stellar/go/support/log"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
)

type KafkaSecurityProtocol string

const (
	KafkaProtocolPlaintext     KafkaSecurityProtocol = "PLAINTEXT"
	KafkaProtocolSASLPlaintext KafkaSecurityProtocol = "SASL_PLAINTEXT"
	KafkaProtocolSASLSSL       KafkaSecurityProtocol = "SASL_SSL"
	KafkaProtocolSSL           KafkaSecurityProtocol = "SSL"
)

var (
	SASLProtocols = []KafkaSecurityProtocol{KafkaProtocolSASLPlaintext, KafkaProtocolSASLSSL}
	SSLProtocols  = []KafkaSecurityProtocol{KafkaProtocolSASLSSL, KafkaProtocolSSL}
)

func ParseKafkaSecurityProtocol(protocol string) (KafkaSecurityProtocol, error) {
	protocol = strings.ToUpper(protocol)
	switch KafkaSecurityProtocol(protocol) {
	case KafkaProtocolPlaintext, KafkaProtocolSASLPlaintext, KafkaProtocolSASLSSL, KafkaProtocolSSL:
		return KafkaSecurityProtocol(protocol), nil
	default:
		return "", fmt.Errorf("invalid kafka security protocol: %s", protocol)
	}
}

type KafkaConfig struct {
	Brokers              []string
	SecurityProtocol     KafkaSecurityProtocol
	SASLUsername         string
	SASLPassword         string
	SSLAccessKey         string
	SSLAccessCertificate string
}

func (kc *KafkaConfig) Validate() error {
	if len(kc.Brokers) == 0 {
		return fmt.Errorf("brokers cannot be empty")
	}

	if kc.SecurityProtocol == "" {
		return fmt.Errorf("security protocol cannot be empty")
	}

	if slices.Contains(SASLProtocols, kc.SecurityProtocol) {
		if kc.SASLUsername == "" || kc.SASLPassword == "" {
			return fmt.Errorf("SASL credentials must be provided for SASL_PLAINTEXT and SASL_SSL protocols")
		}
	}

	// Specific validation for the SSL
	if kc.SecurityProtocol == KafkaProtocolSSL {
		if _, err := tls.X509KeyPair([]byte(kc.SSLAccessCertificate), []byte(kc.SSLAccessKey)); err != nil {
			return fmt.Errorf("validating Kafka SSL Access Key/Certificate: %w", err)
		}
	}

	return nil
}

type KafkaProducer struct {
	writer *kafka.Writer
}

// Implements Producer interface
var _ Producer = new(KafkaProducer)

func NewKafkaProducer(config KafkaConfig) (*KafkaProducer, error) {
	k := KafkaProducer{}

	err := config.Validate()
	if err != nil {
		return nil, fmt.Errorf("invalid kafka config: %w", err)
	}

	var tlsConfig *tls.Config
	transport := kafka.DefaultTransport
	if slices.Contains(SSLProtocols, config.SecurityProtocol) {
		tlsConfig = &tls.Config{}
		if config.SecurityProtocol == KafkaProtocolSSL {
			cert, err := tls.X509KeyPair([]byte(config.SSLAccessCertificate), []byte(config.SSLAccessKey))
			if err != nil {
				return nil, fmt.Errorf("parsing SSL access key and certificate: %w", err)
			}
			tlsConfig.Certificates = []tls.Certificate{cert}
		}

		transport = &kafka.Transport{
			TLS: tlsConfig,
		}
	}

	if config.SecurityProtocol == KafkaProtocolSASLPlaintext || config.SecurityProtocol == KafkaProtocolSASLSSL {
		transport = &kafka.Transport{
			SASL: plain.Mechanism{
				Username: config.SASLUsername,
				Password: config.SASLPassword,
			},
			TLS: tlsConfig,
		}
	}

	k.writer = &kafka.Writer{
		Addr:         kafka.TCP(config.Brokers...),
		Balancer:     &kafka.RoundRobin{},
		RequiredAcks: -1,
		Transport:    transport,
	}

	return &k, nil
}

func (k *KafkaProducer) WriteMessages(ctx context.Context, messages ...Message) error {
	kafkaMessages := make([]kafka.Message, 0, len(messages))
	for _, msg := range messages {
		if err := msg.Validate(); err != nil {
			return fmt.Errorf("invalid message: %w", err)
		}

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
		log.Ctx(ctx).Errorf("writing message on kafka for topic %s: %s", k.writer.Topic, err.Error())
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

func NewKafkaConsumer(config KafkaConfig, topic string, consumerGroupID string, handlers ...EventHandler) (*KafkaConsumer, error) {
	k := KafkaConsumer{}

	err := config.Validate()
	if err != nil {
		return nil, fmt.Errorf("invalid kafka config: %w", err)
	}

	var tlsConfig *tls.Config
	dialer := kafka.DefaultDialer
	if slices.Contains(SSLProtocols, config.SecurityProtocol) {
		tlsConfig = &tls.Config{}
		if config.SecurityProtocol == KafkaProtocolSSL {
			cert, err := tls.X509KeyPair([]byte(config.SSLAccessCertificate), []byte(config.SSLAccessKey))
			if err != nil {
				return nil, fmt.Errorf("parsing SSL access key and certificate: %w", err)
			}
			tlsConfig.Certificates = []tls.Certificate{cert}
		}

		dialer = &kafka.Dialer{
			TLS: tlsConfig,
		}
	}

	if config.SecurityProtocol == KafkaProtocolSASLPlaintext || config.SecurityProtocol == KafkaProtocolSASLSSL {
		dialer = &kafka.Dialer{
			SASLMechanism: plain.Mechanism{
				Username: config.SASLUsername,
				Password: config.SASLPassword,
			},
			TLS: tlsConfig,
		}
	}

	k.reader = kafka.NewReader(kafka.ReaderConfig{
		Brokers: config.Brokers,
		Topic:   topic,
		GroupID: consumerGroupID,
		Dialer:  dialer,
	})

	if len(handlers) == 0 {
		return nil, fmt.Errorf("handlers cannot be empty")
	}

	ehMap := make(map[string]EventHandler)
	for _, handler := range handlers {
		log.Infof("registering event handler %s for topic %s", handler.Name(), topic)
		ehMap[handler.Name()] = handler
	}
	k.handlers = maps.Values(ehMap)

	return &k, nil
}

func (k *KafkaConsumer) ReadMessage(ctx context.Context) error {
	log.Ctx(ctx).Infof("fetching messages from kafka for topic %s", k.reader.Config().Topic)
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
			handleErr := handler.Handle(ctx, &msg)
			if handleErr != nil {
				return fmt.Errorf("handling message: %w", handleErr)
			}
		}
	}

	// Acknowledgement
	if err = k.reader.CommitMessages(ctx, kafkaMessage); err != nil {
		return fmt.Errorf("committing message: %w", err)
	}

	return nil
}

func (k *KafkaConsumer) Topic() string {
	return k.reader.Config().Topic
}

func (k *KafkaConsumer) Close() error {
	log.Info("closing kafka consumer")
	return k.reader.Close()
}
