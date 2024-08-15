package message

import (
	"context"
	"fmt"

	"github.com/stellar/go/support/log"
)

type MessageChannel string

const (
	MessageChannelEmail MessageChannel = "EMAIL"
	MessageChannelSMS   MessageChannel = "SMS"
)

//go:generate mockery --name MessageDispatcherInterface --case=underscore --structname=MockMessageDispatcher --inpackage
type MessageDispatcherInterface interface {
	RegisterClient(ctx context.Context, channel MessageChannel, client MessengerClient)
	SendMessage(message Message, channel MessageChannel) error
	GetClient(channel MessageChannel) (MessengerClient, error)
}

type MessageDispatcher struct {
	clients map[MessageChannel]MessengerClient
}

func NewMessageDispatcher() *MessageDispatcher {
	return &MessageDispatcher{
		clients: make(map[MessageChannel]MessengerClient),
	}
}

func (d *MessageDispatcher) RegisterClient(ctx context.Context, channel MessageChannel, client MessengerClient) {
	log.Ctx(ctx).Infof("📡 [MessageDispatcher] Registering client %s for channel %s", client.MessengerType(), channel)
	d.clients[channel] = client
}

func (d *MessageDispatcher) SendMessage(message Message, channel MessageChannel) error {
	client, err := d.GetClient(channel)
	if err != nil {
		return fmt.Errorf("getting client for channel: %w", err)
	}

	return client.SendMessage(message)
}

func (d *MessageDispatcher) GetClient(channel MessageChannel) (MessengerClient, error) {
	client, ok := d.clients[channel]
	if !ok {
		return nil, fmt.Errorf("no client registered for channel %q", channel)
	}

	return client, nil
}

var _ MessageDispatcherInterface = &MessageDispatcher{}
