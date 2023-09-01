package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/lib/pq"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/internal/db"
)

const defaultOwnerRoleName = "owner"

type RoleManager interface {
	GetUserRoles(ctx context.Context, user *User) ([]string, error)
	// HasAllRoles validates whether the user has all roles passed by parameter.
	HasAllRoles(ctx context.Context, user *User, roleNames []string) (bool, error)
	// HasAnyRoles validates whether the user has one or more roles passed by parameter.
	HasAnyRoles(ctx context.Context, user *User, roleNames []string) (bool, error)
	IsSuperUser(ctx context.Context, user *User) (bool, error)
	UpdateRoles(ctx context.Context, user *User, roleNames []string) error
}

type userRolesInfo struct {
	Roles   pq.StringArray `db:"roles"`
	IsOwner bool           `db:"is_owner"`
}

type defaultRoleManager struct {
	dbConnectionPool db.DBConnectionPool
	ownerRoleName    string
}

func (rm *defaultRoleManager) getUserRolesInfo(ctx context.Context, user *User) (*userRolesInfo, error) {
	const query = "SELECT roles, is_owner FROM auth_users WHERE id = $1"

	var ur userRolesInfo
	err := rm.dbConnectionPool.GetContext(ctx, &ur, query, user.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err = ErrUserNotFound
		}
		return nil, fmt.Errorf("error querying user ID %s roles: %w", user.ID, err)
	}

	return &ur, nil
}

func (rm *defaultRoleManager) GetUserRoles(ctx context.Context, user *User) ([]string, error) {
	ur, err := rm.getUserRolesInfo(ctx, user)
	if err != nil {
		return nil, fmt.Errorf("getting user roles info from the database, %w", err)
	}

	if ur.IsOwner {
		return []string{rm.ownerRoleName}, nil
	}

	return ur.Roles, nil
}

func (rm *defaultRoleManager) HasAllRoles(ctx context.Context, user *User, roleNames []string) (bool, error) {
	userRoles, err := rm.GetUserRoles(ctx, user)
	if err != nil {
		return false, fmt.Errorf("getting user roles: %w", err)
	}

	userRolesMap := make(map[string]struct{}, len(userRoles))
	for _, role := range userRoles {
		userRolesMap[role] = struct{}{}
	}

	for _, role := range roleNames {
		if _, ok := userRolesMap[role]; !ok {
			return false, nil
		}
	}

	return true, nil
}

func (rm *defaultRoleManager) HasAnyRoles(ctx context.Context, user *User, roleNames []string) (bool, error) {
	userRoles, err := rm.GetUserRoles(ctx, user)
	if err != nil {
		return false, fmt.Errorf("getting user roles: %w", err)
	}

	userRolesMap := make(map[string]struct{}, len(userRoles))
	for _, role := range userRoles {
		userRolesMap[role] = struct{}{}
	}

	for _, role := range roleNames {
		if _, ok := userRolesMap[role]; ok {
			return true, nil
		}
	}

	return false, nil
}

func (rm *defaultRoleManager) IsSuperUser(ctx context.Context, user *User) (bool, error) {
	ur, err := rm.getUserRolesInfo(ctx, user)
	if err != nil {
		return false, fmt.Errorf("getting user roles info from the database, %w", err)
	}

	return ur.IsOwner, nil
}

func (rm *defaultRoleManager) UpdateRoles(ctx context.Context, user *User, roleNames []string) error {
	const query = "UPDATE auth_users SET roles = $1 WHERE id = $2"
	result, err := rm.dbConnectionPool.ExecContext(ctx, query, pq.Array(roleNames), user.ID)
	if err != nil {
		return fmt.Errorf("error updating user roles ID %s roles: %w", user.ID, err)
	}

	numRowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("error getting number of rows affected: %w", err)
	}

	if numRowsAffected == 0 {
		return ErrNoRowsAffected
	}

	return nil
}

var _ RoleManager = (*defaultRoleManager)(nil)

type defaultRoleManagerOption func(m *defaultRoleManager)

func newDefaultRoleManager(options ...defaultRoleManagerOption) *defaultRoleManager {
	defaultRoleManager := &defaultRoleManager{
		ownerRoleName: defaultOwnerRoleName,
	}

	for _, option := range options {
		option(defaultRoleManager)
	}

	return defaultRoleManager
}

func withRoleManagerDBConnectionPool(dbConnectionPool db.DBConnectionPool) defaultRoleManagerOption {
	return func(m *defaultRoleManager) {
		m.dbConnectionPool = dbConnectionPool
	}
}

func withOwnerRoleName(ownerRoleName string) defaultRoleManagerOption {
	return func(m *defaultRoleManager) {
		m.ownerRoleName = ownerRoleName
	}
}
