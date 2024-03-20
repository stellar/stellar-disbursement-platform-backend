package services

import (
	"context"
	"fmt"
	"net/url"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/htmltemplate"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

const invitationMessageTitle = "Welcome to Stellar Disbursement Platform"

type CreateAuthUserService struct {
	models           *data.Models
	dbConnectionPool db.DBConnectionPool
	authManager      auth.AuthManager
	messengerClient  message.MessengerClient
}

func NewCreateUserService(models *data.Models, dbConnectionPool db.DBConnectionPool, authManager auth.AuthManager, messengerClient message.MessengerClient) *CreateAuthUserService {
	return &CreateAuthUserService{
		models:           models,
		dbConnectionPool: dbConnectionPool,
		authManager:      authManager,
		messengerClient:  messengerClient,
	}
}

func (s *CreateAuthUserService) CreateUser(ctx context.Context, sqlExecutor db.SQLExecuter, newUser auth.User, uiBaseURL string) (*auth.User, error) {
	// The password is empty so the AuthManager will generate one automatically.
	u, err := s.authManager.CreateUser(ctx, sqlExecutor, &newUser, "")
	if err != nil {
		return nil, fmt.Errorf("creating new user: %w", err)
	}

	organization, err := s.models.Organizations.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting organization: %w", err)
	}

	forgotPasswordLink, err := url.JoinPath(uiBaseURL, "forgot-password")
	if err != nil {
		return nil, fmt.Errorf("getting forgot password link: %w", err)
	}

	invitationMsgData := htmltemplate.InvitationMessageTemplate{
		FirstName:          u.FirstName,
		Role:               u.Roles[0],
		ForgotPasswordLink: forgotPasswordLink,
		OrganizationName:   organization.Name,
	}
	messageContent, err := htmltemplate.ExecuteHTMLTemplateForInvitationMessage(invitationMsgData)
	if err != nil {
		return nil, fmt.Errorf("executing invitation message HTML template: %w", err)
	}

	msg := message.Message{
		ToEmail: u.Email,
		Message: messageContent,
		Title:   invitationMessageTitle,
	}
	err = s.messengerClient.SendMessage(msg)
	if err != nil {
		return nil, fmt.Errorf("sending invitation email for user %s: %w", u.ID, err)
	}

	return u, nil
}
