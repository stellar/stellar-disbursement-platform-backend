package cmd

import (
	"fmt"
	"go/types"

	"github.com/spf13/cobra"
	"github.com/stellar/go/support/config"
	"github.com/stellar/go/support/log"
	cmdUtils "github.com/stellar/stellar-disbursement-platform-backend/cmd/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
)

type MessageCommand struct{}

type MessengerServiceInterface interface {
	GetClient(opts message.MessengerOptions) (message.MessengerClient, error)
	SendMessage(opts message.MessengerOptions, message message.Message) error
}

type MessengerService struct{}

func (m *MessengerService) GetClient(opts message.MessengerOptions) (message.MessengerClient, error) {
	return message.GetClient(opts)
}

func (m *MessengerService) SendMessage(opts message.MessengerOptions, message message.Message) error {
	messengerClient, err := m.GetClient(opts)
	if err != nil {
		return fmt.Errorf("getting messenger client: %w", err)
	}

	return messengerClient.SendMessage(message)
}

func (s *MessageCommand) Command(messengerService MessengerServiceInterface) *cobra.Command {
	opts := message.MessengerOptions{}
	messageCmdConfigOpts := config.ConfigOptions{
		// message sender type
		{
			Name:           "message-sender-type",
			Usage:          `Message Sender Type. Options: "TWILIO_SMS", "AWS_SMS", "AWS_EMAIL", "DRY_RUN"`,
			OptType:        types.String,
			CustomSetValue: cmdUtils.SetConfigOptionMessengerType,
			ConfigKey:      &opts.MessengerType,
			Required:       true,
		},
	}
	messageCmdConfigOpts = append(messageCmdConfigOpts, cmdUtils.TwilioConfigOptions(&opts)...)
	messageCmdConfigOpts = append(messageCmdConfigOpts, cmdUtils.AWSConfigOptions(&opts)...)

	messageCmd := &cobra.Command{
		Use:   "message",
		Short: "Messenger related commands",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			cmd.Parent().PersistentPreRun(cmd.Parent(), args)
			// Inject dependencies:
			opts.Environment = globalOptions.environment

			// Validate & ingest input parameters
			messageCmdConfigOpts.Require()
			err := messageCmdConfigOpts.SetValues()
			if err != nil {
				log.Fatalf("Error setting values of config options: %s", err.Error())
			}
		},
		Run: func(cmd *cobra.Command, _ []string) {
			_, err := messengerService.GetClient(opts)
			if err != nil {
				log.Fatalf("Error calling help command: %s", err.Error())
			}

			log.Infof("🎉 Successfully mounted messenger client for type %s", opts.MessengerType)
		},
	}
	err := messageCmdConfigOpts.Init(messageCmd)
	if err != nil {
		log.Fatalf("Error initializing messageCmd config option: %s", err.Error())
	}

	sendMessageCmd := s.sendMessageCommand(messengerService, &opts)
	messageCmd.AddCommand(sendMessageCmd)

	return messageCmd
}

func (s *MessageCommand) sendMessageCommand(messengerService MessengerServiceInterface, messageOptions *message.MessengerOptions) *cobra.Command {
	msg := message.Message{}
	// CLI arguments to send a message
	sendMessageCmdConfigOpts := config.ConfigOptions{
		{
			Name:      "phone-number",
			Usage:     "The phone number to send the message to, in E.164. Mandatory if sending an SMS",
			OptType:   types.String,
			ConfigKey: &msg.ToPhoneNumber,
			Required:  false,
		},
		{
			Name:      "email",
			Usage:     "The email to send the message to. Mandatory if sending an email.",
			OptType:   types.String,
			ConfigKey: &msg.ToEmail,
			Required:  false,
		},
		{
			Name:      "title",
			Usage:     "The title to be set in the email. Mandatory if sending an email.",
			OptType:   types.String,
			ConfigKey: &msg.Title,
			Required:  false,
		},
		{
			Name:      "message",
			Usage:     "The text of the message to be sent",
			OptType:   types.String,
			ConfigKey: &msg.Message,
			Required:  true,
		},
	}
	sendMessageCmd := &cobra.Command{
		Use:   "send",
		Short: "Send a message",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			cmd.Parent().PersistentPreRun(cmd.Parent(), args)

			// Validate & ingest input parameters
			sendMessageCmdConfigOpts.Require()
			err := sendMessageCmdConfigOpts.SetValues()
			if err != nil {
				log.Fatalf("Error setting values of config options: %s", err.Error())
			}
		},
		Run: func(_ *cobra.Command, _ []string) {
			err := messengerService.SendMessage(*messageOptions, msg)
			if err != nil {
				log.Fatalf("Error sending message: %s", err.Error())
			}
		},
	}
	err := sendMessageCmdConfigOpts.Init(sendMessageCmd)
	if err != nil {
		log.Fatalf("Error initializing a sendMessageCmd option: %s", err.Error())
	}

	return sendMessageCmd
}
