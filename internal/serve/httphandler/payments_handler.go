package httphandler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/stellar/go/support/http/httpdecode"
	"github.com/stellar/go/support/log"
	"github.com/stellar/go/support/render/httpjson"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events/schemas"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpresponse"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/middleware"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

type PaymentsHandler struct {
	Models                      *data.Models
	DBConnectionPool            db.DBConnectionPool
	AuthManager                 auth.AuthManager
	EventProducer               events.Producer
	CrashTrackerClient          crashtracker.CrashTrackerClient
	DistributionAccountResolver signing.DistributionAccountResolver
	DirectPaymentService        *services.DirectPaymentService
}

type RetryPaymentsRequest struct {
	PaymentIDs []string `json:"payment_ids"`
}

func (r *RetryPaymentsRequest) validate() *httperror.HTTPError {
	validator := validators.NewValidator()
	validator.Check(len(r.PaymentIDs) != 0, "payment_ids", "payment_ids should not be empty")
	if validator.HasErrors() {
		return httperror.BadRequest("", nil, validator.Errors)
	}

	return nil
}

func (p PaymentsHandler) decorateWithCircleTransactionInfo(ctx context.Context, payments ...data.Payment) ([]data.Payment, error) {
	if len(payments) == 0 {
		return payments, nil
	}

	distAccount, err := p.DistributionAccountResolver.DistributionAccountFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolving distribution account: %w", err)
	}

	if !distAccount.IsCircle() {
		return payments, nil
	}

	paymentIDs := make([]string, len(payments))
	for i, payment := range payments {
		paymentIDs[i] = payment.ID
	}

	transfersByPaymentID, err := p.Models.CircleTransferRequests.GetCurrentTransfersForPaymentIDs(ctx, p.DBConnectionPool, paymentIDs)
	if err != nil {
		return nil, fmt.Errorf("getting circle transfers for payment IDs: %w", err)
	}

	for i, payment := range payments {
		if transfer, ok := transfersByPaymentID[payment.ID]; ok {
			payments[i].CircleTransferRequestID = transfer.CircleTransferID
		}
	}

	return payments, nil
}

func (p PaymentsHandler) GetPayment(w http.ResponseWriter, r *http.Request) {
	paymentID := chi.URLParam(r, "id")
	ctx := r.Context()

	payment, err := p.Models.Payment.Get(ctx, paymentID, p.DBConnectionPool)
	if err != nil {
		if errors.Is(err, data.ErrRecordNotFound) {
			errorResponse := fmt.Sprintf("Cannot retrieve payment with ID: %s", paymentID)
			httperror.NotFound(errorResponse, err, nil).Render(w)
			return
		} else {
			msg := fmt.Sprintf("Cannot retrieve payment with id %s", paymentID)
			httperror.InternalError(ctx, msg, err, nil).Render(w)
			return
		}
	}

	payments, err := p.decorateWithCircleTransactionInfo(ctx, *payment)
	if err != nil {
		httperror.InternalError(ctx, "Cannot retrieve payment with circle info", err, nil).Render(w)
		return
	}

	httpjson.RenderStatus(w, http.StatusOK, payments[0], httpjson.JSON)
}

func (p PaymentsHandler) GetPayments(w http.ResponseWriter, r *http.Request) {
	validator := validators.NewPaymentQueryValidator()
	queryParams := validator.ParseParametersFromRequest(r)
	var err error

	if validator.HasErrors() {
		httperror.BadRequest("request invalid", nil, validator.Errors).Render(w)
		return
	}

	queryParams.Filters = validator.ValidateAndGetPaymentFilters(queryParams.Filters)
	if validator.HasErrors() {
		httperror.BadRequest("request invalid", nil, validator.Errors).Render(w)
		return
	}

	ctx := r.Context()

	response, err := p.getPaymentsWithCount(ctx, queryParams)
	if err != nil {
		httperror.InternalError(ctx, "Cannot retrieve payments", err, nil).Render(w)
		return
	}
	if response.Total == 0 {
		httpjson.RenderStatus(w, http.StatusOK, httpresponse.NewEmptyPaginatedResponse(), httpjson.JSON)
	} else {
		response, errGet := httpresponse.NewPaginatedResponse(r, response.Result, queryParams.Page, queryParams.PageLimit, response.Total)
		if errGet != nil {
			httperror.InternalError(ctx, "Cannot create paginated payments response", errGet, nil).Render(w)
			return
		}
		httpjson.RenderStatus(w, http.StatusOK, response, httpjson.JSON)
	}
}

