package httphandler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/stellar/go/support/render/httpjson"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/dto"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpresponse"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
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

		receiverWallets, innerErr := rh.Models.ReceiverWallet.GetWithReceiverIDs(ctx, dbTx, data.ReceiverIDs{receiver.ID})
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

		receivers, err := rh.Models.Receiver.GetAll(ctx, dbTx, queryParams, data.QueryTypeSelectPaginated)
		if err != nil {
			return nil, fmt.Errorf("error retrieving receivers: %w", err)
		}

		receiverIDs := rh.Models.Receiver.ParseReceiverIDs(receivers)
		receiversWallets, err := rh.Models.ReceiverWallet.GetWithReceiverIDs(ctx, dbTx, receiverIDs)
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
	httpjson.Render(w, data.GetAllVerificationTypes(), httpjson.JSON)
}

func (rh ReceiverHandler) CreateReceiver(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var err error

	var req dto.CreateReceiverRequest
	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperror.BadRequest("invalid request body", err, nil).Render(rw)
		return
	}

	validator := validators.NewReceiverValidator()
	validator.ValidateCreateReceiverRequest(&req)
	if validator.HasErrors() {
		httperror.BadRequest("validation error", nil, validator.Errors).Render(rw)
		return
	}

	var response *GetReceiverResponse
	response, err = db.RunInTransactionWithResult(ctx, rh.DBConnectionPool, nil, func(dbTx db.DBTransaction) (*GetReceiverResponse, error) {
		var txErr error

		// Step 1: Prepare the receiver data for insertion into the database
		receiverInsert := data.ReceiverInsert{
			Email:       &req.Email,
			PhoneNumber: &req.PhoneNumber,
			ExternalId:  &req.ExternalID,
		}

		if req.Email == "" {
			receiverInsert.Email = nil
		}

		if req.PhoneNumber == "" {
			receiverInsert.PhoneNumber = nil
		}

		// Step 2: Insert the receiver record
		var receiver *data.Receiver
		if receiver, txErr = rh.Models.Receiver.Insert(ctx, dbTx, receiverInsert); txErr != nil {
			return nil, fmt.Errorf("creating receiver: %w", txErr)
		}

		// Step 3: Process verification requirements
		for _, v := range req.Verifications {
			if _, insErr := rh.Models.ReceiverVerification.Insert(ctx, dbTx, data.ReceiverVerificationInsert{
				ReceiverID:        receiver.ID,
				VerificationField: v.Type,
				VerificationValue: v.Value,
			}); insErr != nil {
				return nil, fmt.Errorf("creating verification: %w", insErr)
			}
		}

		// Step 4: Handle wallet assignments
		wallets := []data.ReceiverWallet{}

		if len(req.Wallets) > 0 {
			// Find the user-managed wallet
			var userManagedWallets []data.Wallet
			if userManagedWallets, txErr = rh.Models.Wallets.FindWallets(ctx, data.Filter{
				Key:   data.FilterUserManaged,
				Value: true,
			}); txErr != nil {
				return nil, fmt.Errorf("finding user managed wallet: %w", txErr)
			}

			if len(userManagedWallets) == 0 {
				return nil, fmt.Errorf("no user managed wallet found")
			}

			userManagedWallet := userManagedWallets[0]

			// Create receiver wallet associations
			for _, w := range req.Wallets {
				walletInsert := data.ReceiverWalletInsert{
					ReceiverID: receiver.ID,
					WalletID:   userManagedWallet.ID,
				}

				var receiverWalletID string
				if receiverWalletID, txErr = rh.Models.ReceiverWallet.Insert(ctx, dbTx, walletInsert); txErr != nil {
					return nil, fmt.Errorf("creating receiver wallet: %w", txErr)
				}

				// Update wallet with Stellar address and memo details
				walletUpdate := data.ReceiverWalletUpdate{
					Status:         data.RegisteredReceiversWalletStatus,
					StellarAddress: w.Address,
				}

				// Only set memo and memo type if memo is provided
				if w.Memo != "" {
					_, memoType, parseErr := schema.ParseMemo(w.Memo)
					if parseErr != nil {
						return nil, fmt.Errorf("parsing memo value: %w", parseErr)
					}
					walletUpdate.StellarMemo = &w.Memo
					walletUpdate.StellarMemoType = &memoType
				}

				if updErr := rh.Models.ReceiverWallet.Update(ctx, receiverWalletID, walletUpdate, dbTx); updErr != nil {
					return nil, fmt.Errorf("updating receiver wallet: %w", updErr)
				}

				var receiverWallet *data.ReceiverWallet
				if receiverWallet, txErr = rh.Models.ReceiverWallet.GetByID(ctx, dbTx, receiverWalletID); txErr != nil {
					return nil, fmt.Errorf("getting created receiver wallet: %w", txErr)
				}

				wallets = append(wallets, *receiverWallet)
			}
		}

		// Step 5: Retrieve verification records for response
		var receiverVerifications []data.ReceiverVerification
		if receiverVerifications, txErr = rh.Models.ReceiverVerification.GetAllByReceiverId(ctx, dbTx, receiver.ID); txErr != nil {
			return nil, fmt.Errorf("getting receiver verifications: %w", txErr)
		}

		return &GetReceiverResponse{
			Receiver:      *receiver,
			Wallets:       wallets,
			Verifications: receiverVerifications,
		}, nil
	})
	if err != nil {
		if httpErr := parseConflictErrorIfNeeded(err); httpErr != nil {
			httpErr.Render(rw)
			return
		}

		httperror.InternalError(ctx, "Error creating receiver", err, nil).Render(rw)
		return
	}

	httpjson.RenderStatus(rw, http.StatusCreated, response, httpjson.JSON)
}
