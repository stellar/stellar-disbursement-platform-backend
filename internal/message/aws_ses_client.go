package message

import (
	"context"
	"fmt"
	"html/template"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ses"
	"github.com/aws/aws-sdk-go-v2/service/ses/types"
	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/htmltemplate"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

// awsSESInterface is used to send emails.
type awsSESInterface interface {
	SendEmail(context.Context, *ses.SendEmailInput, ...func(*ses.Options)) (*ses.SendEmailOutput, error)
}

// awsSESClient is used to send emails.
type awsSESClient struct {
	emailService awsSESInterface
	senderID     string
}

func (c *awsSESClient) MessengerType() MessengerType {
	return MessengerTypeAWSEmail
}

func (c *awsSESClient) SendMessage(ctx context.Context, message Message) error {
	err := message.ValidateFor(c.MessengerType())
	if err != nil {
		return fmt.Errorf("validating message to send an email through AWS: %w", err)
	}

	emailTemplate, err := generateAWSEmail(message, c.senderID)
	if err != nil {
		return fmt.Errorf("generating AWS SES email template: %w", err)
	}

	_, err = c.emailService.SendEmail(ctx, emailTemplate)
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
		Destination: &types.Destination{
			ToAddresses: []string{message.ToEmail},
		},
		Message: &types.Message{
			Body: &types.Body{
				Html: &types.Content{
					Charset: aws.String("utf-8"),
					Data:    aws.String(emailBody),
				},
			},
			Subject: &types.Content{
				Charset: aws.String("utf-8"),
				Data:    aws.String(message.Title),
			},
		},
		Source: aws.String(sender),
	}, nil
}

// NewAWSSESClient creates a new AWS SES client, that is used to send emails.
func NewAWSSESClient(accessKeyID, secretAccessKey, region, senderID string) (*awsSESClient, error) {
	senderID = strings.TrimSpace(senderID)
	if err := utils.ValidateEmail(senderID); err != nil {
		return nil, fmt.Errorf("aws SES (email) senderID is invalid: %w", err)
	}

	cfg, err := loadAWSConfig(accessKeyID, secretAccessKey, region)
	if err != nil {
		return nil, fmt.Errorf("loading AWS config for SES: %w", err)
	}

	sesClient := ses.NewFromConfig(cfg)

	return &awsSESClient{
		senderID:     senderID,
		emailService: sesClient,
	}, nil
}

// loadAWSConfig loads the AWS config from static credentials, if available, otherwise from AWS default session.
func loadAWSConfig(accessKeyID, secretAccessKey, region string) (aws.Config, error) {
	accessKeyID = strings.TrimSpace(accessKeyID)
	secretAccessKey = strings.TrimSpace(secretAccessKey)
	region = strings.TrimSpace(region)

	var cfg aws.Config
	var err error

	// Use static credentials if provided, otherwise load from AWS default session
	if accessKeyID != "" && secretAccessKey != "" && region != "" {
		log.Info("⚙️ AWS will be configured with static credentials")
		cfg, err = config.LoadDefaultConfig(context.Background(),
			config.WithRegion(region),
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, "")),
		)
		if err != nil {
			return aws.Config{}, fmt.Errorf("loading AWS config from static credentials: %w", err)
		}
	} else {
		log.Info("⚙️ AWS will be configured from AWS default session")
		cfg, err = config.LoadDefaultConfig(context.Background(), config.WithRegion(region))
		if err != nil {
			return aws.Config{}, fmt.Errorf("loading AWS config from AWS Session: %w", err)
		}
	}

	return cfg, nil
}

var _ MessengerClient = (*awsSESClient)(nil)
