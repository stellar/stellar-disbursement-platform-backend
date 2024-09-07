package httphandler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"github.com/stellar/go/support/log"
	"github.com/stellar/go/support/render/httpjson"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/anchorplatform"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

type OTPRegistrationType string

// OTPMessageDisclaimer contains disclaimer text that needs to be added as part of the OTP message to remind the
// receiver how sensitive the data is.
const OTPMessageDisclaimer = " If you did not request this code, please ignore. Do not share your code with anyone."

type ReceiverSendOTPHandler struct {
	Models             *data.Models
	MessageDispatcher  message.MessageDispatcherInterface
	ReCAPTCHAValidator validators.ReCAPTCHAValidator
}

type ReceiverSendOTPData struct {
	OTP              string
	OrganizationName string
}

type ReceiverSendOTPRequest struct {
	PhoneNumber    string `json:"phone_number"`
	Email          string `json:"email"`
	ReCAPTCHAToken string `json:"recaptcha_token"`
}

// validateContactInfo validates the contact information provided in the ReceiverSendOTPRequest. It ensures that either
// the phone number or email is provided, but not both. It also validates the phone number and email format.
// TODO: use a validator instead!
func (r ReceiverSendOTPRequest) validateContactInfo() *httperror.HTTPError {
	r.Email = utils.TrimAndLower(r.Email)
	r.PhoneNumber = utils.TrimAndLower(r.PhoneNumber)

	switch {
	case r.PhoneNumber == "" && r.Email == "":
		extras := map[string]interface{}{"phone_number": "phone_number or email is required", "email": "phone_number or email is required"}
		return httperror.BadRequest("", nil, extras)

	case r.PhoneNumber != "" && r.Email != "":
		extras := map[string]interface{}{"phone_number": "phone_number and email cannot be both provided", "email": "phone_number and email cannot be both provided"}
		return httperror.BadRequest("", nil, extras)

	case r.PhoneNumber != "":
		if err := utils.ValidatePhoneNumber(r.PhoneNumber); err != nil {
			extras := map[string]interface{}{"phone_number": err.Error()}
			return httperror.BadRequest("", err, extras)
		}
	case r.Email != "":
		if err := utils.ValidateEmail(r.Email); err != nil {
			extras := map[string]interface{}{"email": err.Error()}
			return httperror.BadRequest("", err, extras)
		}
	}

	return nil
}

type ReceiverSendOTPResponseBody struct {
	Message           string                `json:"message"`
	VerificationField data.VerificationType `json:"verification_field"`
}

func (h ReceiverSendOTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse request body
	receiverSendOTPRequest := ReceiverSendOTPRequest{}
	err := json.NewDecoder(r.Body).Decode(&receiverSendOTPRequest)
	if err != nil {
		httperror.BadRequest("invalid request body", err, nil).Render(w)
		return
	}
	receiverSendOTPRequest.PhoneNumber = utils.TrimAndLower(receiverSendOTPRequest.PhoneNumber)
	receiverSendOTPRequest.Email = utils.TrimAndLower(receiverSendOTPRequest.Email)

	// validating reCAPTCHA Token
	isValid, err := h.ReCAPTCHAValidator.IsTokenValid(ctx, receiverSendOTPRequest.ReCAPTCHAToken)
	if err != nil {
		httperror.InternalError(ctx, "Cannot validate reCAPTCHA token", err, nil).Render(w)
		return
	}
	if !isValid {
		log.Ctx(ctx).Errorf("reCAPTCHA token is invalid")
		httperror.BadRequest("reCAPTCHA token is invalid", nil, nil).Render(w)
		return
	}

	// Validate SEP-24 JWT claims
	sep24Claims := anchorplatform.GetSEP24Claims(ctx)
	if sep24Claims == nil {
		err = fmt.Errorf("no SEP-24 claims found in the request context")
		log.Ctx(ctx).Error(err)
		httperror.Unauthorized("", err, nil).Render(w)
		return
	}
	err = sep24Claims.Valid()
	if err != nil {
		err = fmt.Errorf("SEP-24 claims are invalid: %w", err)
		log.Ctx(ctx).Error(err)
		httperror.Unauthorized("", err, nil).Render(w)
		return
	}

	// Ensure XOR(PhoneNumber, Email)
	if httpErr := receiverSendOTPRequest.validateContactInfo(); httpErr != nil {
		httpErr.Render(w)
		return
	}

	// Determine the contact type and handle accordingly
	var contactType data.ReceiverContactType
	var contactInfo string
	if receiverSendOTPRequest.PhoneNumber != "" {
		contactType, contactInfo = data.ReceiverContactTypeSMS, receiverSendOTPRequest.PhoneNumber
	} else if receiverSendOTPRequest.Email != "" {
		contactType, contactInfo = data.ReceiverContactTypeEmail, receiverSendOTPRequest.Email
	} else {
		httperror.InternalError(ctx, "unexpected contact info", nil, nil).Render(w)
		return
	}
	verificationField, httpErr := h.handleOTPForReceiver(ctx, contactType, contactInfo, sep24Claims.ClientDomainClaim)
	if httpErr != nil {
		httpErr.Render(w)
		return
	}

	response := newReceiverSendOTPResponseBody(contactType, verificationField)
	httpjson.RenderStatus(w, http.StatusOK, response, httpjson.JSON)
}

