package message

import (
	"fmt"
	"html/template"
	"strings"

	"github.com/sendgrid/rest"
	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/htmltemplate"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

type twilioSendGridInterface interface {
	Send(email *mail.SGMailV3) (*rest.Response, error)
}

var _ twilioSendGridInterface = (*sendgrid.Client)(nil)

type twilioSendGridClient struct {
	client        twilioSendGridInterface
	senderAddress string
}

func (t *twilioSendGridClient) MessengerType() MessengerType {
	return MessengerTypeTwilioEmail
}

func (t *twilioSendGridClient) SendMessage(message Message) error {
	err := message.ValidateFor(t.MessengerType())
	if err != nil {
		return fmt.Errorf("validating message to send an email through SendGrid: %w", err)
	}

	from := mail.NewEmail("", t.senderAddress)
	to := mail.NewEmail("", message.ToEmail)

	emailBody := message.Body
	if !strings.Contains(emailBody, "<html") {
		var htmlErr error
		emailBody, htmlErr = htmltemplate.ExecuteHTMLTemplateForEmailEmptyBody(htmltemplate.EmptyBodyEmailTemplate{Body: template.HTML(emailBody)})
		if htmlErr != nil {
			return fmt.Errorf("generating html template: %w", htmlErr)
		}
	}

	email := mail.NewSingleEmail(from, message.Title, to, "", emailBody)

	response, err := t.client.Send(email)
	if err != nil {
		return fmt.Errorf("sending SendGrid email: %w", err)
	}

	if response.StatusCode >= 400 {
		return fmt.Errorf("sendGrid API returned error status code= %d, body= %s",
			response.StatusCode, response.Body)
	}

	log.Debugf("🎉 SendGrid sent an email to the receiver %q", utils.TruncateString(message.ToEmail, 3))
	return nil
}

// NewTwilioSendGridClient creates a new SendGrid client that is used to send emails
func NewTwilioSendGridClient(apiKey string, senderAddress string) (MessengerClient, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil, fmt.Errorf("sendGrid API key is empty")
	}

	senderAddress = strings.TrimSpace(senderAddress)
	if err := utils.ValidateEmail(senderAddress); err != nil {
		return nil, fmt.Errorf("sendGrid senderAddress is invalid: %w", err)
	}

	return &twilioSendGridClient{
		client:        sendgrid.NewSendClient(apiKey),
		senderAddress: senderAddress,
	}, nil
}

var _ MessengerClient = (*twilioSendGridClient)(nil)