func (p PaymentsHandler) RetryPayments(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	token, ok := ctx.Value(middleware.TokenContextKey).(string)
	if !ok {
		httperror.Unauthorized("", nil, nil).Render(rw)
		return
	}

	user, err := p.AuthManager.GetUser(ctx, token)
	if err != nil {
		httperror.InternalError(ctx, "", err, nil).Render(rw)
		return
	}

	var reqBody RetryPaymentsRequest
	if err = httpdecode.DecodeJSON(req, &reqBody); err != nil {
		httperror.BadRequest("", err, nil).Render(rw)
		return
	}

	if err := reqBody.validate(); err != nil {
		err.Render(rw)
		return
	}

	opts := db.TransactionOptions{
		DBConnectionPool: p.DBConnectionPool,
		AtomicFunctionWithPostCommit: func(dbTx db.DBTransaction) (postCommitFn db.PostCommitFunction, err error) {
			err = p.Models.Payment.RetryFailedPayments(ctx, dbTx, user.Email, reqBody.PaymentIDs...)
			if err != nil {
				return nil, fmt.Errorf("retrying failed payments: %w", err)
			}

			var tnt *tenant.Tenant
			if tnt, err = tenant.GetTenantFromContext(ctx); err != nil {
				return nil, fmt.Errorf("getting tenant from context: %w", err)
			} else if tnt.DistributionAccountType.IsCircle() {
				_, err = p.Models.CircleRecipient.ResetRecipientsForRetryIfNeeded(ctx, dbTx, reqBody.PaymentIDs...)
				if err != nil {
					return nil, fmt.Errorf("resetting circle recipients for retry if needed: %w", err)
				}
			}

			// Producing event to send ready payments to TSS
			var payments []*data.Payment
			payments, err = p.Models.Payment.GetReadyByID(ctx, dbTx, reqBody.PaymentIDs...)
			if err != nil {
				return nil, fmt.Errorf("getting ready payments by IDs: %w", err)
			}

			if len(payments) > 0 {
				msg, err := p.buildPaymentsReadyEventMessage(ctx, payments)
				if err != nil {
					return nil, fmt.Errorf("building event message for payment retry: %w", err)
				}

				postCommitFn = func() error {
					postErr := events.ProduceEvents(ctx, p.EventProducer, msg)
					if postErr != nil {
						p.CrashTrackerClient.LogAndReportErrors(ctx, postErr, "writing retry payment message on the event producer")
					}

					return nil
				}
			}

			return postCommitFn, nil
		},
	}
	err = db.RunInTransactionWithPostCommit(ctx, &opts)
	if err != nil {
		if errors.Is(err, data.ErrMismatchNumRowsAffected) {
			httperror.BadRequest("Invalid payment ID(s) provided. All payment IDs must exist and be in the 'FAILED' state.", err, nil).Render(rw)
			return
		}

		httperror.InternalError(ctx, "", err, nil).Render(rw)
		return
	}

	httpjson.RenderStatus(rw, http.StatusOK, map[string]string{"message": "Payments retried successfully"}, httpjson.JSON)
}

func (p PaymentsHandler) buildPaymentsReadyEventMessage(ctx context.Context, payments []*data.Payment) (*events.Message, error) {
	if len(payments) == 0 {
		log.Ctx(ctx).Warnf("no payments to retry")
		return nil, nil
	}

	distAccount, err := p.DistributionAccountResolver.DistributionAccountFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolving distribution account: %w", err)
	}

	msg, err := events.NewPaymentReadyToPayMessage(ctx, distAccount.Type.Platform(), "", events.PaymentReadyToPayRetryFailedPayment)
	if err != nil {
		return nil, fmt.Errorf("creating a new message: %w", err)
	}

	paymentsReadyToPay := schemas.EventPaymentsReadyToPayData{TenantID: msg.TenantID}
	for _, payment := range payments {
		paymentsReadyToPay.Payments = append(paymentsReadyToPay.Payments, schemas.PaymentReadyToPay{ID: payment.ID})
	}
	msg.Data = paymentsReadyToPay
	msg.Key = paymentsReadyToPay.TenantID

	err = msg.Validate()
	if err != nil {
		return nil, fmt.Errorf("validating message: %w", err)
	}

	return msg, nil
}

