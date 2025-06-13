package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/middleware"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

func GetUserFromContext(ctx context.Context, authManager auth.AuthManager) (*auth.User, error) {
	userID, ok := ctx.Value(middleware.UserIDContextKey).(string)
	if !ok || userID == "" {
		return nil, errors.New("user ID not found in context")
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
