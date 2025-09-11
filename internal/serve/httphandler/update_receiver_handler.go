package httphandler

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/stellar/go/support/http/httpdecode"
	"github.com/stellar/go/support/log"
	"github.com/stellar/go/support/render/httpjson"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

type UpdateReceiverHandler struct {
	Models           *data.Models
	DBConnectionPool db.DBConnectionPool
	AuthManager      auth.AuthManager
}

func createVerificationInsert(updateReceiverInfo *validators.UpdateReceiverRequest, receiverID string) []data.ReceiverVerificationInsert {
	receiverVerifications := []data.ReceiverVerificationInsert{}
	appendNewVerificationValue := func(verificationField data.VerificationType, verificationValue string) {
		if verificationValue != "" {
			receiverVerifications = append(receiverVerifications, data.ReceiverVerificationInsert{
				ReceiverID:        receiverID,
				VerificationField: verificationField,
				VerificationValue: verificationValue,
			})
		}
	}

	for _, verificationField := range data.GetAllVerificationTypes() {
		switch verificationField {
		case data.VerificationTypeDateOfBirth:
			appendNewVerificationValue(verificationField, updateReceiverInfo.DateOfBirth)
		case data.VerificationTypeYearMonth:
			appendNewVerificationValue(verificationField, updateReceiverInfo.YearMonth)
		case data.VerificationTypePin:
			appendNewVerificationValue(verificationField, updateReceiverInfo.Pin)
		case data.VerificationTypeNationalID:
			appendNewVerificationValue(verificationField, updateReceiverInfo.NationalID)
		}
	}

	return receiverVerifications
}

func (h UpdateReceiverHandler) UpdateReceiver(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	userID, err := sdpcontext.GetUserIDFromContext(ctx)
	if err != nil {
		httperror.Unauthorized("", nil, nil).Render(rw)
		return
	}

	var reqBody validators.UpdateReceiverRequest
	if err = httpdecode.DecodeJSON(req, &reqBody); err != nil {
		err = fmt.Errorf("decoding the request body: %w", err)
		log.Ctx(ctx).Error(err)
		httperror.BadRequest("", err, nil).Render(rw)
		return
	}

	// validate request payload
	validator := validators.NewUpdateReceiverValidator()
	validator.ValidateReceiver(&reqBody)
	if validator.HasErrors() {
		log.Ctx(ctx).Errorf("request invalid: %s", validator.Errors)
		httperror.BadRequest("request invalid", nil, validator.Errors).Render(rw)
		return
	}

	receiverID := chi.URLParam(req, "id")
	_, err = h.Models.Receiver.Get(ctx, h.DBConnectionPool, receiverID)
	if err != nil {
		if errors.Is(err, data.ErrRecordNotFound) {
			httperror.NotFound("Receiver not found", err, nil).Render(rw)
		} else {
			httperror.InternalError(ctx, "Cannot retrieve receiver", err, nil).Render(rw)
		}
		return
	}

	receiverVerifications := createVerificationInsert(&reqBody, receiverID)
	receiver, err := db.RunInTransactionWithResult(ctx, h.DBConnectionPool, nil, func(dbTx db.DBTransaction) (response *data.Receiver, innerErr error) {
		for _, rv := range receiverVerifications {
			innerErr = h.Models.ReceiverVerification.UpsertVerificationValue(
				ctx,
				dbTx,
				userID,
				rv.ReceiverID,
				rv.VerificationField,
				rv.VerificationValue,
			)

			if innerErr != nil {
				return nil, fmt.Errorf("updating receiver verification %s: %w", rv.VerificationField, innerErr)
			}
		}

		var receiverUpdate data.ReceiverUpdate
		if reqBody.Email != "" {
			receiverUpdate.Email = &reqBody.Email
		}
		if reqBody.PhoneNumber != "" {
			receiverUpdate.PhoneNumber = &reqBody.PhoneNumber
		}
		if reqBody.ExternalID != "" {
			receiverUpdate.ExternalId = &reqBody.ExternalID
		}

		if !receiverUpdate.IsEmpty() {
			if innerErr = h.Models.Receiver.Update(ctx, dbTx, receiverID, receiverUpdate); innerErr != nil {
				return nil, fmt.Errorf("updating receiver with ID %s: %w", receiverID, innerErr)
			}
		}

		receiver, innerErr := h.Models.Receiver.Get(ctx, dbTx, receiverID)
		if innerErr != nil {
			return nil, fmt.Errorf("querying receiver with ID %s: %w", receiverID, innerErr)
		}

		return receiver, nil
	})
	if err != nil {
		if httpErr := parseConflictErrorIfNeeded(err); httpErr != nil {
			httpErr.Render(rw)
			return
		}

		httperror.InternalError(ctx, "", err, nil).Render(rw)
		return
	}

	httpjson.Render(rw, receiver, httpjson.JSON)
}

func parseConflictErrorIfNeeded(err error) *httperror.HTTPError {
	// Handle wallet address conflicts
	if errors.Is(err, data.ErrDuplicateStellarAddress) {
		return httperror.Conflict("The provided wallet address is already associated with another receiver.", err, map[string]interface{}{
			"wallet_address": "wallet address must be unique",
		})
	}

	// Handle email conflicts
	if errors.Is(err, data.ErrDuplicateEmail) {
		return httperror.Conflict("The provided email is already associated with another user.", err, map[string]interface{}{
			"email": "email must be unique",
		})
	}

	// Handle phone number conflicts
	if errors.Is(err, data.ErrDuplicatePhoneNumber) {
		return httperror.Conflict("The provided phone_number is already associated with another user.", err, map[string]interface{}{
			"phone_number": "phone_number must be unique",
		})
	}

	return nil
}
