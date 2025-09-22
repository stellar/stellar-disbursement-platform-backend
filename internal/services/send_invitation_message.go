package services

import (
	"context"
	"fmt"
	"net/url"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/htmltemplate"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
)

type SendInvitationMessageOptions struct {
	FirstName string
	Email     string
	Role      string
	UIBaseURL string
}

func (o SendInvitationMessageOptions) Validate() error {
	if o.FirstName == "" {
		return fmt.Errorf("first name is required")
	}
	if o.Email == "" {
		return fmt.Errorf("email is required")
	}
	if o.Role == "" {
		return fmt.Errorf("role is required")
	}
	if o.UIBaseURL == "" {
		return fmt.Errorf("UI base URL is required")
	} else {
		_, err := url.Parse(o.UIBaseURL)
		if err != nil {
			return fmt.Errorf("UI base URL is not a valid URL: %w", err)
		}
	}

	return nil
}

const invitationMessageTitle = "Welcome to Stellar Disbursement Platform"

func SendInvitationMessage(ctx context.Context, messengerClient message.MessengerClient, models *data.Models, opts SendInvitationMessageOptions) error {
	err := opts.Validate()
	if err != nil {
		return fmt.Errorf("invalid options: %w", err)
	}

	organization, err := models.Organizations.Get(ctx)
	if err != nil {
		return fmt.Errorf("getting organization: %w", err)
	}

	forgotPasswordLink, err := url.JoinPath(opts.UIBaseURL, "forgot-password")
	if err != nil {
		return fmt.Errorf("getting forgot password link: %w", err)
	}

	invitationMsgData := htmltemplate.StaffInvitationEmailMessageTemplate{
		FirstName:          opts.FirstName,
		Role:               opts.Role,
		ForgotPasswordLink: forgotPasswordLink,
		OrganizationName:   organization.Name,
	}
	messageContent, err := htmltemplate.ExecuteHTMLTemplateForStaffInvitationEmailMessage(invitationMsgData)
	if err != nil {
		return fmt.Errorf("executing invitation message HTML template: %w", err)
	}

	msg := message.Message{
		ToEmail: opts.Email,
		Body:    messageContent,
		Title:   invitationMessageTitle,
		Type:    message.MessageTypeUserInvitation,
		TemplateVariables: map[string]string{
			"FirstName":          opts.FirstName,
			"Role":               opts.Role,
			"ForgotPasswordLink": forgotPasswordLink,
			"OrganizationName":   organization.Name,
		},
	}

	if sendMsgErr := messengerClient.SendMessage(ctx, msg); sendMsgErr != nil {
		return fmt.Errorf("sending invitation message via messenger client: %w", sendMsgErr)
	}

	return nil
}
