package httphandler

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/stellar/go/support/render/httpjson"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpresponse"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
)

type ReceiverHandler struct {
	Models           *data.Models
	DBConnectionPool db.DBConnectionPool
}

type GetReceiverResponse struct {
	data.Receiver
	Wallets       []data.ReceiverWallet       `json:"wallets"`
	Verifications []data.ReceiverVerification `json:"verifications,omitempty"`
}

func (rh ReceiverHandler) buildReceiversResponse(receivers []data.Receiver, receiversWallets []data.ReceiverWallet) []GetReceiverResponse {
	var responses []GetReceiverResponse

	for _, receiver := range receivers {
		wallets := make([]data.ReceiverWallet, 0)
		for _, wallet := range receiversWallets {
			if wallet.Receiver.ID == receiver.ID {
				wallets = append(wallets, wallet)
			}
		}
		responses = append(responses, GetReceiverResponse{
			Receiver: receiver,
			Wallets:  wallets,
		})
	}

	return responses
}

func (rh ReceiverHandler) GetReceiver(w http.ResponseWriter, r *http.Request) {
	receiverID := chi.URLParam(r, "id")
	ctx := r.Context()

	response, err := db.RunInTransactionWithResult(ctx, rh.DBConnectionPool, nil, func(dbTx db.DBTransaction) (response *GetReceiverResponse, innerErr error) {
		receiver, innerErr := rh.Models.Receiver.Get(ctx, dbTx, receiverID)
		if innerErr != nil {
			return nil, fmt.Errorf("getting receiver by ID: %w", innerErr)
		}

		receiverWallets, innerErr := rh.Models.ReceiverWallet.GetWithReceiverIds(ctx, dbTx, data.ReceiverIDs{receiver.ID})
		if innerErr != nil {
			return nil, fmt.Errorf("getting receiver wallets with receiver IDs: %w", innerErr)
		}

		receiverVerifications, innerErr := rh.Models.ReceiverVerification.GetAllByReceiverId(ctx, dbTx, receiver.ID)
		if innerErr != nil {
			return nil, fmt.Errorf("getting receiver verifications for receiver ID: %w", innerErr)
		}

		return &GetReceiverResponse{
			Receiver:      *receiver,
			Wallets:       receiverWallets,
			Verifications: receiverVerifications,
		}, nil
	})
	if err != nil {
		if errors.Is(err, data.ErrRecordNotFound) {
			errorResponse := fmt.Sprintf("could not retrieve receiver with ID: %s", receiverID)
			httperror.NotFound(errorResponse, err, nil).Render(w)
		} else {
			msg := fmt.Sprintf("Cannot retrieve receiver with ID %s", receiverID)
			httperror.InternalError(ctx, msg, err, nil).Render(w)
		}
		return
	}

	httpjson.RenderStatus(w, http.StatusOK, response, httpjson.JSON)
}

func (rh ReceiverHandler) GetReceivers(w http.ResponseWriter, r *http.Request) {
	validator := validators.NewReceiverQueryValidator()

	queryParams := validator.ParseParametersFromRequest(r)
	queryParams.Filters = validator.ValidateAndGetReceiverFilters(queryParams.Filters)
	if validator.HasErrors() {
		httperror.BadRequest("request invalid", nil, validator.Errors).Render(w)
		return
	}

	ctx := r.Context()

	httpResponse, err := db.RunInTransactionWithResult(ctx, rh.DBConnectionPool, nil, func(dbTx db.DBTransaction) (*httpresponse.PaginatedResponse, error) {
		totalReceivers, err := rh.Models.Receiver.Count(ctx, dbTx, queryParams)
		if err != nil {
			return nil, fmt.Errorf("error retrieving receivers count: %w", err)
		}

		if totalReceivers == 0 {
			httpResponse := httpresponse.NewEmptyPaginatedResponse()
			return &httpResponse, nil
		}

		receivers, err := rh.Models.Receiver.GetAll(ctx, dbTx, queryParams)
		if err != nil {
			return nil, fmt.Errorf("error retrieving receivers: %w", err)
		}

		receiverIDs := rh.Models.Receiver.ParseReceiverIDs(receivers)
		receiversWallets, err := rh.Models.ReceiverWallet.GetWithReceiverIds(ctx, dbTx, receiverIDs)
		if err != nil {
			return nil, fmt.Errorf("error retrieving receiver wallets: %w", err)
		}

		receiversResponse := rh.buildReceiversResponse(receivers, receiversWallets)
		httpResponse, err := httpresponse.NewPaginatedResponse(r, receiversResponse, queryParams.Page, queryParams.PageLimit, totalReceivers)
		if err != nil {
			return nil, fmt.Errorf("error creating paginated response for receivers: %w", err)
		}

		return &httpResponse, nil
	})
	if err != nil {
		httperror.InternalError(ctx, "Cannot retrieve receivers", err, nil).Render(w)
		return
	}

	httpjson.RenderStatus(w, http.StatusOK, httpResponse, httpjson.JSON)
}

// GetReceiverVerification returns a list of verification types
func (rh ReceiverHandler) GetReceiverVerificationTypes(w http.ResponseWriter, r *http.Request) {
	httpjson.Render(w, data.GetAllVerificationFields(), httpjson.JSON)
}
