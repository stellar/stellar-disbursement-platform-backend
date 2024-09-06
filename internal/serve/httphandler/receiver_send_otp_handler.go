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
	if receiverSendOTPRequest.PhoneNumber == "" && receiverSendOTPRequest.Email == "" {
		httperror.BadRequest("request invalid", errors.New("phone_number or email is required"), nil).Render(w)
		return
	}
	if receiverSendOTPRequest.PhoneNumber != "" && receiverSendOTPRequest.Email != "" {
		httperror.BadRequest("request invalid", errors.New("phone_number and email cannot be both provided"), nil).Render(w)
		return
	}

	// Determine OTP registration type and handle accordingly
	var otpRegistrationType data.ReceiverContactType
	var verificationField data.VerificationType
	var httpErr *httperror.HTTPError
	if receiverSendOTPRequest.PhoneNumber != "" {
		otpRegistrationType = data.ReceiverContactTypeSMS
		verificationField, httpErr = h.handleOTPForSMSReceiver(ctx, receiverSendOTPRequest.PhoneNumber, sep24Claims.ClientDomainClaim)
	} else {
		otpRegistrationType = data.ReceiverContactTypeEmail
		verificationField, httpErr = h.HandleOTPForEmailReceiver(ctx, sep24Claims, receiverSendOTPRequest)
	}
	if httpErr != nil {
		httpErr.Render(w)
		return
	}

	response := newReceiverSendOTPResponseBody(otpRegistrationType, verificationField)
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

// handleOTPForSMSReceiver handles the OTP generation and sending for a receiver with a phone number through SMS.
func (h ReceiverSendOTPHandler) handleOTPForSMSReceiver(
	ctx context.Context,
	phoneNumber string,
	sep24ClientDomain string,
) (data.VerificationType, *httperror.HTTPError) {
	placeholderVerificationField := data.VerificationTypeDateOfBirth
	var err error

	// Validate phone number
	truncatedPhoneNumber := utils.TruncateString(phoneNumber, 3)
	if err = utils.ValidatePhoneNumber(phoneNumber); err != nil {
		extras := map[string]interface{}{"phone_number": err.Error()}
		return placeholderVerificationField, httperror.BadRequest("", err, extras)
	}

	// get receiverVerification by that value phoneNumber
	receiverVerification, err := h.Models.ReceiverVerification.GetLatestByPhoneNumber(ctx, phoneNumber)
	if err != nil {
		err = fmt.Errorf("cannot find latest receiver verification for phone number %s: %w", truncatedPhoneNumber, err)
		log.Ctx(ctx).Warn(err)
		return placeholderVerificationField, nil
	}

	// Generate a new 6 digits OTP
	newOTP, err := utils.RandomString(6, utils.NumberBytes)
	if err != nil {
		return placeholderVerificationField, httperror.InternalError(ctx, "Cannot generate OTP for receiver wallet", err, nil)
	}

	// Update OTP for receiver wallet
	numberOfUpdatedRows, err := h.Models.ReceiverWallet.UpdateOTPByReceiverPhoneNumberAndWalletDomain(ctx, phoneNumber, sep24ClientDomain, newOTP)
	if err != nil {
		return placeholderVerificationField, httperror.InternalError(ctx, "Cannot update OTP for receiver wallet", err, nil)
	}
	if numberOfUpdatedRows < 1 {
		log.Ctx(ctx).Warnf("updated no rows in ReceiverSendOTPHandler, please verify if the provided phone number (%s) and client domain (%s) are valid", truncatedPhoneNumber, sep24ClientDomain)
		return placeholderVerificationField, nil
	}

	// Send OTP message
	err = h.sendOTP(ctx, data.ReceiverContactTypeSMS, phoneNumber, newOTP)
	if err != nil {
		err = fmt.Errorf("sending SMS message: %w", err)
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
	log.Ctx(ctx).Infof("sending OTP message to %s: %s", utils.HumanizeString(string(contactType)), truncatedContactInfo)
	err = h.MessageDispatcher.SendMessage(ctx, msg, organization.MessageChannelPriority)
	if err != nil {
		return fmt.Errorf("cannot send OTP message through %s: %w", utils.HumanizeString(string(contactType)), err)
	}

	return nil
}

func (h ReceiverSendOTPHandler) HandleOTPForEmailReceiver(ctx context.Context, sep24Claims *anchorplatform.SEP24JWTClaims, receiverSendOTPRequest ReceiverSendOTPRequest) (data.VerificationType, *httperror.HTTPError) {
	verificationField := data.VerificationTypeDateOfBirth
	return verificationField, httperror.NewHTTPError(http.StatusNotImplemented, "Not implemented", nil, nil)
}