func (p PaymentsHandler) getPaymentsWithCount(ctx context.Context, queryParams *data.QueryParams) (*utils.ResultWithTotal, error) {
	return db.RunInTransactionWithResult(ctx, p.DBConnectionPool, nil, func(dbTx db.DBTransaction) (response *utils.ResultWithTotal, innerErr error) {
		totalPayments, innerErr := p.Models.Payment.Count(ctx, queryParams, dbTx)
		if innerErr != nil {
			return nil, fmt.Errorf("error counting payments: %w", innerErr)
		}

		var payments []data.Payment
		if totalPayments != 0 {
			payments, innerErr = p.Models.Payment.GetAll(ctx, queryParams, dbTx, data.QueryTypeSelectPaginated)
			if innerErr != nil {
				return nil, fmt.Errorf("error querying payments: %w", innerErr)
			}
		}

		payments, err := p.decorateWithCircleTransactionInfo(ctx, payments...)
		if err != nil {
			return nil, fmt.Errorf("adding circle info to payments: %w", err)
		}

		return utils.NewResultWithTotal(totalPayments, payments), nil
	})
}

type PatchPaymentStatusRequest struct {
	Status string `json:"status"`
}

type UpdatePaymentStatusResponseBody struct {
	Message string `json:"message"`
}

func (p PaymentsHandler) PatchPaymentStatus(w http.ResponseWriter, r *http.Request) {
	var patchRequest PatchPaymentStatusRequest
	err := json.NewDecoder(r.Body).Decode(&patchRequest)
	if err != nil {
		httperror.BadRequest("invalid request body", err, nil).Render(w)
		return
	}

	// validate request
	toStatus, err := data.ToPaymentStatus(patchRequest.Status)
	if err != nil {
		httperror.BadRequest("invalid status", err, nil).Render(w)
		return
	}

	paymentManagementService := services.NewPaymentManagementService(p.Models, p.DBConnectionPool)
	response := UpdatePaymentStatusResponseBody{}

	ctx := r.Context()
	paymentID := chi.URLParam(r, "id")

	switch toStatus {
	case data.CanceledPaymentStatus:
		err = paymentManagementService.CancelPayment(ctx, paymentID)
		response.Message = "Payment canceled"
	default:
		err = services.ErrPaymentStatusCantBeChanged
	}

	if err != nil {
		switch {
		case errors.Is(err, services.ErrPaymentNotFound):
			httperror.NotFound(services.ErrPaymentNotFound.Error(), err, nil).Render(w)
		case errors.Is(err, services.ErrPaymentNotReadyToCancel):
			httperror.BadRequest(services.ErrPaymentNotReadyToCancel.Error(), err, nil).Render(w)
		case errors.Is(err, services.ErrPaymentStatusCantBeChanged):
			httperror.BadRequest(services.ErrPaymentStatusCantBeChanged.Error(), err, nil).Render(w)
		default:
			msg := fmt.Sprintf("Cannot update payment ID %s with status: %s", paymentID, toStatus)
			httperror.InternalError(ctx, msg, err, nil).Render(w)
		}
		return
	}

	httpjson.RenderStatus(w, http.StatusOK, response, httpjson.JSON)
}

