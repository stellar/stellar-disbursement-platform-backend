package message

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_NewMessageDispatcher(t *testing.T) {
	dispatcher := NewMessageDispatcher()
	assert.NotNil(t, dispatcher)
	assert.Empty(t, dispatcher.clients)
}

func Test_MessageDispatcher_RegisterClient(t *testing.T) {
	ctx := context.Background()

	dispatcher := NewMessageDispatcher()
	client := NewMessengerClientMock(t)
	client.On("MessengerType").Return(MessengerTypeDryRun).Once()

	dispatcher.RegisterClient(ctx, MessageChannelEmail, client)

	assert.Len(t, dispatcher.clients, 1)
	assert.Equal(t, client, dispatcher.clients[MessageChannelEmail])
}

func Test_MessageDispatcher_GetClient(t *testing.T) {
	ctx := context.Background()
	dispatcher := NewMessageDispatcher()
	emailClient := NewMessengerClientMock(t)
	emailClient.On("MessengerType").Return(MessengerTypeDryRun).Once()
	dispatcher.RegisterClient(ctx, MessageChannelEmail, emailClient)

	tests := []struct {
		name        string
		channel     MessageChannel
		expected    MessengerClient
		expectedErr error
	}{
		{
			name:        "Existing Email client",
			channel:     MessageChannelEmail,
			expected:    emailClient,
			expectedErr: nil,
		},
		{
			name:        "Non-existing client",
			channel:     MessageChannelSMS,
			expected:    nil,
			expectedErr: errors.New("no client registered for channel \"SMS\""),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := dispatcher.GetClient(tt.channel)
			assert.Equal(t, tt.expected, result)
			if tt.expectedErr != nil {
				assert.EqualError(t, err, tt.expectedErr.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_MessageDispatcher_SendMessage(t *testing.T) {
	ctx := context.Background()

	dispatcher := NewMessageDispatcher()
	message := Message{}

	tests := []struct {
		name        string
		channel     MessageChannel
		setupMock   func(*MessengerClientMock)
		expectedErr error
	}{
		{
			name:    "Successful send",
			channel: MessageChannelEmail,
			setupMock: func(clientMock *MessengerClientMock) {
				clientMock.On("SendMessage", message).Return(nil)
			},
			expectedErr: nil,
		},
		{
			name:        "Client not found",
			channel:     MessageChannelSMS,
			setupMock:   func(clientMock *MessengerClientMock) {},
			expectedErr: errors.New("getting client for channel: no client registered for channel \"SMS\""),
		},
		{
			name:    "Client error",
			channel: MessageChannelEmail,
			setupMock: func(clientMock *MessengerClientMock) {
				clientMock.On("SendMessage", message).Return(errors.New("send error"))
			},
			expectedErr: errors.New("send error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewMessengerClientMock(t)
			client.On("MessengerType").Return(MessengerTypeDryRun).Once()
			dispatcher.RegisterClient(ctx, MessageChannelEmail, client)

			tt.setupMock(client)

			err := dispatcher.SendMessage(message, tt.channel)
			if tt.expectedErr != nil {
				assert.EqualError(t, err, tt.expectedErr.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
