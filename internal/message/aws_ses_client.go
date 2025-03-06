package message

import (
	"fmt"
	"html/template"
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
	emailBody := message.Body
	var err error
	// If the email body does not contain an HTML tag, then it is considered as a plain text email:
	if !strings.Contains(emailBody, "<html") {
		emailBody, err = htmltemplate.ExecuteHTMLTemplateForEmailEmptyBody(htmltemplate.EmptyBodyEmailTemplate{Body: template.HTML(emailBody)})
		if err != nil {
			return nil, fmt.Errorf("generating html template: %w", err)
		}
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
					Data:    aws.String(emailBody),
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
	secretAccessKey = strings.TrimSpace(secretAccessKey)
	region = strings.TrimSpace(region)

	awsConfig := aws.Config{}
	if accessKeyID != "" && secretAccessKey != "" && region != "" {
		log.Debug("Using SDP custom AWS static credential configuration")
		awsConfig.Credentials = credentials.NewStaticCredentials(accessKeyID, secretAccessKey, "")
		awsConfig.Region = aws.String(region)
	}

	awsSession, err := session.NewSession(&awsConfig)
	if err != nil {
		return nil, fmt.Errorf("creating AWS session: %w", err)
	}

	senderID = strings.TrimSpace(senderID)
	if err := utils.ValidateEmail(senderID); err != nil {
		return nil, fmt.Errorf("aws SES (email) senderID is invalid: %w", err)
	}

	return &awsSESClient{
		senderID:     senderID,
		emailService: ses.New(awsSession),
	}, nil
}

var _ MessengerClient = (*awsSESClient)(nil)
