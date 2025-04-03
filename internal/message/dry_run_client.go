package message

import (
	"context"
	"fmt"
	"strings"
)

type dryRunClient struct{}

func (c *dryRunClient) SendMessage(_ context.Context, message Message) error {
	recipient := message.ToEmail
	if message.ToEmail == "" {
		recipient = message.ToPhoneNumber
	}

	fmt.Println(strings.Repeat("-", 79))
	fmt.Println("Recipient:", recipient)
	fmt.Println("Subject:", message.Title)
	fmt.Println("Content:", message.Body)
	fmt.Println(strings.Repeat("-", 79))

	return nil
}

func (c *dryRunClient) MessengerType() MessengerType {
	return MessengerTypeDryRun
}

func NewDryRunClient() (MessengerClient, error) {
	return &dryRunClient{}, nil
}
