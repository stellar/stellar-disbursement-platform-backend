package httphandler

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"

	"github.com/stellar/go/support/http/httpdecode"
	"github.com/stellar/go/support/log"
	"github.com/stellar/go/support/render/httpjson"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/htmltemplate"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/middleware"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

const invitationMessageTitle = "Welcome to Stellar Disbursement Platform"

type UserHandler struct {
	AuthManager     auth.AuthManager
	MessengerClient message.MessengerClient
	UIBaseURL       string
	Models          *data.Models
}

type UserActivationRequest struct {
	UserID   string `json:"user_id"`
	IsActive *bool  `json:"is_active"`
}

type UserSorterByEmail []auth.User

func (a UserSorterByEmail) Len() int           { return len(a) }
func (a UserSorterByEmail) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a UserSorterByEmail) Less(i, j int) bool { return a[i].Email < a[j].Email }

type UserSorterByIsActive []auth.User

func (a UserSorterByIsActive) Len() int           { return len(a) }
func (a UserSorterByIsActive) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a UserSorterByIsActive) Less(i, j int) bool { return a[i].IsActive }

func (uar UserActivationRequest) validate() *httperror.HTTPError {
	validator := validators.NewValidator()

	validator.Check(uar.UserID != "", "user_id", "user_id is required")
	validator.Check(uar.IsActive != nil, "is_active", "is_active is required")

	if validator.HasErrors() {
		return httperror.BadRequest("Request invalid", nil, validator.Errors)
	}

	return nil
}

type CreateUserRequest struct {
	FirstName string          `json:"first_name"`
	LastName  string          `json:"last_name"`
	Email     string          `json:"email"`
	Roles     []data.UserRole `json:"roles"`
}

func (cur CreateUserRequest) validate() *httperror.HTTPError {
	validator := validators.NewValidator()

	validator.Check(cur.FirstName != "", "fist_name", "fist_name is required")
	validator.Check(cur.LastName != "", "last_name", "last_name is required")
	validator.Check(cur.Email != "", "email", "email is required")
	validateRoles(validator, cur.Roles)

	if validator.HasErrors() {
		return httperror.BadRequest("Request invalid", nil, validator.Errors)
	}

	return nil
}

type UpdateRolesRequest struct {
	UserID string          `json:"user_id"`
	Roles  []data.UserRole `json:"roles"`
}

func (upr UpdateRolesRequest) validate() *httperror.HTTPError {
	validator := validators.NewValidator()

	validator.Check(upr.UserID != "", "user_id", "user_id is required")
	validateRoles(validator, upr.Roles)

	if validator.HasErrors() {
		return httperror.BadRequest("Request invalid", nil, validator.Errors)
	}

	return nil
}

func validateRoles(validator *validators.Validator, roles []data.UserRole) {
	// NOTE: in the MVP, users should have only one role.
	validator.Check(len(roles) == 1, "roles", "the number of roles required is exactly one")

	// Validating the role of the request is a valid value
	if _, ok := validator.Errors["roles"]; !ok {
		role := roles[0]
		validator.Check(role.IsValid(), "roles", fmt.Sprintf("unexpected value for roles[0]=%s. Expect one of these values: %s", role, data.GetAllRoles()))
	}
}

