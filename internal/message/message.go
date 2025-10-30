package message

import (
	"fmt"
	"strings"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

type MessageType string

const (
	MessageTypeUserForgotPassword MessageType = "user_forgot_password"
	MessageTypeUserMFA            MessageType = "user_mfa"
	MessageTypeUserInvitation     MessageType = "user_invitation"
	MessageTypeReceiverInvitation MessageType = "receiver_invitation"
	MessageTypeReceiverOTP        MessageType = "receiver_otp"
)

type TemplateVariable string

const (
	TemplateVarOrgName                  TemplateVariable = "org_name"
	TemplateVarReceiverOTP              TemplateVariable = "receiver_otp"
	TemplateVarReceiverRegistrationLink TemplateVariable = "registration_link"
	TemplateVarResetPasswordLink        TemplateVariable = "reset_password_link"
	TemplateVarMFACode                  TemplateVariable = "mfa_code"
	TemplateVarFirstName                TemplateVariable = "first_name"
	TemplateVarRole                     TemplateVariable = "role"
	TemplateVarForgotPasswordLink       TemplateVariable = "forgot_password_link"
)

// receiverMessageTypes returns MessageType values related to receivers.
func receiverMessageTypes() []MessageType {
	return []MessageType{
		MessageTypeReceiverInvitation,
		MessageTypeReceiverOTP,
	}
}

type Message struct {
	Type              MessageType
	ToPhoneNumber     string
	ToEmail           string
	Body              string
	Title             string
	TemplateVariables map[TemplateVariable]string
}

// ValidateFor validates if the message object is valid for the given messengerType.
func (m Message) ValidateFor(messengerType MessengerType) error {
	if messengerType.IsSMS() {
		if err := utils.ValidatePhoneNumber(m.ToPhoneNumber); err != nil {
			return fmt.Errorf("invalid message: %w", err)
		}
	}

	if messengerType.IsEmail() {
		if err := m.IsValidForEmail(); err != nil {
			return fmt.Errorf("invalid e-mail: %w", err)
		}
	}

	// WhatsApp template messages don't need a body since they use predefined templates
	if messengerType != MessengerTypeTwilioWhatsApp && strings.TrimSpace(m.Body) == "" {
		return fmt.Errorf("message is empty")
	}

	return nil
}

func (m Message) IsValidForEmail() error {
	if err := utils.ValidateEmail(m.ToEmail); err != nil {
		return fmt.Errorf("invalid email format: %w", err)
	}

	if strings.TrimSpace(m.Title) == "" {
		return fmt.Errorf("title is empty")
	}
	return nil
}

func (m Message) SupportedChannels() []MessageChannel {
	var supportedChannels []MessageChannel

	if utils.ValidatePhoneNumber(m.ToPhoneNumber) == nil {
		supportedChannels = append(supportedChannels, MessageChannelSMS)
	}

	if err := m.IsValidForEmail(); err == nil {
		supportedChannels = append(supportedChannels, MessageChannelEmail)
	}

	return supportedChannels
}

func (m Message) String() string {
	return fmt.Sprintf("Message{ToPhoneNumber: %s, ToEmail: %s, Message: %s, Title: %s}",
		utils.TruncateString(m.ToPhoneNumber, 3),
		utils.TruncateString(m.ToEmail, 3),
		utils.TruncateString(m.Body, 3),
		utils.TruncateString(m.Title, 3))
}
