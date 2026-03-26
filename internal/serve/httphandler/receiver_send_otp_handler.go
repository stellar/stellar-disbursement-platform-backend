package httphandler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/stellar/go-stellar-sdk/support/log"
	"github.com/stellar/go-stellar-sdk/support/render/httpjson"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sepauth"
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
	ReCAPTCHADisabled  bool
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
func (r ReceiverSendOTPRequest) validateContactInfo() validators.Validator {
	v := *validators.NewValidator()
	r.Email = utils.TrimAndLower(r.Email)
	r.PhoneNumber = utils.TrimAndLower(r.PhoneNumber)

	switch {
	case r.PhoneNumber == "" && r.Email == "":
		v.Check(false, "phone_number", "phone_number or email is required")
		v.Check(false, "email", "phone_number or email is required")
	case r.PhoneNumber != "" && r.Email != "":
		v.Check(false, "phone_number", "phone_number and email cannot be both provided")
		v.Check(false, "email", "phone_number and email cannot be both provided")
	case r.PhoneNumber != "":
		v.CheckError(utils.ValidatePhoneNumber(r.PhoneNumber), "phone_number", "")
	case r.Email != "":
		v.CheckError(utils.ValidateEmail(r.Email), "email", "")
	}

	return v
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
		httperror.BadRequest("invalid request body", err, nil).WithErrorCode(httperror.Code400_0).Render(w)
		return
	}
	receiverSendOTPRequest.PhoneNumber = utils.TrimAndLower(receiverSendOTPRequest.PhoneNumber)
	receiverSendOTPRequest.Email = utils.TrimAndLower(receiverSendOTPRequest.Email)

	// validating reCAPTCHA Token if it is enabled
	if !IsCAPTCHADisabled(ctx, CAPTCHAConfig{
		Models:            h.Models,
		ReCAPTCHADisabled: h.ReCAPTCHADisabled,
	}) {
		isValid, tokenErr := h.ReCAPTCHAValidator.IsTokenValid(ctx, receiverSendOTPRequest.ReCAPTCHAToken)
		if tokenErr != nil {
			httperror.InternalError(ctx, "Cannot validate reCAPTCHA token", tokenErr, nil).WithErrorCode(httperror.Code500_5).Render(w)
			return
		}
		if !isValid {
			log.Ctx(ctx).Errorf("reCAPTCHA token is invalid")
			httperror.BadRequest("reCAPTCHA token is invalid", nil, nil).WithErrorCode(httperror.Code400_1).Render(w)
			return
		}
	}

	// Validate SEP-24 JWT claims
	sep24Claims := sepauth.GetSEP24Claims(ctx)
	if sep24Claims == nil {
		err = fmt.Errorf("no SEP-24 claims found in the request context")
		log.Ctx(ctx).Error(err)
		httperror.Unauthorized("", err, nil).WithErrorCode(httperror.Code401_0).Render(w)
		return
	}
	err = sep24Claims.Valid()
	if err != nil {
		err = fmt.Errorf("SEP-24 claims are invalid: %w", err)
		log.Ctx(ctx).Error(err)
		httperror.Unauthorized("", err, nil).WithErrorCode(httperror.Code401_0).Render(w)
		return
	}

	// Ensure XOR(PhoneNumber, Email)
	if v := receiverSendOTPRequest.validateContactInfo(); v.HasErrors() {
		// TODO: how to manage these extras?
		httperror.BadRequest("", nil, v.Errors).WithErrorCode(httperror.Code400_0).Render(w)
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
		httperror.InternalError(ctx, "Unexpected contact info", nil, nil).WithErrorCode(httperror.Code500_6).Render(w)
		return
	}
	verificationField, httpErr := h.handleOTPForReceiver(ctx, contactType, contactInfo, sep24Claims.ClientDomainClaim)
	if httpErr != nil {
		httpErr.Render(w)
		return
	}

	response := newReceiverSendOTPResponseBody(contactType, verificationField)
	httpjson.Render(w, response, httpjson.JSON)
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
	contactTypeStr := utils.Humanize(string(contactType))

	// get receiverVerification by that value of contactInfo
	receiverVerification, err := h.Models.ReceiverVerification.GetLatestByContactInfo(ctx, contactInfo)
	if err != nil {
		log.Ctx(ctx).Warnf("Could not find ANY receiver verification for %s %s: %v", contactTypeStr, truncatedContactInfo, err)
		h.recordRegistrationAttempt(ctx, contactType, contactInfo)
		return placeholderVerificationField, nil
	}

	// Generate a new 6 digits OTP
	newOTP, err := utils.RandomString(6, utils.NumberBytes)
	if err != nil {
		return placeholderVerificationField, httperror.InternalError(ctx, "Cannot generate OTP for receiver wallet", err, nil).WithErrorCode(httperror.Code500_7)
	}

	// Update OTP for receiver wallet
	numberOfUpdatedRows, err := h.Models.ReceiverWallet.UpdateOTPByReceiverContactInfoAndWalletDomain(ctx, contactInfo, sep24ClientDomain, newOTP)
	if err != nil && !errors.Is(err, data.ErrRecordNotFound) {
		return placeholderVerificationField, httperror.InternalError(ctx, "Cannot update OTP for receiver wallet", err, nil).WithErrorCode(httperror.Code500_8)
	}
	if numberOfUpdatedRows < 1 {
		log.Ctx(ctx).Warnf("Could not find a match between %s (%s) and client domain (%s)", contactTypeStr, truncatedContactInfo, sep24ClientDomain)
		h.recordRegistrationAttempt(ctx, contactType, contactInfo)
		return placeholderVerificationField, nil
	}

	// Send OTP message
	if err = h.sendOTP(ctx, contactType, contactInfo, newOTP); err != nil {
		err = fmt.Errorf("sending OTP message: %w", err)
		return placeholderVerificationField, httperror.InternalError(ctx, "Failed to send OTP message, reason: "+err.Error(), err, nil).WithErrorCode(httperror.Code500_9)
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

	msg := message.Message{
		Type: message.MessageTypeReceiverOTP,
		Body: builder.String(),
		TemplateVariables: map[message.TemplateVariable]string{
			message.TemplateVarReceiverOTP: otp,
			message.TemplateVarOrgName:     organization.Name,
		},
	}
	switch contactType {
	case data.ReceiverContactTypeSMS:
		msg.ToPhoneNumber = contactInfo
	case data.ReceiverContactTypeEmail:
		msg.ToEmail = contactInfo
		msg.Title = "Your One-Time Password: " + otp
	}

	truncatedContactInfo := utils.TruncateString(contactInfo, 3)
	contactTypeStr := utils.Humanize(string(contactType))
	log.Ctx(ctx).Infof("sending OTP message to %s %s...", contactTypeStr, truncatedContactInfo)
	_, err = h.MessageDispatcher.SendMessage(ctx, msg, organization.MessageChannelPriority)
	if err != nil {
		return fmt.Errorf("cannot send OTP message through %s to %s: %w", contactTypeStr, truncatedContactInfo, err)
	}

	return nil
}

func (h ReceiverSendOTPHandler) recordRegistrationAttempt(
	ctx context.Context,
	contactType data.ReceiverContactType,
	contactInfo string,
) {
	claims := sepauth.GetSEP24Claims(ctx)
	attempt := data.ReceiverRegistrationAttempt{
		PhoneNumber:   "",
		Email:         "",
		AttemptTS:     time.Now(),
		ClientDomain:  claims.ClientDomain(),
		TransactionID: claims.TransactionID(),
		WalletAddress: claims.Account(),
		WalletMemo:    claims.Memo(),
	}

	switch contactType {
	case data.ReceiverContactTypeSMS:
		attempt.PhoneNumber = contactInfo
	case data.ReceiverContactTypeEmail:
		attempt.Email = contactInfo
	}

	if err := h.Models.ReceiverRegistrationAttempt.InsertReceiverRegistrationAttempt(ctx, attempt); err != nil {
		log.Ctx(ctx).Errorf("failed to record registration attempt: %v", err)
	}
}
