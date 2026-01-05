package message

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/stellar/go-stellar-sdk/support/log"
	"github.com/twilio/twilio-go"
	twilioApi "github.com/twilio/twilio-go/rest/api/v2010"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

type twilioWhatsAppClient struct {
	apiService twilioAPIInterface
	fromNumber string
	templates  map[MessageType]string
}

func (t *twilioWhatsAppClient) MessengerType() MessengerType {
	return MessengerTypeTwilioWhatsApp
}

// SendMessage sends a WhatsApp message using a predefined template.
func (t *twilioWhatsAppClient) SendMessage(ctx context.Context, message Message) error {
	err := message.ValidateFor(t.MessengerType())
	if err != nil {
		return fmt.Errorf("validating WhatsApp message: %w", err)
	}

	toWhatsApp := formatWhatsAppNumber(message.ToPhoneNumber)
	fromWhatsApp := formatWhatsAppNumber(t.fromNumber)

	params := &twilioApi.CreateMessageParams{
		To:   &toWhatsApp,
		From: &fromWhatsApp,
	}

	templateID, ok := t.templates[message.Type]
	if !ok || strings.TrimSpace(templateID) == "" {
		return fmt.Errorf("no WhatsApp template SID configured for message type %q", message.Type)
	}
	params.SetContentSid(templateID)

	if len(message.TemplateVariables) > 0 {
		contentVariables, contentVarErr := formatContentVariables(message.Type, message.TemplateVariables)
		if contentVarErr != nil {
			return fmt.Errorf("formatting WhatsApp content variables: %w", contentVarErr)
		}
		params.SetContentVariables(contentVariables)
	}

	log.Ctx(ctx).Debugf("📞 Sending WhatsApp template message with SID %s to phoneNumber %q",
		utils.TruncateString(templateID, 3),
		utils.TruncateString(message.ToPhoneNumber, 3))

	resp, err := t.apiService.CreateMessage(params)
	if err != nil {
		return fmt.Errorf("sending Twilio WhatsApp message: %w", err)
	}

	if resp.ErrorCode != nil || resp.ErrorMessage != nil {
		return parseTwilioErr(resp)
	}

	return nil
}

// templateMapping defines the required variables and their position for each message type
type templateMapping struct {
	requiredVars map[TemplateVariable]string // maps variable to position key
}

var messageTemplateConfig = map[MessageType]templateMapping{
	MessageTypeReceiverInvitation: {
		requiredVars: map[TemplateVariable]string{
			TemplateVarOrgName:                  "1",
			TemplateVarReceiverRegistrationLink: "2",
		},
	},
	MessageTypeReceiverOTP: {
		requiredVars: map[TemplateVariable]string{
			TemplateVarReceiverOTP: "1",
		},
	},
}

// formatContentVariables formats the template variables into a JSON string as required by Twilio's API.
func formatContentVariables(messageType MessageType, vars map[TemplateVariable]string) (string, error) {
	config, ok := messageTemplateConfig[messageType]
	if !ok {
		return "", fmt.Errorf("unsupported message type %s for WhatsApp template variables", messageType)
	}

	// Validate all required variables are present (extra variables are allowed and ignored)
	for templateVar := range config.requiredVars {
		if _, ok := vars[templateVar]; !ok {
			return "", fmt.Errorf("missing required template variable %s for message type %s", templateVar, messageType)
		}
	}

	// Build content variables map with position mapping
	contentVars := make(map[string]string, len(config.requiredVars))
	for templateVar, position := range config.requiredVars {
		value, ok := vars[templateVar]
		if !ok {
			return "", fmt.Errorf("missing template variable %s for message type %s",
				templateVar, messageType)
		}
		contentVars[position] = value
	}

	contentVarsJSON, err := json.Marshal(contentVars)
	if err != nil {
		return "", fmt.Errorf("marshaling WhatsApp content variables to JSON: %w", err)
	}

	return string(contentVarsJSON), nil
}

// formatWhatsAppNumber ensures the phone number has the `whatsapp:` prefix.
func formatWhatsAppNumber(phoneNumber string) string {
	phoneNumber = strings.TrimSpace(phoneNumber)
	if !strings.HasPrefix(phoneNumber, "whatsapp:") {
		return "whatsapp:" + phoneNumber
	}
	return phoneNumber
}

// NewTwilioWhatsAppClient creates a new Twilio WhatsApp client that is used to send WhatsApp messages.
func NewTwilioWhatsAppClient(accountSid, authToken, fromNumber string, templates map[MessageType]string) (MessengerClient, error) {
	accountSid = strings.TrimSpace(accountSid)
	if accountSid == "" {
		return nil, fmt.Errorf("twilio WhatsApp accountSid is empty")
	}

	authToken = strings.TrimSpace(authToken)
	if authToken == "" {
		return nil, fmt.Errorf("twilio WhatsApp authToken is empty")
	}

	fromNumber = strings.TrimSpace(fromNumber)
	if fromNumber == "" {
		return nil, fmt.Errorf("twilio WhatsApp fromNumber is empty")
	}

	cleanFromNumber := strings.TrimPrefix(fromNumber, "whatsapp:")
	if err := utils.ValidatePhoneNumber(cleanFromNumber); err != nil {
		return nil, fmt.Errorf("twilio WhatsApp fromNumber is invalid: %w", err)
	}

	for _, msgType := range receiverMessageTypes() {
		if templateID, ok := templates[msgType]; !ok || strings.TrimSpace(templateID) == "" {
			return nil, fmt.Errorf("missing template SID for message type %s", msgType)
		}
	}

	return &twilioWhatsAppClient{
		apiService: twilio.NewRestClientWithParams(twilio.ClientParams{
			Username: accountSid,
			Password: authToken,
		}).Api,
		fromNumber: fromNumber,
		templates:  templates,
	}, nil
}

var _ MessengerClient = (*twilioWhatsAppClient)(nil)
