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
	SendMessage(ctx context.Context, message Message, channelPriority []MessageChannel) error
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

func (d *MessageDispatcher) SendMessage(ctx context.Context, message Message, channelPriority []MessageChannel) error {
	supportedChannels := make(map[MessageChannel]bool)
	for _, ch := range message.SupportedChannels() {
		supportedChannels[ch] = true
	}

	if len(supportedChannels) == 0 {
		return fmt.Errorf("no valid channel found for message %s", message)
	}

	for _, channel := range channelPriority {
		if !supportedChannels[channel] {
			log.Ctx(ctx).Debugf("Channel %q is not supported for the message %s", channel, message)
			continue
		}

		client, ok := d.clients[channel]
		if !ok {
			log.Ctx(ctx).Warnf("No client registered for channel %q", channel)
			continue
		}

		err := client.SendMessage(message)
		if err == nil {
			return nil
		}

		log.Ctx(ctx).Errorf("Error sending message %s using channel %q: %v", message, channel, err)
	}

	return fmt.Errorf("unable to send message %s using any of the supported channels [%v]", message, supportedChannels)
}

func (d *MessageDispatcher) GetClient(channel MessageChannel) (MessengerClient, error) {
	client, ok := d.clients[channel]
	if !ok {
		return nil, fmt.Errorf("no client registered for channel %q", channel)
	}

	return client, nil
}

var _ MessageDispatcherInterface = &MessageDispatcher{}