// newReceiverSendOTPResponseBody creates a new ReceiverSendOTPResponseBody based on the OTP registration type and verification field.
func newReceiverSendOTPResponseBody(contactType data.ReceiverContactType, verificationField data.VerificationType) ReceiverSendOTPResponseBody {
	resp := ReceiverSendOTPResponseBody{VerificationField: verificationField}

	switch contactType {
	case data.ReceiverContactTypeSMS:
		resp.Message = "if your phone number is registered, you'll receive an OTP"
	case data.ReceiverContactTypeEmail:
		resp.Message = "if your email is registered, you'll receive an OTP"
	}

	return resp
}

// handleOTPReceiver handles the OTP generation and sending for a receiver with the provided contactType and contactInfo.
func (h ReceiverSendOTPHandler) handleOTPForReceiver(
	ctx context.Context,
	contactType data.ReceiverContactType,
	contactInfo string,
	sep24ClientDomain string,
) (data.VerificationType, *httperror.HTTPError) {
	var err error
	placeholderVerificationField := data.VerificationTypeDateOfBirth
	truncatedContactInfo := utils.TruncateString(contactInfo, 3)
	contactTypeStr := utils.HumanizeString(string(contactType))

	// get receiverVerification by that value of contactInfo
	receiverVerification, err := h.Models.ReceiverVerification.GetLatestByContactInfo(ctx, contactInfo)
	if err != nil {
		log.Ctx(ctx).Warnf("cannot find ANY receiver verification for %s %s: %v", contactTypeStr, truncatedContactInfo, err)
		return placeholderVerificationField, nil
	}

	// Generate a new 6 digits OTP
	newOTP, err := utils.RandomString(6, utils.NumberBytes)
	if err != nil {
		return placeholderVerificationField, httperror.InternalError(ctx, "Cannot generate OTP for receiver wallet", err, nil)
	}

	// Update OTP for receiver wallet
	numberOfUpdatedRows, err := h.Models.ReceiverWallet.UpdateOTPByReceiverContactInfoAndWalletDomain(ctx, contactInfo, sep24ClientDomain, newOTP)
	if err != nil && !errors.Is(err, data.ErrRecordNotFound) {
		return placeholderVerificationField, httperror.InternalError(ctx, "Cannot update OTP for receiver wallet", err, nil)
	}
	if numberOfUpdatedRows < 1 {
		log.Ctx(ctx).Warnf("could not find a match between %s (%s) and client domain (%s)", contactTypeStr, truncatedContactInfo, sep24ClientDomain)
		return placeholderVerificationField, nil
	}

	// Send OTP message
	err = h.sendOTP(ctx, contactType, contactInfo, newOTP)
	if err != nil {
		err = fmt.Errorf("sending OTP message: %w", err)
		return placeholderVerificationField, httperror.InternalError(ctx, "Failed to send OTP message, reason: "+err.Error(), err, nil)
	}

	return receiverVerification.VerificationField, nil
}

// sendOTP sends an OTP through the provided contact type to the provided contact information.
func (h ReceiverSendOTPHandler) sendOTP(ctx context.Context, contactType data.ReceiverContactType, contactInfo, otp string) error {
	organization, err := h.Models.Organizations.Get(ctx)
	if err != nil {
		return fmt.Errorf("cannot get organization: %w", err)
	}

	otpMessageTemplate := organization.OTPMessageTemplate + OTPMessageDisclaimer
	if !strings.Contains(organization.OTPMessageTemplate, "{{.OTP}}") {
		// Adding the OTP code to the template
		otpMessageTemplate = fmt.Sprintf(`{{.OTP}} %s`, strings.TrimSpace(otpMessageTemplate))
	}

	sendOTPMessageTpl, err := template.New("").Parse(otpMessageTemplate)
	if err != nil {
		return fmt.Errorf("cannot parse OTP template: %w", err)
	}

	sendOTPData := ReceiverSendOTPData{
		OTP:              otp,
		OrganizationName: organization.Name,
	}

	builder := new(strings.Builder)
	if err = sendOTPMessageTpl.Execute(builder, sendOTPData); err != nil {
		return fmt.Errorf("cannot execute OTP template: %w", err)
	}

	msg := message.Message{Message: builder.String()}
	switch contactType {
	case data.ReceiverContactTypeSMS:
		msg.ToPhoneNumber = contactInfo
	case data.ReceiverContactTypeEmail:
		msg.ToEmail = contactInfo
		msg.Title = "Your One-Time Password: " + otp
	}

	truncatedContactInfo := utils.TruncateString(contactInfo, 3)
	contactTypeStr := utils.HumanizeString(string(contactType))
	log.Ctx(ctx).Infof("sending OTP message to %s %s", contactTypeStr, truncatedContactInfo)
	err = h.MessageDispatcher.SendMessage(ctx, msg, organization.MessageChannelPriority)
	if err != nil {
		return fmt.Errorf("cannot send OTP message through %s: %w", contactTypeStr, err)
	}

	return nil
}