func (h UserHandler) UserActivation(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	token, ok := ctx.Value(middleware.TokenContextKey).(string)
	if !ok {
		log.Ctx(ctx).Warn("token not found when updating user activation")
		httperror.Unauthorized("", nil, nil).Render(rw)
		return
	}

	var reqBody UserActivationRequest
	if err := httpdecode.DecodeJSON(req, &reqBody); err != nil {
		err = fmt.Errorf("decoding the request body: %w", err)
		log.Ctx(ctx).Error(err)
		httperror.BadRequest("", err, nil).Render(rw)
		return
	}
	if err := reqBody.validate(); err != nil {
		err.Render(rw)
		return
	}

	// Check if the users are trying to update their own activation
	userID, err := h.AuthManager.GetUserID(ctx, token)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidToken) {
			httperror.Unauthorized("", err, nil).Render(rw)
			return
		}
		err = fmt.Errorf("getting user from token: %w", err)
		httperror.InternalError(ctx, "", err, nil).Render(rw)
		return
	}
	if userID == reqBody.UserID {
		httperror.BadRequest("", nil, map[string]interface{}{"user_id": "cannot update your own activation"}).Render(rw)
		return
	}

	var activationErr error
	if *reqBody.IsActive {
		log.Ctx(ctx).Infof("[ActivateUserAccount] - User ID %s activating user with account ID %s", userID, reqBody.UserID)
		activationErr = h.AuthManager.ActivateUser(ctx, token, reqBody.UserID)
	} else {
		log.Ctx(ctx).Infof("[DeactivateUserAccount] - User ID %s deactivating user with account ID %s", userID, reqBody.UserID)
		activationErr = h.AuthManager.DeactivateUser(ctx, token, reqBody.UserID)
	}

	if activationErr != nil {
		if errors.Is(activationErr, auth.ErrInvalidToken) {
			httperror.Unauthorized("", activationErr, nil).Render(rw)
		} else if errors.Is(activationErr, auth.ErrNoRowsAffected) {
			httperror.BadRequest("", activationErr, map[string]interface{}{"user_id": "user_id is invalid"}).Render(rw)
		} else {
			httperror.InternalError(ctx, "", activationErr, nil).Render(rw)
		}
		return
	}

	httpjson.RenderStatus(rw, http.StatusOK, map[string]string{"message": "user activation was updated successfully"}, httpjson.JSON)
}

func (h UserHandler) CreateUser(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	token, ok := ctx.Value(middleware.TokenContextKey).(string)
	if !ok {
		log.Ctx(ctx).Warn("token not found when updating user activation")
		httperror.Unauthorized("", nil, nil).Render(rw)
		return
	}

	var reqBody CreateUserRequest
	if err := httpdecode.DecodeJSON(req, &reqBody); err != nil {
		err = fmt.Errorf("decoding the request body: %w", err)
		log.Ctx(ctx).Error(err)
		httperror.BadRequest("", err, nil).Render(rw)
		return
	}

	if err := reqBody.validate(); err != nil {
		err.Render(rw)
		return
	}

	authenticatedUserID, err := h.AuthManager.GetUserID(ctx, token)
	if err != nil {
		err = fmt.Errorf("getting request authenticated user ID: %w", err)
		log.Ctx(ctx).Error(err)
		httperror.Unauthorized("", err, nil).Render(rw)
		return
	}

	newUser := &auth.User{
		FirstName: reqBody.FirstName,
		LastName:  reqBody.LastName,
		Email:     reqBody.Email,
		Roles:     data.FromUserRoleArrayToStringArray(reqBody.Roles),
	}

	// The password is empty so the AuthManager will generate one automatically.
	u, err := h.AuthManager.CreateUser(ctx, newUser, "")
	if err != nil {
		if errors.Is(err, auth.ErrUserEmailAlreadyExists) {
			httperror.BadRequest(auth.ErrUserEmailAlreadyExists.Error(), err, nil).Render(rw)
			return
		}

		httperror.InternalError(ctx, "Cannot create user", err, nil).Render(rw)
		return
	}

	organization, err := h.Models.Organizations.Get(ctx)
	if err != nil {
		httperror.InternalError(ctx, "Cannot get organization data", err, nil).Render(rw)
		return
	}

	forgotPasswordLink, err := url.JoinPath(h.UIBaseURL, "forgot-password")
	if err != nil {
		httperror.InternalError(ctx, "Cannot get forgot password link", err, nil).Render(rw)
		return
	}

	invitationMsgData := htmltemplate.InvitationMessageTemplate{
		FirstName:          u.FirstName,
		Role:               u.Roles[0],
		ForgotPasswordLink: forgotPasswordLink,
		OrganizationName:   organization.Name,
	}
	messageContent, err := htmltemplate.ExecuteHTMLTemplateForInvitationMessage(invitationMsgData)
	if err != nil {
		httperror.InternalError(ctx, "Cannot execute invitation message template", err, nil).Render(rw)
		return
	}

	msg := message.Message{
		ToEmail: u.Email,
		Message: messageContent,
		Title:   invitationMessageTitle,
	}
	err = h.MessengerClient.SendMessage(msg)
	if err != nil {
		msg := fmt.Sprintf("Cannot send invitation email for user %s", u.ID)
		httperror.InternalError(ctx, msg, err, nil).Render(rw)
		return
	}

	log.Ctx(ctx).Infof("[CreateUserAccount] - User ID %s created user with account ID %s", authenticatedUserID, u.ID)
	httpjson.RenderStatus(rw, http.StatusCreated, u, httpjson.JSON)
}

