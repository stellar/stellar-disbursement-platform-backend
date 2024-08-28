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
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
)

type UpdateReceiverHandler struct {
	Models           *data.Models
	DBConnectionPool db.DBConnectionPool
}

func createVerificationInsert(updateReceiverInfo *validators.UpdateReceiverRequest, receiverID string) []data.ReceiverVerificationInsert {
	receiverVerifications := []data.ReceiverVerificationInsert{}
	appendNewVerificationValue := func(verificationField data.VerificationField, verificationValue string) {
		if verificationValue != "" {
			receiverVerifications = append(receiverVerifications, data.ReceiverVerificationInsert{
				ReceiverID:        receiverID,
				VerificationField: verificationField,
				VerificationValue: verificationValue,
			})
		}
	}

	for _, verificationField := range data.GetAllVerificationFields() {
		switch verificationField {
		case data.VerificationFieldDateOfBirth:
			appendNewVerificationValue(verificationField, updateReceiverInfo.DateOfBirth)
		case data.VerificationFieldYearMonth:
			appendNewVerificationValue(verificationField, updateReceiverInfo.YearMonth)
		case data.VerificationFieldPin:
			appendNewVerificationValue(verificationField, updateReceiverInfo.Pin)
		case data.VerificationFieldNationalID:
			appendNewVerificationValue(verificationField, updateReceiverInfo.NationalID)
		}
	}

	return receiverVerifications
}

func (h UpdateReceiverHandler) UpdateReceiver(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	var reqBody validators.UpdateReceiverRequest
	err := httpdecode.DecodeJSON(req, &reqBody)
	if err != nil {
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
				req.Context(),
				dbTx,
				rv.ReceiverID,
				rv.VerificationField,
				rv.VerificationValue,
			)

			if innerErr != nil {
				return nil, fmt.Errorf("error updating receiver verification %s: %w", rv.VerificationField, innerErr)
			}
		}

		receiverUpdate := data.ReceiverUpdate{}
		if reqBody.Email != "" {
			receiverUpdate.Email = &reqBody.Email
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
			return nil, fmt.Errorf("error querying receiver with ID %s: %w", receiverID, innerErr)
		}

		return receiver, nil
	})
	if err != nil {
		httperror.InternalError(ctx, "", err, nil).Render(rw)
		return
	}

	httpjson.Render(rw, receiver, httpjson.JSON)
}
