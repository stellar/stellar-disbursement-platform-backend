package auth

import (
	"context"
	"errors"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/middleware"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

func GetUserFromContext(ctx context.Context, authManager auth.AuthManager) (*auth.User, *httperror.HTTPError) {
	userID, ok := ctx.Value(middleware.UserIDContextKey).(string)
	if !ok || userID == "" {
		return nil, httperror.Unauthorized("User ID not found in context", nil, nil)
	}

	user, err := authManager.GetUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, auth.ErrUserNotFound) {
			return nil, httperror.BadRequest("User not found", err, nil)
		}
		return nil, httperror.InternalError(ctx, "Cannot get user", err, nil)
	}

	return user, nil
}
