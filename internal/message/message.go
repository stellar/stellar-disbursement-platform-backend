package message

import (
	"fmt"
	"strings"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

type Message struct {
	ToPhoneNumber string
	ToEmail       string
	Message       string
	Title         string
}

// ValidateFor validates if the message object is valid for the given messengerType.
func (s *Message) ValidateFor(messengerType MessengerType) error {
	if messengerType.IsSMS() {
		if err := utils.ValidatePhoneNumber(s.ToPhoneNumber); err != nil {
			return fmt.Errorf("invalid message: %w", err)
		}
	}

	if messengerType.IsEmail() {
		if err := utils.ValidateEmail(s.ToEmail); err != nil {
			return fmt.Errorf("invalid message: %w", err)
		}

		if strings.Trim(s.Title, " ") == "" {
			return fmt.Errorf("title is empty")
		}
	}

	if strings.Trim(s.Message, " ") == "" {
		return fmt.Errorf("message is empty")
	}

	return nil
}
