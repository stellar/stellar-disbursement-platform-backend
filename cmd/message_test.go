package cmd

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
	cmdUtils "github.com/stellar/stellar-disbursement-platform-backend/cmd/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockMessengerService struct {
	mock.Mock
}

func (m *mockMessengerService) GetClient(opts message.MessengerOptions) (message.MessengerClient, error) {
	args := m.Called(opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(message.MessengerClient), args.Error(1)
}

func (m *mockMessengerService) SendMessage(opts message.MessengerOptions, message message.Message) error {
	return m.Called(opts, message).Error(0)
}

func Test_message_help(t *testing.T) {
	// setup
	var out bytes.Buffer
	rootCmd := SetupCLI("x.y.z", "1234567890abcdef")
	messageCmdFound := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "message" {
			messageCmdFound = true
		}
	}
	require.True(t, messageCmdFound, "message command not found")
	rootCmd.SetArgs([]string{"message", "--help"})
	rootCmd.SetOut(&out)

	// test
	err := rootCmd.Execute()
	require.NoError(t, err)

	// assert
	assert.Contains(t, out.String(), "stellar-disbursement-platform message [flags]", "should have printed help message for message command")
}

func Test_message_GetClient_wasCalled(t *testing.T) {
	cmdUtils.ClearTestEnvironment(t)

	mMessageService := mockMessengerService{}
	wantMessageOptions := message.MessengerOptions{
		MessengerType: message.MessengerTypeTwilioSMS,
		Environment:   "development",
	}
	mMessageService.On("GetClient", wantMessageOptions).Return(nil, nil).Once()

	// setup
	rootCmd := SetupCLI("x.y.z", "1234567890abcdef")
	var commandToRemove *cobra.Command
	commandToAdd := (&MessageCommand{}).Command(&mMessageService)
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "message" {
			commandToRemove = cmd
		}
	}
	require.NotNil(t, commandToRemove, "message command not found")
	rootCmd.RemoveCommand(commandToRemove)
	rootCmd.AddCommand(commandToAdd)
	rootCmd.SetArgs([]string{"message", "--message-sender-type", "twilio_sms"})

	// test
	err := rootCmd.Execute()
	require.NoError(t, err)

	// assert
	mMessageService.AssertExpectations(t)
}

func Test_message_send_SendMessage_wasCalled(t *testing.T) {
	cmdUtils.ClearTestEnvironment(t)

	mMessageService := mockMessengerService{}
	wantMessageOptions := message.MessengerOptions{
		MessengerType: message.MessengerTypeTwilioSMS,
		Environment:   "development",
	}
	wantMessage := message.Message{
		ToPhoneNumber: "+41555511111",
		Message:       "hello world",
	}
	mMessageService.On("SendMessage", wantMessageOptions, wantMessage).Return(nil).Once()

	// setup
	rootCmd := SetupCLI("x.y.z", "1234567890abcdef")
	var commandToRemove *cobra.Command
	commandToAdd := (&MessageCommand{}).Command(&mMessageService)
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "message" {
			commandToRemove = cmd
		}
	}
	require.NotNil(t, commandToRemove, "message command not found")
	rootCmd.RemoveCommand(commandToRemove)
	rootCmd.AddCommand(commandToAdd)
	rootCmd.SetArgs([]string{"message", "send", "--message-sender-type", "twilio_SMS", "--phone-number", "+41555511111", "--message", "hello world"})

	// test
	err := rootCmd.Execute()
	require.NoError(t, err)

	// assert
	mMessageService.AssertExpectations(t)
}
