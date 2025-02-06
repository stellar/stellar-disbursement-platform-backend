package httphandler

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gocarina/gocsv"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
)

type ExportHandler struct {
	Models *data.Models
}

func (e ExportHandler) ExportDisbursements(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	validator := validators.NewDisbursementQueryValidator()
	queryParams := validator.ParseParametersFromRequest(r)

	if validator.HasErrors() {
		httperror.BadRequest("Request invalid", nil, validator.Errors).Render(rw)
		return
	}

	queryParams.Filters = validator.ValidateAndGetDisbursementFilters(queryParams.Filters)
	if validator.HasErrors() {
		httperror.BadRequest("Request invalid", nil, validator.Errors).Render(rw)
		return
	}

	disbursements, err := e.Models.Disbursements.GetAll(ctx, e.Models.DBConnectionPool, queryParams, data.QueryTypeSelectAll)
	if err != nil {
		httperror.InternalError(ctx, "Failed to get disbursements", err, nil).Render(rw)
		return
	}

	fileName := fmt.Sprintf("disbursements_%s.csv", time.Now().Format("2006-01-02-15-04-05"))
	rw.Header().Set("Content-Type", "text/csv")
	rw.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", fileName))

	if err := gocsv.Marshal(disbursements, rw); err != nil {
		httperror.InternalError(ctx, "Failed to write CSV", err, nil).Render(rw)
		return
	}
}

type PaymentCSV struct {
	ID                      string
	Amount                  string
	StellarTransactionID    string
	Status                  data.PaymentStatus
	DisbursementID          string `csv:"Disbursement.ID"`
	Asset                   data.Asset
	Wallet                  data.Wallet
	ReceiverID              string                     `csv:"Receiver.ID"`
	ReceiverPhoneNumber     string                     `csv:"Receiver.PhoneNumber"`
	ReceiverEmail           string                     `csv:"Receiver.Email"`
	ReceiverWalletAddress   string                     `csv:"ReceiverWallet.Address"`
	ReceiverWalletStatus    data.ReceiversWalletStatus `csv:"ReceiverWallet.Status"`
	CreatedAt               time.Time
	UpdatedAt               time.Time
	ExternalPaymentID       string
	CircleTransferRequestID *string
}

func (e ExportHandler) ExportPayments(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	validator := validators.NewPaymentQueryValidator()
	queryParams := validator.ParseParametersFromRequest(r)

	if validator.HasErrors() {
		httperror.BadRequest("Request invalid", nil, validator.Errors).Render(rw)
		return
	}

	queryParams.Filters = validator.ValidateAndGetPaymentFilters(queryParams.Filters)
	if validator.HasErrors() {
		httperror.BadRequest("Request invalid", nil, validator.Errors).Render(rw)
		return
	}

	payments, err := e.Models.Payment.GetAll(ctx, queryParams, e.Models.DBConnectionPool, data.QueryTypeSelectAll)
	if err != nil {
		httperror.InternalError(ctx, "Failed to get payments", err, nil).Render(rw)
		return
	}

	receiversMap, err := e.getPaymentReceiversMap(ctx, payments)
	if err != nil {
		httperror.InternalError(ctx, "Failed to get receivers", err, nil).Render(rw)
		return
	}

	paymentCSVs := e.convertPaymentsToCSV(payments, receiversMap)

	fileName := fmt.Sprintf("payments_%s.csv", time.Now().Format("2006-01-02-15-04-05"))
	rw.Header().Set("Content-Type", "text/csv")
	rw.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", fileName))

	if err := gocsv.Marshal(paymentCSVs, rw); err != nil {
		httperror.InternalError(ctx, "Failed to write CSV", err, nil).Render(rw)
		return
	}
}

// getPaymentReceiversMap returns a map of receivers by receiverID for the given payments.
func (e ExportHandler) getPaymentReceiversMap(ctx context.Context, payments []data.Payment) (map[string]data.Receiver, error) {
	receiverIDs := make([]string, 0, len(payments))

	if len(payments) == 0 {
		return map[string]data.Receiver{}, nil
	}

	for _, payment := range payments {
		receiverIDs = append(receiverIDs, payment.ReceiverWallet.Receiver.ID)
	}

	receivers, err := e.Models.Receiver.GetAll(ctx, e.Models.DBConnectionPool, &data.QueryParams{
		Filters: map[data.FilterKey]interface{}{
			data.FilterKeyIDs: receiverIDs,
		},
	}, data.QueryTypeSelectAll)
	if err != nil {
		return nil, fmt.Errorf("failed to get receivers: %w", err)
	}

	receiversMap := make(map[string]data.Receiver, len(receivers))
	for _, receiver := range receivers {
		receiversMap[receiver.ID] = receiver
	}
	return receiversMap, nil
}

// convertPaymentsToCSV converts the given payments and receivers to a slice of PaymentCSV.
func (e ExportHandler) convertPaymentsToCSV(payments []data.Payment, receiversMap map[string]data.Receiver) []*PaymentCSV {
	paymentCSVs := make([]*PaymentCSV, 0, len(payments))
	for _, payment := range payments {
		paymentCSV := &PaymentCSV{
			ID:                      payment.ID,
			Amount:                  payment.Amount,
			StellarTransactionID:    payment.StellarTransactionID,
			Status:                  payment.Status,
			DisbursementID:          payment.Disbursement.ID,
			Asset:                   payment.Asset,
			Wallet:                  payment.ReceiverWallet.Wallet,
			ReceiverID:              payment.ReceiverWallet.Receiver.ID,
			ReceiverPhoneNumber:     receiversMap[payment.ReceiverWallet.Receiver.ID].PhoneNumber,
			ReceiverEmail:           receiversMap[payment.ReceiverWallet.Receiver.ID].Email,
			ReceiverWalletAddress:   payment.ReceiverWallet.StellarAddress,
			ReceiverWalletStatus:    payment.ReceiverWallet.Status,
			CreatedAt:               payment.CreatedAt,
			UpdatedAt:               payment.UpdatedAt,
			ExternalPaymentID:       payment.ExternalPaymentID,
			CircleTransferRequestID: payment.CircleTransferRequestID,
		}
		paymentCSVs = append(paymentCSVs, paymentCSV)
	}
	return paymentCSVs
}

func (e ExportHandler) ExportReceivers(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	validator := validators.NewReceiverQueryValidator()
	queryParams := validator.ParseParametersFromRequest(r)
	if validator.HasErrors() {
		httperror.BadRequest("Request invalid", nil, validator.Errors).Render(rw)
		return
	}

	queryParams.Filters = validator.ValidateAndGetReceiverFilters(queryParams.Filters)
	if validator.HasErrors() {
		httperror.BadRequest("Request invalid", nil, validator.Errors).Render(rw)
		return
	}

	receivers, err := e.Models.Receiver.GetAll(ctx, e.Models.DBConnectionPool, queryParams, data.QueryTypeSelectAll)
	if err != nil {
		httperror.InternalError(ctx, "Failed to get receivers", err, nil).Render(rw)
		return
	}

	fileName := fmt.Sprintf("receivers_%s.csv", time.Now().Format("2006-01-02-15-04-05"))
	rw.Header().Set("Content-Type", "text/csv")
	rw.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", fileName))

	if err := gocsv.Marshal(receivers, rw); err != nil {
		httperror.InternalError(ctx, "Failed to write CSV", err, nil).Render(rw)
		return
	}
}
