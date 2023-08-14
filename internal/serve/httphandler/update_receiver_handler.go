package httphandler

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/stellar/go/support/http/httpdecode"
	"github.com/stellar/go/support/log"
	"github.com/stellar/go/support/render/httpjson"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
)

type UpdateReceiverHandler struct {
	Models           *data.Models
	DBConnectionPool db.DBConnectionPool
}

func createVerificationInsert(updateReceiverInfo *validators.UpdateReceiverRequest, receiverID string) []data.ReceiverVerificationInsert {
	receiverVerifications := []data.ReceiverVerificationInsert{}

	if updateReceiverInfo.DateOfBirth != "" {
		receiverVerifications = append(receiverVerifications, data.ReceiverVerificationInsert{
			ReceiverID:        receiverID,
			VerificationField: data.VerificationFieldDateOfBirth,
			VerificationValue: updateReceiverInfo.DateOfBirth,
		})
	}

	if updateReceiverInfo.Pin != "" {
		receiverVerifications = append(receiverVerifications, data.ReceiverVerificationInsert{
			ReceiverID:        receiverID,
			VerificationField: data.VerificationFieldPin,
			VerificationValue: updateReceiverInfo.Pin,
		})
	}

	if updateReceiverInfo.NationalID != "" {
		receiverVerifications = append(receiverVerifications, data.ReceiverVerificationInsert{
			ReceiverID:        receiverID,
			VerificationField: data.VerificationFieldNationalID,
			VerificationValue: updateReceiverInfo.NationalID,
		})
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
	receiverVerifications := createVerificationInsert(&reqBody, receiverID)
	receiver, err := db.RunInTransactionWithResult(ctx, h.DBConnectionPool, nil, func(dbTx db.DBTransaction) (response *data.Receiver, innerErr error) {
		for _, rv := range receiverVerifications {
			innerErr = h.Models.ReceiverVerification.UpdateVerificationValue(
				req.Context(),
				h.Models.DBConnectionPool,
				rv.ReceiverID,
				rv.VerificationField,
				rv.VerificationValue,
			)

			if innerErr != nil {
				return nil, fmt.Errorf("error updating receiver verification %s: %w", rv.VerificationField, innerErr)
			}
		}

		receiverUpdate := data.ReceiverUpdate{
			Email:      reqBody.Email,
			ExternalId: reqBody.ExternalID,
		}
		if receiverUpdate.Email != "" || receiverUpdate.ExternalId != "" {
			if innerErr = h.Models.Receiver.Update(ctx, dbTx, receiverID, receiverUpdate); innerErr != nil {
				return nil, fmt.Errorf("error updating receiver with ID %s: %w", receiverID, innerErr)
			}
		}

		receiver, innerErr := h.Models.Receiver.Get(ctx, h.Models.DBConnectionPool, receiverID)
		if innerErr != nil {
			return nil, fmt.Errorf("error querying receiver with ID %s: %w", receiverID, innerErr)
		}

		return receiver, nil
	})
	if err != nil {
		httperror.InternalError(ctx, "", err, nil).Render(rw)
	}

	httpjson.RenderStatus(rw, http.StatusOK, receiver, httpjson.JSON)
}
