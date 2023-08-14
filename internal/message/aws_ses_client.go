package message

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ses"
	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/htmltemplate"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

// awsSESInterface is used to send emails.
type awsSESInterface interface {
	SendEmail(input *ses.SendEmailInput) (*ses.SendEmailOutput, error)
}

// awsSESClient is used to send emails.
type awsSESClient struct {
	emailService awsSESInterface
	senderID     string
}

func (t *awsSESClient) MessengerType() MessengerType {
	return MessengerTypeAWSEmail
}

func (a *awsSESClient) SendMessage(message Message) error {
	err := message.ValidateFor(a.MessengerType())
	if err != nil {
		return fmt.Errorf("validating message to send an email through AWS: %w", err)
	}

	emailTemplate, err := generateAWSEmail(message, a.senderID)
	if err != nil {
		return fmt.Errorf("generating AWS SES email template: %w", err)
	}

	_, err = a.emailService.SendEmail(emailTemplate)
	if err != nil {
		return fmt.Errorf("sending AWS SES email: %w", err)
	}

	log.Debugf("🎉 AWS SES sent an email to the receiver %q", utils.TruncateString(message.ToEmail, 3))
	return nil
}

// generateAWSEmail generates the email object to send an email through AWS SES.
func generateAWSEmail(message Message, sender string) (*ses.SendEmailInput, error) {
	html, err := htmltemplate.ExecuteHTMLTemplateForEmailEmptyBody(htmltemplate.EmptyBodyEmailTemplate{Body: message.Message})
	if err != nil {
		return nil, fmt.Errorf("generating html template: %w", err)
	}

	return &ses.SendEmailInput{
		Destination: &ses.Destination{
			CcAddresses: []*string{},
			ToAddresses: []*string{
				aws.String(message.ToEmail),
			},
		},
		Message: &ses.Message{
			Body: &ses.Body{
				Html: &ses.Content{
					Charset: aws.String("utf-8"),
					Data:    aws.String(html),
				},
			},
			Subject: &ses.Content{
				Charset: aws.String("utf-8"),
				Data:    aws.String(message.Title),
			},
		},
		Source: aws.String(sender),
	}, nil
}

// NewAWSSESClient creates a new AWS SES client, that is used to send emails.
func NewAWSSESClient(accessKeyID, secretAccessKey, region, senderID string) (*awsSESClient, error) {
	accessKeyID = strings.TrimSpace(accessKeyID)
	if accessKeyID == "" {
		return nil, fmt.Errorf("aws accessKeyID is empty")
	}

	secretAccessKey = strings.TrimSpace(secretAccessKey)
	if secretAccessKey == "" {
		return nil, fmt.Errorf("aws secretAccessKey is empty")
	}

	region = strings.TrimSpace(region)
	if region == "" {
		return nil, fmt.Errorf("aws region is empty")
	}

	senderID = strings.TrimSpace(senderID)
	if err := utils.ValidateEmail(senderID); err != nil {
		return nil, fmt.Errorf("aws SES (email) senderID is invalid: %w", err)
	}

	awsSession, err := session.NewSession(&aws.Config{
		Credentials: credentials.NewStaticCredentials(accessKeyID, secretAccessKey, ""),
		Region:      aws.String(region),
	})
	if err != nil {
		return nil, fmt.Errorf("creating AWS session: %w", err)
	}

	return &awsSESClient{
		senderID:     senderID,
		emailService: ses.New(awsSession),
	}, nil
}

var _ MessengerClient = (*awsSESClient)(nil)
