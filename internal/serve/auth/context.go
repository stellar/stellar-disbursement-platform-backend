package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

func GetUserFromContext(ctx context.Context, authManager auth.AuthManager) (*auth.User, error) {
	userID, err := sdpcontext.GetUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	user, err := authManager.GetUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, auth.ErrUserNotFound) {
			return nil, fmt.Errorf("user not found with id %s", userID)
		}
		return nil, fmt.Errorf("cannot get user %w", err)
	}

	return user, nil
}
