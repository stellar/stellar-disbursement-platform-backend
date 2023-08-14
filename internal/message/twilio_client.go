package message

import (
	"fmt"
	"strings"

	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/twilio/twilio-go"
	twilioApi "github.com/twilio/twilio-go/rest/api/v2010"
)

type twilioApiInterface interface {
	CreateMessage(params *twilioApi.CreateMessageParams) (*twilioApi.ApiV2010Message, error)
}

type twilioClient struct {
	apiService twilioApiInterface
	senderID   string
}

func (t *twilioClient) MessengerType() MessengerType {
	return MessengerTypeTwilioSMS
}

func (t *twilioClient) CreateMessage(params *twilioApi.CreateMessageParams) (*twilioApi.ApiV2010Message, error) {
	return t.apiService.CreateMessage(params)
}

func (t *twilioClient) SendMessage(message Message) error {
	err := message.ValidateFor(t.MessengerType())
	if err != nil {
		return fmt.Errorf("validating SMS message: %w", err)
	}

	resp, err := t.CreateMessage(&twilioApi.CreateMessageParams{
		To:                  &message.ToPhoneNumber,
		Body:                &message.Message,
		MessagingServiceSid: &t.senderID,
	})
	if err != nil {
		return fmt.Errorf("sending Twilio SMS: %w", err)
	}

	if resp.ErrorCode != nil || resp.ErrorMessage != nil {
		var errorCode string
		if resp.ErrorCode != nil {
			errorCode = fmt.Sprintf("%d", *resp.ErrorCode)
		}

		var errorMessage string
		if resp.ErrorMessage != nil {
			errorMessage = *resp.ErrorMessage
		}

		return fmt.Errorf("sending Twilio SMS responded an error {code: %q, message: %q}", errorCode, errorMessage)
	}

	log.Debugf("Twilio sent an SMS to the phoneNumber %q", utils.TruncateString(message.ToPhoneNumber, 3))
	return nil
}

func NewTwilioClient(accountSid, authToken, senderID string) (*twilioClient, error) {
	accountSid = strings.TrimSpace(accountSid)
	if accountSid == "" {
		return nil, fmt.Errorf("twilio accountSid is empty")
	}

	authToken = strings.TrimSpace(authToken)
	if authToken == "" {
		return nil, fmt.Errorf("twilio authToken is empty")
	}

	senderID = strings.TrimSpace(senderID)
	if senderID == "" {
		return nil, fmt.Errorf("twilio senderID is empty")
	}

	return &twilioClient{
		apiService: twilio.NewRestClientWithParams(twilio.ClientParams{
			Username: accountSid,
			Password: authToken,
		}).Api,
		senderID: senderID,
	}, nil
}

var _ MessengerClient = (*twilioClient)(nil)
