package sdpcontext

import (
	"context"
	"errors"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

var (
	ErrTenantNotFoundInContext                = errors.New("tenant not found in context")
	ErrUserIDNotFoundInContext                = errors.New("user ID not found in context")
	ErrTokenNotFoundInContext                 = errors.New("token not found in context")
	ErrAPIKeyNotFoundInContext                = errors.New("API key not found in context")
	ErrWalletContractAddressNotFoundInContext = errors.New("wallet contract address not found in context")
)

type (
	tenantContextKey                struct{}
	tokenContextKey                 struct{}
	userIDContextKey                struct{}
	apiKeyContextKey                struct{}
	walletContractAddressContextKey struct{}
)

const (
	NoTenantName = "no_tenant"
)

// GetTenantFromContext retrieves the tenant information from the context.
func GetTenantFromContext(ctx context.Context) (*schema.Tenant, error) {
	currentTenant, ok := ctx.Value(tenantContextKey{}).(*schema.Tenant)
	if !ok {
		return nil, ErrTenantNotFoundInContext
	}
	return currentTenant, nil
}

// MustGetTenantNameFromContext retrieves the tenant information from the context and defaults to a no_tenant if not found.
func MustGetTenantNameFromContext(ctx context.Context) string {
	t, err := GetTenantFromContext(ctx)
	if err != nil || t == nil {
		return NoTenantName
	}
	return t.Name
}

// SetTenantInContext stores the tenant information in the context.
func SetTenantInContext(ctx context.Context, t *schema.Tenant) context.Context {
	return context.WithValue(ctx, tenantContextKey{}, t)
}

// GetUserIDFromContext retrieves the user ID from the context.
func GetUserIDFromContext(ctx context.Context) (string, error) {
	userID, ok := ctx.Value(userIDContextKey{}).(string)
	if !ok || userID == "" {
		return "", ErrUserIDNotFoundInContext
	}
	return userID, nil
}

// SetUserIDInContext stores the user ID in the context.
func SetUserIDInContext(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDContextKey{}, userID)
}

// GetTokenFromContext retrieves the authentication token from the context.
func GetTokenFromContext(ctx context.Context) (string, error) {
	token, ok := ctx.Value(tokenContextKey{}).(string)
	if !ok || token == "" {
		return "", ErrTokenNotFoundInContext
	}
	return token, nil
}

// SetTokenInContext stores the authentication token in the context.
func SetTokenInContext(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, tokenContextKey{}, token)
}

// GetAPIKeyFromContext retrieves the API key from the context.
func GetAPIKeyFromContext(ctx context.Context) (*data.APIKey, error) {
	apiKey, ok := ctx.Value(apiKeyContextKey{}).(*data.APIKey)
	if !ok {
		return nil, ErrAPIKeyNotFoundInContext
	}
	return apiKey, nil
}

// SetAPIKeyInContext stores the API key in the context.
func SetAPIKeyInContext(ctx context.Context, apiKey *data.APIKey) context.Context {
	return context.WithValue(ctx, apiKeyContextKey{}, apiKey)
}

// GetWalletContractAddressFromContext retrieves the wallet contract address from the context.
func GetWalletContractAddressFromContext(ctx context.Context) (string, error) {
	contractAddress, ok := ctx.Value(walletContractAddressContextKey{}).(string)
	if !ok || contractAddress == "" {
		return "", ErrWalletContractAddressNotFoundInContext
	}
	return contractAddress, nil
}

// SetWalletContractAddressInContext stores the wallet contract address in the context.
func SetWalletContractAddressInContext(ctx context.Context, contractAddress string) context.Context {
	return context.WithValue(ctx, walletContractAddressContextKey{}, contractAddress)
}
