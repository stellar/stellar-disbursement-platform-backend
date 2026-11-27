package message

import (
	"context"
	"fmt"
	"strings"

	"github.com/sendgrid/rest"
	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
	"github.com/stellar/go-stellar-sdk/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/htmltemplate"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

type twilioSendGridInterface interface {
	SendWithContext(ctx context.Context, email *mail.SGMailV3) (*rest.Response, error)
}

// sendGridSender creates a sendgrid.Client per email: the library stores each request body on the client, so a shared
// client lets concurrent sends overwrite each other.
type sendGridSender struct {
	apiKey string
}

func (s sendGridSender) SendWithContext(ctx context.Context, email *mail.SGMailV3) (*rest.Response, error) {
	response, err := sendgrid.NewSendClient(s.apiKey).SendWithContext(ctx, email)
	if err != nil {
		return nil, fmt.Errorf("calling the SendGrid API: %w", err)
	}
	return response, nil
}

var _ twilioSendGridInterface = sendGridSender{}

type twilioSendGridClient struct {
	client        twilioSendGridInterface
	senderAddress string
}

func (t *twilioSendGridClient) MessengerType() MessengerType {
	return MessengerTypeTwilioEmail
}

func (t *twilioSendGridClient) SendMessage(ctx context.Context, message Message) error {
	err := message.ValidateFor(t.MessengerType())
	if err != nil {
		return fmt.Errorf("validating message to send an email through SendGrid: %w", err)
	}

	from := mail.NewEmail("", t.senderAddress)
	to := mail.NewEmail("", message.ToEmail)

	emailBody := message.Body
	// Only trusted staff emails are pre-rendered HTML; everything else (receiver messages, unknown types)
	// is untrusted text and must be escaped, so it can't smuggle live markup past this wrapper.
	if !message.Type.IsTrustedHTMLBody() {
		var htmlErr error
		emailBody, htmlErr = htmltemplate.ExecuteHTMLTemplateForEmailEmptyBody(htmltemplate.EmptyBodyEmailTemplate{Body: emailBody})
		if htmlErr != nil {
			return fmt.Errorf("generating html template: %w", htmlErr)
		}
	}

	email := mail.NewSingleEmail(from, message.Title, to, "", emailBody)

	response, err := t.client.SendWithContext(ctx, email)
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
		client:        sendGridSender{apiKey: apiKey},
		senderAddress: senderAddress,
	}, nil
}

var _ MessengerClient = (*twilioSendGridClient)(nil)
