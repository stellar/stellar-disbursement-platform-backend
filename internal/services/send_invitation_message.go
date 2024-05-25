package services

import (
	"context"
	"fmt"
	"net/url"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/htmltemplate"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
)

const invitationMessageTitle = "Welcome to Stellar Disbursement Platform"

func SendInvitationMessage(ctx context.Context, messengerClient message.MessengerClient, models *data.Models, firstName, role, email, uiBaseURL string) error {
	organization, err := models.Organizations.Get(ctx)
	if err != nil {
		return fmt.Errorf("getting organization: %w", err)
	}

	forgotPasswordLink, err := url.JoinPath(uiBaseURL, "forgot-password")
	if err != nil {
		return fmt.Errorf("getting forgot password link: %w", err)
	}

	invitationMsgData := htmltemplate.InvitationMessageTemplate{
		FirstName:          firstName,
		Role:               role,
		ForgotPasswordLink: forgotPasswordLink,
		OrganizationName:   organization.Name,
	}
	messageContent, err := htmltemplate.ExecuteHTMLTemplateForInvitationMessage(invitationMsgData)
	if err != nil {
		return fmt.Errorf("executing invitation message HTML template: %w", err)
	}

	msg := message.Message{
		ToEmail: email,
		Message: messageContent,
		Title:   invitationMessageTitle,
	}

	if sendMsgErr := messengerClient.SendMessage(msg); sendMsgErr != nil {
		return fmt.Errorf("sending invitation message: %w", sendMsgErr)
	}

	return nil
}
