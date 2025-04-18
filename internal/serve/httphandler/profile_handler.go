package httphandler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"

	// Don't remove the `image/jpeg` and `image/png` packages import unless
	// the `image` package is no longer necessary.
	// It registers the `Decoders` to handle the image decoding - `image.Decode`.
	// See https://pkg.go.dev/image#pkg-overview
	_ "image/jpeg"
	_ "image/png"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/stellar/go/support/http/httpdecode"
	"github.com/stellar/go/support/log"
	"github.com/stellar/go/support/render/httpjson"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/middleware"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
	authUtils "github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

// DefaultMaxMemoryAllocation limits the max of memory allocation up to 2MB
// when parsing the multipart form data request
const DefaultMaxMemoryAllocation = 2 * 1024 * 1024

type ProfileHandler struct {
	Models                      *data.Models
	AuthManager                 auth.AuthManager
	MaxMemoryAllocation         int64
	DistributionAccountResolver signing.DistributionAccountResolver
	PasswordValidator           *authUtils.PasswordValidator
	utils.NetworkType
}

type PatchOrganizationProfileRequest struct {
	OrganizationName                    string  `json:"organization_name"`
	TimezoneUTCOffset                   string  `json:"timezone_utc_offset"`
	IsApprovalRequired                  *bool   `json:"is_approval_required"`
	IsLinkShortenerEnabled              *bool   `json:"is_link_shortener_enabled"`
	IsMemoTracingEnabled                *bool   `json:"is_memo_tracing_enabled"`
	ReceiverInvitationResendInterval    *int64  `json:"receiver_invitation_resend_interval_days"`
	PaymentCancellationPeriodDays       *int64  `json:"payment_cancellation_period_days"`
	ReceiverRegistrationMessageTemplate *string `json:"receiver_registration_message_template"`
	OTPMessageTemplate                  *string `json:"otp_message_template"`
	PrivacyPolicyLink                   *string `json:"privacy_policy_link"`
}

func (r *PatchOrganizationProfileRequest) AreAllFieldsEmpty() bool {
	return r == nil || utils.IsEmpty(*r)
}

type PatchUserProfileRequest struct {
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
}

type GetProfileResponse struct {
	ID               string   `json:"id"`
	FirstName        string   `json:"first_name"`
	LastName         string   `json:"last_name"`
	Email            string   `json:"email"`
	Roles            []string `json:"roles"`
	OrganizationName string   `json:"organization_name"`
}

type PatchUserPasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (h ProfileHandler) PatchOrganizationProfile(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	_, user, httpErr := getTokenAndUser(ctx, h.AuthManager)
	if httpErr != nil {
		httpErr.Render(rw)
		return
	}

	// limiting the size of the request
	req.Body = http.MaxBytesReader(rw, req.Body, h.MaxMemoryAllocation)

	// limiting the amount of memory allocated in the server to handle the request
	if err := req.ParseMultipartForm(h.MaxMemoryAllocation); err != nil {
		err = fmt.Errorf("parsing multipart form: %w", err)
		log.Ctx(ctx).Error(err)
		httperror.BadRequest("could not parse multipart form data", err, map[string]interface{}{
			"details": "request too large. Max size 2MB.",
		}).Render(rw)
		return
	}

	multipartFile, _, err := req.FormFile("logo")
	if err != nil && !errors.Is(err, http.ErrMissingFile) {
		err = fmt.Errorf("parsing logo file: %w", err)
		log.Ctx(ctx).Error(err)
		httperror.BadRequest("could not parse request logo", err, nil).Render(rw)
		return
	}

	var fileContentBytes []byte
	// a file is present in the request
	if multipartFile != nil {
		fileContentBytes, err = io.ReadAll(multipartFile)
		if err != nil {
			httperror.InternalError(ctx, "Cannot read file contents", err, nil).Render(rw)
			return
		}

		// We need to ensure the the type of file is one of the accepted - image/png and image/jpeg
		fileContentType := http.DetectContentType(fileContentBytes)

		validator := validators.NewValidator()
		expectedContentTypes := fmt.Sprintf("%s %s", data.PNGLogoType.ToHTTPContentType(), data.JPEGLogoType.ToHTTPContentType())
		validator.Check(strings.Contains(expectedContentTypes, fileContentType), "logo", "invalid file type provided. Expected png or jpeg.")
		if validator.HasErrors() {
			httperror.BadRequest("", nil, validator.Errors).Render(rw)
			return
		}
	}

	var reqBody PatchOrganizationProfileRequest
	d := req.FormValue("data")
	if err = json.Unmarshal([]byte(d), &reqBody); err != nil {
		err = fmt.Errorf("decoding data: %w", err)
		log.Ctx(ctx).Error(err)
		httperror.BadRequest("", err, nil).Render(rw)
		return
	}

	// validate wether the logo or the organization_name were sent in the request
	if len(fileContentBytes) == 0 && reqBody.AreAllFieldsEmpty() {
		httperror.BadRequest("request is invalid", nil, map[string]interface{}{
			"details": "data or logo is required",
		}).Render(rw)
		return
	}

	validator := validators.NewValidator()
	if reqBody.PrivacyPolicyLink != nil && *reqBody.PrivacyPolicyLink != "" {
		schemes := []string{"https"}
		if !h.IsPubnet() {
			schemes = append(schemes, "http")
		}
		validator.CheckError(utils.ValidateURLScheme(*reqBody.PrivacyPolicyLink, schemes...), "privacy_policy_link", "")
	}
	if reqBody.ReceiverRegistrationMessageTemplate != nil {
		validator.CheckError(utils.ValidateNoHTML(*reqBody.ReceiverRegistrationMessageTemplate), "receiver_registration_message_template", "receiver_registration_message_template cannot contain HTML, JS or CSS")
	}
	if validator.HasErrors() {
		httperror.BadRequest("", nil, validator.Errors).Render(rw)
		return
	}

	organizationUpdate := data.OrganizationUpdate{
		Name:                                 reqBody.OrganizationName,
		Logo:                                 fileContentBytes,
		TimezoneUTCOffset:                    reqBody.TimezoneUTCOffset,
		IsApprovalRequired:                   reqBody.IsApprovalRequired,
		IsLinkShortenerEnabled:               reqBody.IsLinkShortenerEnabled,
		IsMemoTracingEnabled:                 reqBody.IsMemoTracingEnabled,
		ReceiverRegistrationMessageTemplate:  reqBody.ReceiverRegistrationMessageTemplate,
		OTPMessageTemplate:                   reqBody.OTPMessageTemplate,
		ReceiverInvitationResendIntervalDays: reqBody.ReceiverInvitationResendInterval,
		PaymentCancellationPeriodDays:        reqBody.PaymentCancellationPeriodDays,
		PrivacyPolicyLink:                    reqBody.PrivacyPolicyLink,
	}
	requestDict, err := utils.ConvertType[data.OrganizationUpdate, map[string]interface{}](organizationUpdate)
	if err != nil {
		httperror.InternalError(ctx, "Cannot convert organization update to map", err, nil).Render(rw)
		return
	}
	var nonEmptyChanges []string
	for k, v := range requestDict {
		if k == "Logo" {
			v = "..."
		}
		nonEmptyChanges = append(nonEmptyChanges, fmt.Sprintf("%s='%v'", k, v))
	}
	sort.Strings(nonEmptyChanges)

	log.Ctx(ctx).Warnf("[PatchOrganizationProfile] - userID %s will update the organization fields [%s]", user.ID, strings.Join(nonEmptyChanges, ", "))
	err = h.Models.Organizations.Update(ctx, &organizationUpdate)
	if err != nil {
		httperror.InternalError(ctx, "Cannot update organization", err, nil).Render(rw)
		return
	}

	httpjson.RenderStatus(rw, http.StatusOK, map[string]string{"message": "updated successfully"}, httpjson.JSON)
}

func (h ProfileHandler) PatchUserProfile(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	token, user, httpErr := getTokenAndUser(ctx, h.AuthManager)
	if httpErr != nil {
		httpErr.Render(rw)
		return
	}

	var reqBody PatchUserProfileRequest
	if err := httpdecode.DecodeJSON(req, &reqBody); err != nil {
		err = fmt.Errorf("decoding the request body: %w", err)
		log.Ctx(ctx).Error(err)
		httperror.BadRequest("", err, nil).Render(rw)
		return
	}

	if reqBody.Email != "" {
		if err := utils.ValidateEmail(reqBody.Email); err != nil {
			httperror.BadRequest("", nil, map[string]interface{}{
				"email": "invalid email provided",
			}).Render(rw)
			return
		}
		log.Ctx(ctx).Warnf("[PatchUserProfile] - Will update email for userID %s to %s", user.ID, utils.TruncateString(reqBody.Email, 3))
	}

	if utils.IsEmpty(reqBody) {
		httperror.BadRequest("", nil, map[string]interface{}{
			"details": "provide at least first_name, last_name or email.",
		}).Render(rw)
		return
	}

	err := h.AuthManager.UpdateUser(ctx, token, reqBody.FirstName, reqBody.LastName, reqBody.Email, "")
	if err != nil {
		httperror.InternalError(ctx, "Cannot update user profiles", err, nil).Render(rw)
		return
	}

	httpjson.RenderStatus(rw, http.StatusOK, map[string]string{"message": "user profile updated successfully"}, httpjson.JSON)
}