func (p PaymentsHandler) PostDirectPayment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req validators.CreateDirectPaymentRequest
	if err := httpdecode.DecodeJSON(r, &req); err != nil {
		httperror.BadRequest("invalid request body", err, nil).Render(w)
		return
	}

	validator := validators.NewDirectPaymentValidator()
	validatedReq := validator.ValidateCreateDirectPaymentRequest(&req)
	if validator.HasErrors() {
		httperror.BadRequest("request invalid", nil, validator.Errors).Render(w)
		return
	}

	userID, ok := ctx.Value(middleware.UserIDContextKey).(string)
	if !ok {
		httperror.Unauthorized("", nil, nil).Render(w)
		return
	}
	user, err := p.AuthManager.GetUserByID(ctx, userID)
	if err != nil {
		httperror.InternalError(ctx, "Cannot get user", err, nil).Render(w)
		return
	}

	serviceReq := services.CreateDirectPaymentRequest{
		Amount:            validatedReq.Amount,
		Asset:             convertAssetReference(validatedReq.Asset),
		Receiver:          convertReceiverReference(validatedReq.Receiver),
		Wallet:            convertWalletReference(validatedReq.Wallet),
		ExternalPaymentID: validatedReq.ExternalPaymentID,
	}

	distAccount, err := p.DistributionAccountResolver.DistributionAccountFromContext(ctx)
	if err != nil {
		httperror.InternalError(ctx, "resolving distribution account", err, nil).Render(w)
		return
	}

	payment, err := p.DirectPaymentService.CreateDirectPayment(ctx, serviceReq, user, &distAccount)
	if err != nil {
		var (
			validationErr        services.ValidationError
			notFoundErr          services.NotFoundError
			unsupportedErr       services.UnsupportedError
			ambiguousErr         services.AmbiguousReferenceError
			insufficientFundsErr services.InsufficientBalanceForDirectPaymentError
			walletDisabledErr    services.WalletNotEnabledError
			assetNotSupportedErr services.AssetNotSupportedByWalletError
			recvErr              services.ReceiverWalletNotFoundError
			trustErr             services.TrustlineNotFoundError
			accErr               services.AccountNotFoundError
			circleAccErr         services.CircleAccountNotActivatedError
			circleAssetErr       services.CircleAssetNotSupportedError
		)

		switch {
		case errors.As(err, &validationErr):
			httperror.BadRequest(validationErr.Error(), err, nil).Render(w)
		case errors.As(err, &notFoundErr):
			httperror.NotFound(notFoundErr.Error(), err, nil).Render(w)
		case errors.As(err, &unsupportedErr):
			httperror.BadRequest(unsupportedErr.Error(), err, nil).Render(w)
		case errors.As(err, &ambiguousErr):
			httperror.BadRequest(ambiguousErr.Error(), err, nil).Render(w)
		case errors.As(err, &insufficientFundsErr):
			log.Ctx(ctx).Error(insufficientFundsErr)
			httperror.BadRequest(insufficientFundsErr.Error(), err, nil).Render(w)
		case errors.As(err, &walletDisabledErr):
			httperror.BadRequest(walletDisabledErr.Error(), err, nil).Render(w)
		case errors.As(err, &assetNotSupportedErr):
			httperror.BadRequest(assetNotSupportedErr.Error(), err, nil).Render(w)
		case errors.As(err, &recvErr):
			httperror.BadRequest(err.Error(), err, nil).Render(w)
		case errors.As(err, &trustErr):
			errorMsg := fmt.Sprintf("%s. Please add a trustline for this asset to your distribution account, or choose a different asset that already has a trustline.", err.Error())
			httperror.BadRequest(errorMsg, err, nil).Render(w)
		case errors.As(err, &circleAccErr):
			errorMsg := fmt.Sprintf("%s. Please complete the Circle account activation process...", err.Error())
			httperror.BadRequest(errorMsg, err, nil).Render(w)
		case errors.As(err, &circleAssetErr):
			errorMsg := fmt.Sprintf("%s. Please choose a different asset supported by Circle...", err.Error())
			httperror.BadRequest(errorMsg, err, nil).Render(w)
		case errors.As(err, &accErr):
			errorMsg := fmt.Sprintf("%s. Please ensure your distribution account exists and is funded on the Stellar network.", err.Error())
			httperror.BadRequest(errorMsg, err, nil).Render(w)
		default:
			httperror.InternalError(ctx, "creating payment", err, nil).Render(w)
		}

		return
	}

	httpjson.RenderStatus(w, http.StatusCreated, payment, httpjson.JSON)
}

func convertAssetReference(asset validators.DirectPaymentAsset) services.AssetReference {
	return services.AssetReference{
		ID:         asset.ID,
		Type:       asset.Type,
		Code:       asset.Code,
		Issuer:     asset.Issuer,
		ContractID: asset.ContractID,
	}
}

func convertReceiverReference(receiver validators.DirectPaymentReceiver) services.ReceiverReference {
	return services.ReceiverReference{
		ID:            receiver.ID,
		Email:         receiver.Email,
		PhoneNumber:   receiver.PhoneNumber,
		WalletAddress: receiver.WalletAddress,
	}
}

func convertWalletReference(wallet validators.DirectPaymentWallet) services.WalletReference {
	return services.WalletReference{
		ID:      wallet.ID,
		Address: wallet.Address,
	}
}
