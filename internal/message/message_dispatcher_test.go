package message

import (
	"context"
	"errors"
	"fmt"
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
	emailMessage := Message{
		Title:   "Test Title",
		ToEmail: "mymail@stellar.org",
		Message: "Test Message",
	}

	smsMessage := Message{
		ToPhoneNumber: "+14152111111",
		Message:       "Test Message",
	}

	multiChannelMessage := Message{
		Title:         "Test Title",
		ToEmail:       "mymail@stellar.org",
		ToPhoneNumber: "+14152111111",
		Message:       "Test Message",
	}

	emptyMessage := Message{}

	tests := []struct {
		name                  string
		message               Message
		channelPriority       []MessageChannel
		supportedChannels     []MessageChannel
		setupMock             func(emailClientMock *MessengerClientMock, smsClientMock *MessengerClientMock)
		expectedMessengerType MessengerType
		expectedErr           error
	}{
		{
			name:              "fail when no supported channels",
			message:           emptyMessage,
			channelPriority:   []MessageChannel{MessageChannelEmail, MessageChannelSMS},
			supportedChannels: []MessageChannel{},
			setupMock:         func(emailClientMock *MessengerClientMock, smsClientMock *MessengerClientMock) {},
			expectedErr:       fmt.Errorf("no valid channel found for message %s", emptyMessage),
		},
		{
			name:              "fail when message with wrong format",
			message:           emailMessage,
			channelPriority:   []MessageChannel{MessageChannelSMS},
			supportedChannels: []MessageChannel{MessageChannelSMS},
			setupMock: func(emailClientMock *MessengerClientMock, smsClientMock *MessengerClientMock) {
				smsClientMock.AssertNotCalled(t, "SendMessage", emailMessage)
				emailClientMock.AssertNotCalled(t, "SendMessage", emailMessage)
			},
			expectedErr: fmt.Errorf("unable to send message %s using any of the supported channels [%v]", emailMessage, map[MessageChannel]bool{MessageChannelEmail: true}),
		},
		{
			name:              "successful when single supported channel (e-mail)",
			message:           emailMessage,
			channelPriority:   []MessageChannel{MessageChannelEmail, MessageChannelSMS},
			supportedChannels: []MessageChannel{MessageChannelEmail},
			setupMock: func(emailClientMock *MessengerClientMock, smsClientMock *MessengerClientMock) {
				emailClientMock.
					On("SendMessage", emailMessage).
					Return(nil).
					Once()

				smsClientMock.AssertNotCalled(t, "SendMessage", emailMessage)
			},
			expectedMessengerType: MessengerTypeAWSEmail,
			expectedErr:           nil,
		},
		{
			name:              "successful when single supported channel (sms)",
			message:           smsMessage,
			channelPriority:   []MessageChannel{MessageChannelEmail, MessageChannelSMS},
			supportedChannels: []MessageChannel{MessageChannelSMS},
			setupMock: func(emailClientMock *MessengerClientMock, smsClientMock *MessengerClientMock) {
				smsClientMock.
					On("SendMessage", smsMessage).
					Return(nil).
					Once()

				emailClientMock.AssertNotCalled(t, "SendMessage", smsMessage)
			},
			expectedMessengerType: MessengerTypeTwilioSMS,
			expectedErr:           nil,
		},
		{
			name:              "successful when multiple supported channels",
			message:           multiChannelMessage,
			channelPriority:   []MessageChannel{MessageChannelSMS, MessageChannelEmail},
			supportedChannels: []MessageChannel{MessageChannelEmail, MessageChannelSMS},
			setupMock: func(emailClientMock *MessengerClientMock, smsClientMock *MessengerClientMock) {
				smsClientMock.
					On("SendMessage", multiChannelMessage).
					Return(nil).
					Once()

				emailClientMock.AssertNotCalled(t, "SendMessage", multiChannelMessage)
			},
			expectedMessengerType: MessengerTypeTwilioSMS,
			expectedErr:           nil,
		},
		{
			name:              "successful when first channel fails (sms) but second succeeds (e-mail)",
			message:           multiChannelMessage,
			channelPriority:   []MessageChannel{MessageChannelSMS, MessageChannelEmail},
			supportedChannels: []MessageChannel{MessageChannelSMS, MessageChannelEmail},
			setupMock: func(emailClientMock *MessengerClientMock, smsClientMock *MessengerClientMock) {
				smsClientMock.
					On("SendMessage", multiChannelMessage).
					Return(errors.New("send error")).
					Once()

				emailClientMock.
					On("SendMessage", multiChannelMessage).
					Return(nil).
					Once()
			},
			expectedMessengerType: MessengerTypeAWSEmail,
			expectedErr:           nil,
		},
		{
			name:              "fail when all channels fail",
			message:           multiChannelMessage,
			channelPriority:   []MessageChannel{MessageChannelSMS, MessageChannelEmail},
			supportedChannels: []MessageChannel{MessageChannelSMS, MessageChannelEmail},
			setupMock: func(emailClientMock *MessengerClientMock, smsClientMock *MessengerClientMock) {
				emailClientMock.
					On("SendMessage", multiChannelMessage).
					Return(errors.New("send error")).
					Once()

				smsClientMock.
					On("SendMessage", multiChannelMessage).
					Return(errors.New("send error")).
					Once()
			},
			expectedErr: fmt.Errorf("unable to send message %s using any of the supported channels [%v]", multiChannelMessage, map[MessageChannel]bool{MessageChannelEmail: true, MessageChannelSMS: true}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			dispatcher := NewMessageDispatcher()

			emailClient := NewMessengerClientMock(t)
			emailClient.On("MessengerType").Return(MessengerTypeAWSEmail).Maybe()
			dispatcher.RegisterClient(ctx, MessageChannelEmail, emailClient)

			smsClient := NewMessengerClientMock(t)
			smsClient.On("MessengerType").Return(MessengerTypeTwilioSMS).Maybe()
			dispatcher.RegisterClient(ctx, MessageChannelSMS, smsClient)

			tt.setupMock(emailClient, smsClient)

			messengerType, err := dispatcher.SendMessage(ctx, tt.message, tt.channelPriority)
			if tt.expectedErr != nil {
				assert.EqualError(t, err, tt.expectedErr.Error())
			} else {
				assert.Equal(t, tt.expectedMessengerType, messengerType)
				assert.NoError(t, err)
			}
		})
	}
}