func (h ProfileHandler) PatchUserPassword(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	token, user, httpErr := getTokenAndUser(ctx, h.AuthManager)
	if httpErr != nil {
		httpErr.Render(rw)
		return
	}

	var reqBody PatchUserPasswordRequest
	if err := httpdecode.DecodeJSON(req, &reqBody); err != nil {
		err = fmt.Errorf("decoding the request body: %w", err)
		log.Ctx(ctx).Error(err)
		httperror.BadRequest("", err, nil).Render(rw)
		return
	}

	// basic incoming parameters validation
	v := validators.NewValidator()
	v.Check(reqBody.CurrentPassword != "", "current_password", "current_password is required")
	v.Check(reqBody.CurrentPassword != reqBody.NewPassword, "new_password", "new_password should be different from current_password")
	if v.HasErrors() {
		httperror.BadRequest("", nil, v.Errors).Render(rw)
		return
	}

	// validate if the password format attends the requirements
	badRequestExtras := map[string]interface{}{}
	if validatePasswordError := h.PasswordValidator.ValidatePassword(reqBody.NewPassword); validatePasswordError != nil {
		for k, v := range validatePasswordError.FailedValidations() {
			badRequestExtras[k] = v
		}
	}
	if len(badRequestExtras) > 0 {
		httperror.BadRequest("", nil, badRequestExtras).Render(rw)
		return
	}

	log.Ctx(ctx).Warnf("[PatchUserPassword] - Will update password for user account ID %s", user.ID)
	err := h.AuthManager.UpdatePassword(ctx, token, reqBody.CurrentPassword, reqBody.NewPassword)
	if err != nil {
		httperror.InternalError(ctx, "Cannot update user password", err, nil).Render(rw)
		return
	}

	httpjson.RenderStatus(rw, http.StatusOK, map[string]string{"message": "user password updated successfully"}, httpjson.JSON)
}

func (h ProfileHandler) GetProfile(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	_, user, httpErr := getTokenAndUser(ctx, h.AuthManager)
	if httpErr != nil {
		httpErr.Render(rw)
		return
	}

	org, err := h.Models.Organizations.Get(ctx)
	if err != nil {
		httperror.InternalError(ctx, "Cannot get organization", err, nil).Render(rw)
		return
	}

	resp := &GetProfileResponse{
		ID:               user.ID,
		FirstName:        user.FirstName,
		LastName:         user.LastName,
		Email:            user.Email,
		Roles:            user.Roles,
		OrganizationName: org.Name,
	}
	httpjson.RenderStatus(rw, http.StatusOK, resp, httpjson.JSON)
}

