package message

import (
	"fmt"
	"strings"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	authUtils "github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/utils"
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
		if err := s.IsValidForEmail(); err != nil {
			return fmt.Errorf("invalid e-mail: %w", err)
		}
	}

	if strings.Trim(s.Message, " ") == "" {
		return fmt.Errorf("message is empty")
	}

	return nil
}

func (s *Message) IsValidForEmail() error {
	sanitizedEmail, err := authUtils.SanitizeAndValidateEmail(s.ToEmail)
	if err != nil {
		return fmt.Errorf("invalid email format: %w", err)
	}
	s.ToEmail = sanitizedEmail

	if strings.TrimSpace(s.Title) == "" {
		return fmt.Errorf("title is empty")
	}

	return nil
}

func (s *Message) SupportedChannels() []MessageChannel {
	var supportedChannels []MessageChannel

	if utils.ValidatePhoneNumber(s.ToPhoneNumber) == nil {
		supportedChannels = append(supportedChannels, MessageChannelSMS)
	}

	if err := s.IsValidForEmail(); err == nil {
		supportedChannels = append(supportedChannels, MessageChannelEmail)
	}

	return supportedChannels
}

func (s *Message) String() string {
	return fmt.Sprintf("Message{ToPhoneNumber: %s, ToEmail: %s, Message: %s, Title: %s}",
		utils.TruncateString(s.ToPhoneNumber, 3),
		utils.TruncateString(s.ToEmail, 3),
		utils.TruncateString(s.Message, 3),
		utils.TruncateString(s.Title, 3))
}
