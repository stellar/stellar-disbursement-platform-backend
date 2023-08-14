package message

import (
	"fmt"
	"strings"
)

type dryRunClient struct{}

func (c *dryRunClient) SendMessage(message Message) error {
	recipient := message.ToEmail
	if message.ToEmail == "" {
		recipient = message.ToPhoneNumber
	}

	fmt.Println(strings.Repeat("-", 79))
	fmt.Println("Recipient:", recipient)
	fmt.Println("Subject:", message.Title)
	fmt.Println("Content:", message.Message)
	fmt.Println(strings.Repeat("-", 79))

	return nil
}

func (c *dryRunClient) MessengerType() MessengerType {
	return MessengerTypeDryRun
}

func NewDryRunClient() (MessengerClient, error) {
	return &dryRunClient{}, nil
}
