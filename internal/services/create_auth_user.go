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
}

func NewCreateUserService(models *data.Models, dbConnectionPool db.DBConnectionPool, authManager auth.AuthManager) *CreateAuthUserService {
	return &CreateAuthUserService{
		models:           models,
		dbConnectionPool: dbConnectionPool,
		authManager:      authManager,
	}
}

func (s *CreateAuthUserService) CreateUser(ctx context.Context, newUser auth.User, uiBaseURL string) (*auth.User, *message.Message, error) {
	// The password is empty so the AuthManager will generate one automatically.
	u, err := s.authManager.CreateUser(ctx, &newUser, "")
	if err != nil {
		return nil, nil, fmt.Errorf("creating new user: %w", err)
	}

	organization, err := s.models.Organizations.Get(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("getting organization: %w", err)
	}

	forgotPasswordLink, err := url.JoinPath(uiBaseURL, "forgot-password")
	if err != nil {
		return nil, nil, fmt.Errorf("getting forgot password link: %w", err)
	}

	invitationMsgData := htmltemplate.InvitationMessageTemplate{
		FirstName:          u.FirstName,
		Role:               u.Roles[0],
		ForgotPasswordLink: forgotPasswordLink,
		OrganizationName:   organization.Name,
	}
	messageContent, err := htmltemplate.ExecuteHTMLTemplateForInvitationMessage(invitationMsgData)
	if err != nil {
		return nil, nil, fmt.Errorf("executing invitation message HTML template: %w", err)
	}

	msg := &message.Message{
		ToEmail: u.Email,
		Message: messageContent,
		Title:   invitationMessageTitle,
	}

	return u, msg, nil
}
