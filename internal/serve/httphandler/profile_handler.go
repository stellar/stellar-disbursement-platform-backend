package httphandler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"sort"

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
	"strings"

	"github.com/stellar/go/support/http/httpdecode"
	"github.com/stellar/go/support/log"
	"github.com/stellar/go/support/render/httpjson"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/middleware"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
	authUtils "github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/utils"
)

// DefaultMaxMemoryAllocation limits the max of memory allocation up to 2MB
// when parsing the multipart form data request
const DefaultMaxMemoryAllocation = 2 * 1024 * 1024

type ProfileHandler struct {
	Models                *data.Models
	AuthManager           auth.AuthManager
	MaxMemoryAllocation   int64
	BaseURL               string
	PublicFilesFS         fs.FS
	DistributionPublicKey string
	PasswordValidator     *authUtils.PasswordValidator
}

type PatchOrganizationProfileRequest struct {
	OrganizationName               string  `json:"organization_name"`
	TimezoneUTCOffset              string  `json:"timezone_utc_offset"`
	IsApprovalRequired             *bool   `json:"is_approval_required"`
	SMSResendInterval              *int64  `json:"sms_resend_interval"`
	PaymentCancellationPeriodDays  *int64  `json:"payment_cancellation_period_days"`
	SMSRegistrationMessageTemplate *string `json:"sms_registration_message_template"`
	OTPMessageTemplate             *string `json:"otp_message_template"`
}

func (r *PatchOrganizationProfileRequest) AreAllFieldsEmpty() bool {
	return (r.OrganizationName == "" &&
		r.TimezoneUTCOffset == "" &&
		r.IsApprovalRequired == nil &&
		r.SMSRegistrationMessageTemplate == nil &&
		r.OTPMessageTemplate == nil &&
		r.SMSResendInterval == nil &&
		r.PaymentCancellationPeriodDays == nil)
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
		err = fmt.Errorf("error parsing multipart form: %w", err)
		log.Ctx(ctx).Error(err)
		httperror.BadRequest("could not parse multipart form data", err, map[string]interface{}{
			"details": "request too large. Max size 2MB.",
		}).Render(rw)
		return
	}

	multipartFile, _, err := req.FormFile("logo")
	if err != nil && !errors.Is(err, http.ErrMissingFile) {
		err = fmt.Errorf("error parsing logo file: %w", err)
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
		err = fmt.Errorf("error decoding data: %w", err)
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

	organizationUpdate := data.OrganizationUpdate{
		Name:                           reqBody.OrganizationName,
		Logo:                           fileContentBytes,
		TimezoneUTCOffset:              reqBody.TimezoneUTCOffset,
		IsApprovalRequired:             reqBody.IsApprovalRequired,
		SMSRegistrationMessageTemplate: reqBody.SMSRegistrationMessageTemplate,
		OTPMessageTemplate:             reqBody.OTPMessageTemplate,
		SMSResendInterval:              reqBody.SMSResendInterval,
		PaymentCancellationPeriodDays:  reqBody.PaymentCancellationPeriodDays,
	}
	requestDict, err := utils.ConvertType[data.OrganizationUpdate, map[string]interface{}](organizationUpdate)
	if err != nil {
		httperror.InternalError(ctx, "Cannot convert organization update to map", err, nil).Render(rw)
		return
	}
	var nonEmptyChanges []string
	for k, v := range requestDict {
		if !utils.IsEmpty(v) {
			value := v
			if k == "Logo" {
				value = "..."
			}
			nonEmptyChanges = append(nonEmptyChanges, fmt.Sprintf("%s='%v'", k, value))
		}
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
	err := h.PasswordValidator.ValidatePassword(reqBody.NewPassword)
	if err != nil {
		var validatePasswordError *authUtils.ValidatePasswordError
		if errors.As(err, &validatePasswordError) {
			for k, v := range validatePasswordError.FailedValidations() {
				badRequestExtras[k] = v
			}
			log.Ctx(ctx).Errorf("validating password in PatchUserPassword: %v", err)
		} else {
			httperror.InternalError(ctx, "Cannot update user password", err, nil).Render(rw)
			return
		}
	}
	if len(badRequestExtras) > 0 {
		httperror.BadRequest("", nil, badRequestExtras).Render(rw)
		return
	}

	log.Ctx(ctx).Warnf("[PatchUserPassword] - Will update password for user account ID %s", user.ID)
	err = h.AuthManager.UpdatePassword(ctx, token, reqBody.CurrentPassword, reqBody.NewPassword)
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

	token, ok := ctx.Value(middleware.TokenContextKey).(string)
	if !ok {
		httperror.Unauthorized("", nil, nil).Render(rw)
		return
	}

	// We first build the logo URL so we don't hit the database if any error occurs.
	logoURL, err := url.JoinPath(h.BaseURL, "organization", "logo")
	if err != nil {
		httperror.InternalError(ctx, "Cannot get logo URL", err, nil).Render(rw)
		return
	}

	lu, err := url.Parse(logoURL)
	if err != nil {
		httperror.InternalError(ctx, "Cannot parse logo URL", err, nil).Render(rw)
		return
	}

	q := lu.Query()
	q.Add("token", token)
	lu.RawQuery = q.Encode()

	org, err := h.Models.Organizations.Get(ctx)
	if err != nil {
		httperror.InternalError(ctx, "Cannot get organization", err, nil).Render(rw)
		return
	}

	resp := map[string]interface{}{
		"name":                             org.Name,
		"logo_url":                         lu.String(),
		"distribution_account_public_key":  h.DistributionPublicKey,
		"timezone_utc_offset":              org.TimezoneUTCOffset,
		"is_approval_required":             org.IsApprovalRequired,
		"sms_resend_interval":              0,
		"payment_cancellation_period_days": 0,
	}

	if org.SMSRegistrationMessageTemplate != data.DefaultSMSRegistrationMessageTemplate {
		resp["sms_registration_message_template"] = org.SMSRegistrationMessageTemplate
	}

	if org.OTPMessageTemplate != data.DefaultOTPMessageTemplate {
		resp["otp_message_template"] = org.OTPMessageTemplate
	}

	if org.SMSResendInterval != nil {
		resp["sms_resend_interval"] = *org.SMSResendInterval
	}

	if org.PaymentCancellationPeriodDays != nil {
		resp["payment_cancellation_period_days"] = *org.PaymentCancellationPeriodDays
	}

	httpjson.RenderStatus(rw, http.StatusOK, resp, httpjson.JSON)
}

// GetOrganizationLogo renders the stored organization logo. The image is rendered inline (not attached - the attached option downloads the content)
// so the client can embed the image.
func (h ProfileHandler) GetOrganizationLogo(rw http.ResponseWriter, req *http.Request) {
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
