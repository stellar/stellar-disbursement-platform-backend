package httphandler

import (
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

type ReceiverSendOTPHandler struct {
	Models             *data.Models
	SMSMessengerClient message.MessengerClient
	ReCAPTCHAValidator validators.ReCAPTCHAValidator
}

type ReceiverSendOTPData struct {
	OTP              string
	OrganizationName string
}

type ReceiverSendOTPRequest struct {
	PhoneNumber    string `json:"phone_number"`
	ReCAPTCHAToken string `json:"recaptcha_token"`
}

type ReceiverSendOTPResponseBody struct {
	Message string `json:"message"`
}

func (h ReceiverSendOTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	receiverSendOTPRequest := ReceiverSendOTPRequest{}

	err := json.NewDecoder(r.Body).Decode(&receiverSendOTPRequest)
	if err != nil {
		httperror.BadRequest("invalid request body", err, nil).Render(w)
		return
	}

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

	if phoneValidateErr := utils.ValidatePhoneNumber(receiverSendOTPRequest.PhoneNumber); phoneValidateErr != nil {
		extras := map[string]interface{}{"phone_number": "phone_number is required"}
		if !errors.Is(phoneValidateErr, utils.ErrEmptyPhoneNumber) {
			phoneValidateErr = fmt.Errorf("validating phone number %s: %w", utils.TruncateString(receiverSendOTPRequest.PhoneNumber, len(receiverSendOTPRequest.PhoneNumber)/4), phoneValidateErr)
			log.Ctx(ctx).Error(phoneValidateErr)
			extras["phone_number"] = "invalid phone number provided"
		}
		httperror.BadRequest("request invalid", phoneValidateErr, extras).Render(w)
		return
	}

	// Get clains from SEP24 JWT
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

	// Generate a new 6 digits OTP
	newOTP, err := utils.RandomString(6, utils.NumberBytes)
	if err != nil {
		httperror.InternalError(ctx, "Cannot generate OTP for receiver wallet", err, nil).Render(w)
		return
	}

	organization, err := h.Models.Organizations.Get(ctx)
	if err != nil {
		httperror.InternalError(ctx, "Cannot get organization", err, nil).Render(w)
		return
	}

	numberOfUpdatedRows, err := h.Models.ReceiverWallet.UpdateOTPByReceiverPhoneNumberAndWalletDomain(ctx, receiverSendOTPRequest.PhoneNumber, sep24Claims.ClientDomainClaim, newOTP)
	if err != nil {
		httperror.InternalError(ctx, "Cannot update OTP for receiver wallet", err, nil).Render(w)
		return
	}

	if numberOfUpdatedRows < 1 {
		log.Ctx(ctx).Warnf("updated no rows in receiver send OTP handler for phone number: %s", utils.TruncateString(receiverSendOTPRequest.PhoneNumber, len(receiverSendOTPRequest.PhoneNumber)/4))
	} else {
		sendOTPData := ReceiverSendOTPData{
			OTP:              newOTP,
			OrganizationName: organization.Name,
		}

		otpMessageTemplate := organization.OTPMessageTemplate
		if !strings.Contains(organization.OTPMessageTemplate, "{{.OTP}}") {
			// Adding the OTP code to the template
			otpMessageTemplate = fmt.Sprintf(`{{.OTP}} %s`, strings.TrimSpace(otpMessageTemplate))
		}

		sendOTPMessageTpl, err := template.New("").Parse(otpMessageTemplate)
		if err != nil {
			httperror.InternalError(ctx, "Cannot parse OTP template", err, nil).Render(w)
			return
		}

		builder := new(strings.Builder)
		if err = sendOTPMessageTpl.Execute(builder, sendOTPData); err != nil {
			httperror.InternalError(ctx, "Cannot execute OTP template", err, nil).Render(w)
			return
		}

		smsMessage := message.Message{
			ToPhoneNumber: receiverSendOTPRequest.PhoneNumber,
			Message:       builder.String(),
		}

		log.Ctx(ctx).Infof("sending OTP message to phone number: %s", utils.TruncateString(receiverSendOTPRequest.PhoneNumber, 3))
		err = h.SMSMessengerClient.SendMessage(smsMessage)
		if err != nil {
			httperror.InternalError(ctx, "Cannot send OTP message", err, nil).Render(w)
			return
		}
	}

	response := ReceiverSendOTPResponseBody{
		Message: "if your phone number is registered, you'll receive an OTP",
	}
	httpjson.RenderStatus(w, http.StatusOK, response, httpjson.JSON)
}