func (h ProfileHandler) GetOrganizationInfo(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	org, err := h.Models.Organizations.Get(ctx)
	if err != nil {
		httperror.InternalError(ctx, "Cannot get organization", err, nil).Render(rw)
		return
	}

	currentTenant, err := tenant.GetTenantFromContext(ctx)
	if err != nil || currentTenant == nil {
		httperror.InternalError(ctx, "Cannot retrieve the tenant from the context", err, nil).Render(rw)
		return
	}

	logoURL, err := getLogoURL(currentTenant.BaseURL)
	if err != nil {
		httperror.InternalError(ctx, "Cannot get logo URL", err, nil).Render(rw)
		return
	}

	distributionAccount, err := h.DistributionAccountResolver.DistributionAccountFromContext(ctx)
	if err != nil {
		httperror.InternalError(ctx, "Cannot get distribution account", err, nil).Render(rw)
		return
	}

	resp := map[string]interface{}{
		"name":                                     org.Name,
		"logo_url":                                 logoURL,
		"base_url":                                 currentTenant.BaseURL,
		"distribution_account":                     distributionAccount,
		"distribution_account_public_key":          distributionAccount.Address, // TODO: deprecate `distribution_account_public_key`
		"timezone_utc_offset":                      org.TimezoneUTCOffset,
		"is_approval_required":                     org.IsApprovalRequired,
		"is_link_shortener_enabled":                org.IsLinkShortenerEnabled,
		"is_memo_tracing_enabled":                  org.IsMemoTracingEnabled,
		"receiver_invitation_resend_interval_days": 0,
		"payment_cancellation_period_days":         0,
		"privacy_policy_link":                      org.PrivacyPolicyLink,
		"message_channel_priority":                 org.MessageChannelPriority,
	}

	if org.ReceiverRegistrationMessageTemplate != data.DefaultReceiverRegistrationMessageTemplate {
		resp["receiver_registration_message_template"] = org.ReceiverRegistrationMessageTemplate
	}

	if org.OTPMessageTemplate != data.DefaultOTPMessageTemplate {
		resp["otp_message_template"] = org.OTPMessageTemplate
	}

	if org.ReceiverInvitationResendIntervalDays != nil {
		resp["receiver_invitation_resend_interval_days"] = *org.ReceiverInvitationResendIntervalDays
	}

	if org.PaymentCancellationPeriodDays != nil {
		resp["payment_cancellation_period_days"] = *org.PaymentCancellationPeriodDays
	}

	if org.PrivacyPolicyLink != nil {
		resp["privacy_policy_link"] = *org.PrivacyPolicyLink
	}

	httpjson.RenderStatus(rw, http.StatusOK, resp, httpjson.JSON)
}

func getLogoURL(baseURL *string) (string, error) {
	if baseURL == nil {
		return "", fmt.Errorf("baseURL is nil")
	}

	logoURL, err := url.JoinPath(*baseURL, "organization", "logo")
	if err != nil {
		return "", fmt.Errorf("constructing logo URL from base URL: %w", err)
	}

	lu, err := url.Parse(logoURL)
	if err != nil {
		return "", fmt.Errorf("parsing logo URL: %w", err)
	}

	return lu.String(), nil
}

type OrganizationLogoHandler struct {
	PublicFilesFS fs.FS
	Models        *data.Models
}

// GetOrganizationLogo renders the stored organization logo. The image is rendered inline (not attached - the attached option downloads the content)
// so the client can embed the image.
func (h OrganizationLogoHandler) GetOrganizationLogo(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	org, err := h.Models.Organizations.Get(ctx)
	if err != nil {
		httperror.InternalError(ctx, "Cannot get organization", err, nil).Render(rw)
		return
	}

	if len(org.Logo) == 0 {
		var logoBytes []byte
		logoBytes, err = fs.ReadFile(h.PublicFilesFS, "img/logo.png")
		if err != nil {
			httperror.InternalError(ctx, "Cannot open default logo", err, nil).Render(rw)
			return
		}

		org.Logo = logoBytes
	}

	_, ext, err := image.Decode(bytes.NewReader(org.Logo))
	if err != nil {
		httperror.InternalError(ctx, "Cannot decode organization logo", err, nil).Render(rw)
		return
	}

	rw.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, fmt.Sprintf("logo.%s", ext)))
	rw.Header().Set("Content-Type", http.DetectContentType(org.Logo))
	_, err = rw.Write(org.Logo)
	if err != nil {
		httperror.InternalError(ctx, "Cannot write organization logo to response", err, nil).Render(rw)
	}
}

func getTokenAndUser(ctx context.Context, authManager auth.AuthManager) (token string, user *auth.User, httpErr *httperror.HTTPError) {
	token, ok := ctx.Value(middleware.TokenContextKey).(string)
	if !ok {
		return "", nil, httperror.Unauthorized("", nil, nil)
	}

	user, err := authManager.GetUser(ctx, token)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidToken) {
			err = fmt.Errorf("getting user profile: %w", err)
			log.Ctx(ctx).Error(err)
			return "", nil, httperror.Unauthorized("", err, nil)
		}

		if errors.Is(err, auth.ErrUserNotFound) {
			err = fmt.Errorf("user from token %s not found: %w", token, err)
			log.Ctx(ctx).Error(err)
			return "", nil, httperror.BadRequest("", err, nil)
		}

		return "", nil, httperror.InternalError(ctx, "Cannot get user", err, nil)
	}

	return token, user, nil
}
