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
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events/schemas"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpresponse"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/middleware"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

type PaymentsHandler struct {
	Models           *data.Models
	DBConnectionPool db.DBConnectionPool
	AuthManager      auth.AuthManager
	EventProducer    events.Producer
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

func (p PaymentsHandler) GetPayment(w http.ResponseWriter, r *http.Request) {
	payment_id := chi.URLParam(r, "id")

	payment, err := p.Models.Payment.Get(r.Context(), payment_id, p.DBConnectionPool)
	if err != nil {
		if errors.Is(data.ErrRecordNotFound, err) {
			errorResponse := fmt.Sprintf("Cannot retrieve payment with ID: %s", payment_id)
			httperror.NotFound(errorResponse, err, nil).Render(w)
			return
		} else {
			ctx := r.Context()
			msg := fmt.Sprintf("Cannot retrieve payment with id %s", payment_id)
			httperror.InternalError(ctx, msg, err, nil).Render(w)
			return
		}
	}

	httpjson.RenderStatus(w, http.StatusOK, payment, httpjson.JSON)
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

			// Producing event to send ready payments to TSS
			var payments []*data.Payment
			payments, err = p.Models.Payment.GetReadyByID(ctx, dbTx, reqBody.PaymentIDs...)
			if err != nil {
				return nil, fmt.Errorf("getting ready payments by IDs: %w", err)
			}

			if len(payments) > 0 {
				postCommitFn = func() error {
					return p.producePaymentsReadyEvents(ctx, payments)
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

func (p PaymentsHandler) producePaymentsReadyEvents(ctx context.Context, payments []*data.Payment) error {
	if len(payments) == 0 {
		log.Ctx(ctx).Info("No payments to produce ready to pay event")
		return nil
	}
	msg, msgErr := events.NewMessage(ctx, events.PaymentReadyToPayTopic, "", events.PaymentReadyToPayRetryFailedPayment, nil)
	if msgErr != nil {
		return fmt.Errorf("creating a new message: %w", msgErr)
	}

	paymentsReadyToPay := schemas.EventPaymentsReadyToPayData{TenantID: msg.TenantID}
	for _, payment := range payments {
		paymentsReadyToPay.Payments = append(paymentsReadyToPay.Payments, schemas.PaymentReadyToPay{ID: payment.ID})
	}
	msg.Data = paymentsReadyToPay
	msg.Key = paymentsReadyToPay.TenantID

	if p.EventProducer != nil {
		err := p.EventProducer.WriteMessages(ctx, *msg)
		if err != nil {
			return fmt.Errorf("writing message %s on event producer: %w", msg, err)
		}
	} else {
		log.Ctx(ctx).Errorf("event producer is nil, could not publish message %+v", msg)
	}
	return nil
}

func (p PaymentsHandler) getPaymentsWithCount(ctx context.Context, queryParams *data.QueryParams) (*utils.ResultWithTotal, error) {
	return db.RunInTransactionWithResult(ctx, p.DBConnectionPool, nil, func(dbTx db.DBTransaction) (response *utils.ResultWithTotal, innerErr error) {
		totalPayments, innerErr := p.Models.Payment.Count(ctx, queryParams, dbTx)
		if innerErr != nil {
			return nil, fmt.Errorf("error counting payments: %w", innerErr)
		}

		var payments []data.Payment
		if totalPayments != 0 {
			payments, innerErr = p.Models.Payment.GetAll(ctx, queryParams, dbTx)
			if innerErr != nil {
				return nil, fmt.Errorf("error querying payments: %w", innerErr)
			}
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