func (h UserHandler) UpdateUserRoles(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	token, ok := ctx.Value(middleware.TokenContextKey).(string)
	if !ok {
		log.Ctx(ctx).Warn("token not found when updating user roles")
		httperror.Unauthorized("", nil, nil).Render(rw)
		return
	}

	var reqBody UpdateRolesRequest
	if err := httpdecode.DecodeJSON(req, &reqBody); err != nil {
		err = fmt.Errorf("decoding the request body: %w", err)
		log.Ctx(ctx).Error(err)
		httperror.BadRequest("", err, nil).Render(rw)
		return
	}

	if err := reqBody.validate(); err != nil {
		err.Render(rw)
		return
	}

	authenticatedUserID, err := h.AuthManager.GetUserID(ctx, token)
	if err != nil {
		err = fmt.Errorf("getting request authenticated user ID: %w", err)
		log.Ctx(ctx).Error(err)
		httperror.Unauthorized("", err, nil).Render(rw)
		return
	}

	updateUserRolesErr := h.AuthManager.UpdateUserRoles(ctx, token, reqBody.UserID, data.FromUserRoleArrayToStringArray(reqBody.Roles))
	if updateUserRolesErr != nil {
		if errors.Is(updateUserRolesErr, auth.ErrInvalidToken) {
			httperror.Unauthorized("", updateUserRolesErr, nil).Render(rw)
			return
		}

		if errors.Is(updateUserRolesErr, auth.ErrNoRowsAffected) {
			httperror.BadRequest("", updateUserRolesErr, map[string]interface{}{"user_id": "user_id is invalid"}).Render(rw)
			return
		}

		httperror.InternalError(ctx, "Cannot update user activation", updateUserRolesErr, nil).Render(rw)
		return
	}

	log.Ctx(ctx).Infof("[UpdateUserRoles] - User ID %s updated user with account ID %s roles to %v", authenticatedUserID, reqBody.UserID, reqBody.Roles)
	httpjson.RenderStatus(rw, http.StatusOK, map[string]string{"message": "user roles were updated successfully"}, httpjson.JSON)
}

func (h UserHandler) GetAllUsers(rw http.ResponseWriter, req *http.Request) {
	validator := validators.NewUserQueryValidator()
	queryParams := validator.ParseParametersFromRequest(req)
	if validator.HasErrors() {
		httperror.BadRequest("request invalid", nil, validator.Errors).Render(rw)
		return
	}

	ctx := req.Context()

	token, ok := ctx.Value(middleware.TokenContextKey).(string)
	if !ok {
		log.Ctx(ctx).Warn("token not found when getting all users")
		httperror.Unauthorized("", nil, nil).Render(rw)
		return
	}

	users, err := h.AuthManager.GetAllUsers(ctx, token)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidToken) {
			httperror.Unauthorized("", err, nil).Render(rw)
			return
		}
		httperror.InternalError(ctx, "Cannot get all users", err, nil).Render(rw)
		return
	}

	// Order users
	switch queryParams.SortBy {
	case data.SortFieldEmail:
		if queryParams.SortOrder == data.SortOrderDESC {
			sort.Sort(sort.Reverse(UserSorterByEmail(users)))
		} else {
			sort.Sort(UserSorterByEmail(users))
		}
	case data.SortFieldIsActive:
		if queryParams.SortOrder == data.SortOrderDESC {
			sort.Sort(sort.Reverse(UserSorterByIsActive(users)))
		} else {
			sort.Sort(UserSorterByIsActive(users))
		}
	}

	httpjson.RenderStatus(rw, http.StatusOK, users, httpjson.JSON)
}
