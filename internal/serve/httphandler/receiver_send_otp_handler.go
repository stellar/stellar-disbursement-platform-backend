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

// OTPMessageDisclaimer contains disclaimer text that needs to be added as part of the OTP message to remind the
// receiver how sensitive the data is.
const OTPMessageDisclaimer = " If you did not request this code, please ignore. Do not share your code with anyone."

type OTPRegistrationType string

const (
	OTPRegistrationTypeSMS   OTPRegistrationType = "phone_number"
	OTPRegistrationTypeEmail OTPRegistrationType = "email"
)

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
	var otpRegistrationType OTPRegistrationType
	var verificationField data.VerificationType
	var httpErr *httperror.HTTPError
	if receiverSendOTPRequest.PhoneNumber != "" {
		otpRegistrationType = OTPRegistrationTypeSMS
		verificationField, httpErr = h.handleOTPForSMSReceiver(ctx, sep24Claims, receiverSendOTPRequest)
	} else {
		otpRegistrationType = OTPRegistrationTypeEmail
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
func newReceiverSendOTPResponseBody(otpRegistrationType OTPRegistrationType, verificationField data.VerificationType) ReceiverSendOTPResponseBody {
	resp := ReceiverSendOTPResponseBody{VerificationField: verificationField}

	switch otpRegistrationType {
	case OTPRegistrationTypeSMS:
		resp.Message = "if your phone number is registered, you'll receive an OTP"
	case OTPRegistrationTypeEmail:
		resp.Message = "if your email is registered, you'll receive an OTP"
	default:
		resp.Message = "if your contact information is registered, you'll receive an OTP"
	}

	return resp
}

// handleOTPForSMSReceiver handles the OTP generation and sending for a receiver with a phone number through SMS.
func (h ReceiverSendOTPHandler) handleOTPForSMSReceiver(ctx context.Context, sep24Claims *anchorplatform.SEP24JWTClaims, receiverSendOTPRequest ReceiverSendOTPRequest) (data.VerificationType, *httperror.HTTPError) {
	verificationField := data.VerificationTypeDateOfBirth
	var err error

	// Validate phone number
	truncatedPhoneNumber := utils.TruncateString(receiverSendOTPRequest.PhoneNumber, 3)
	if receiverSendOTPRequest.PhoneNumber != "" {
		if err = utils.ValidatePhoneNumber(receiverSendOTPRequest.PhoneNumber); err != nil {
			extras := map[string]interface{}{"phone_number": err.Error()}
			return verificationField, httperror.BadRequest("request invalid", err, extras)
		}
	}

	// get receiverVerification by that value phoneNumber
	if receiverVerification, err := h.Models.ReceiverVerification.GetLatestByPhoneNumber(ctx, receiverSendOTPRequest.PhoneNumber); err != nil {
		err = fmt.Errorf("cannot find latest receiver verification for phone number %s: %w", truncatedPhoneNumber, err)
		log.Ctx(ctx).Warn(err)
		return verificationField, nil
	} else {
		verificationField = receiverVerification.VerificationField
	}

	// Generate a new 6 digits OTP
	newOTP, err := utils.RandomString(6, utils.NumberBytes)
	if err != nil {
		return verificationField, httperror.InternalError(ctx, "Cannot generate OTP for receiver wallet", err, nil)
	}

	// Update OTP for receiver wallet
	numberOfUpdatedRows, err := h.Models.ReceiverWallet.UpdateOTPByReceiverPhoneNumberAndWalletDomain(ctx, receiverSendOTPRequest.PhoneNumber, sep24Claims.ClientDomainClaim, newOTP)
	if err != nil {
		return verificationField, httperror.InternalError(ctx, "Cannot update OTP for receiver wallet", err, nil)
	}
	if numberOfUpdatedRows < 1 {
		log.Ctx(ctx).Warnf("updated no rows in ReceiverSendOTPHandler, please verify if the provided OTP, phone number (%s) and client domain (%s) are valid", truncatedPhoneNumber, sep24Claims.ClientDomainClaim)
		return verificationField, nil
	}

	// Send OTP message
	err = h.sendSMSMessage(ctx, receiverSendOTPRequest.PhoneNumber, newOTP)
	if err != nil {
		return verificationField, httperror.InternalError(ctx, "Failed to send OTP message, reason: "+err.Error(), err, nil)
	}

	return verificationField, nil
}

// sendSMSMessage sends an OTP through an SMS message to the provided phone number.
func (h ReceiverSendOTPHandler) sendSMSMessage(ctx context.Context, phoneNumber, otp string) error {
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
		ToPhoneNumber: phoneNumber,
		Message:       builder.String(),
	}

	truncatedPhoneNumber := utils.TruncateString(phoneNumber, 3)
	log.Ctx(ctx).Infof("sending OTP message to phone number: %s", truncatedPhoneNumber)
	err = h.MessageDispatcher.SendMessage(ctx, msg, organization.MessageChannelPriority)
	if err != nil {
		return fmt.Errorf("cannot send OTP message: %w", err)
	}

	return nil
}

func (h ReceiverSendOTPHandler) HandleOTPForEmailReceiver(ctx context.Context, sep24Claims *anchorplatform.SEP24JWTClaims, receiverSendOTPRequest ReceiverSendOTPRequest) (data.VerificationType, *httperror.HTTPError) {
	verificationField := data.VerificationTypeDateOfBirth
	return verificationField, httperror.NewHTTPError(http.StatusNotImplemented, "Not implemented", nil, nil)
}
