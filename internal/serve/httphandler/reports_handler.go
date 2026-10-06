package httphandler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/stellar/go-stellar-sdk/clients/horizonclient"
	"github.com/stellar/go-stellar-sdk/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/pdf/statement"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/pdf/transaction"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

const (
	errStatementOnlySupportedForStellar = "Statement is only supported for Stellar distribution accounts"
	errStatementNotForSharedAccount     = "Statements are not available for tenants on the shared host distribution account"
	errStatementAccountNotProvisioned   = "The distribution account has not been provisioned yet"
	internalNotesMaxLength              = 500
	disbursementTimestampFormat         = "Jan 2, 2006 · 15:04:05 UTC"
)

// ReportsHandler handles GET /reports/statement (statement PDF) and GET /reports/payment/{id} (payment notice PDF).
type ReportsHandler struct {
	ReportsService   services.ReportsServiceInterface
	Models           *data.Models
	DBConnectionPool db.DBConnectionPool
	HorizonClient    horizonclient.ClientInterface
	AuthManager      auth.AuthManager
}

// GetStatementExport returns the statement PDF for one distribution account: the one named by
// X-Wallet-Id, or the only account the caller can see. Reads follow the membership taxonomy, so an
// account outside the caller's scope is a 404.
func (h ReportsHandler) GetStatementExport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	v := validators.NewStatementQueryValidator()
	params := v.ValidateAndGetStatementParams(r)
	if v.HasErrors() {
		httperror.BadRequest("invalid query parameters", nil, v.Validator.Errors).Render(w)
		return
	}

	wallet, httpErr := h.resolveStatementWallet(ctx, r)
	if httpErr != nil {
		httpErr.Render(w)
		return
	}
	account, httpErr := statementAccount(wallet)
	if httpErr != nil {
		httpErr.Render(w)
		return
	}

	result, err := h.ReportsService.GetStatement(ctx, &account, wallet.ID, params.AssetCode, params.FromDate, params.ToDate)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrStatementAccountNotStellar):
			httperror.BadRequest(errStatementOnlySupportedForStellar, err, nil).Render(w)
			return
		case errors.Is(err, services.ErrStatementAssetNotFound):
			httperror.NotFound("asset not found for account", err, nil).Render(w)
			return
		default:
			httperror.InternalError(ctx, "Cannot retrieve statement", err, nil).Render(w)
			return
		}
	}
	result.Summary.AccountName = wallet.Name
	if wallet.Status == data.ArchivedDistributionWalletStatus {
		result.Summary.AccountName += " (archived)"
	}

	var orgName string
	var orgLogo []byte
	if org, err := h.Models.Organizations.Get(ctx); err == nil {
		orgName = org.Name
		orgLogo = org.Logo
	}

	pdfBytes, err := statement.BuildPDF(result, params.FromDate, params.ToDate, orgName, orgLogo, params.OperatedByBaseURL)
	if err != nil {
		httperror.InternalError(ctx, "Cannot generate statement PDF", err, nil).Render(w)
		return
	}

	filename := fmt.Sprintf("statement_%s_%s-%s.pdf",
		filenameToken(wallet.Name),
		params.FromDate.Format("20060102"),
		params.ToDate.Format("20060102"))
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Length", strconv.Itoa(len(pdfBytes)))
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	if _, err := w.Write(pdfBytes); err != nil {
		log.Ctx(ctx).Errorf("writing statement PDF response: %v", err)
		return
	}
}

// resolveStatementWallet picks the account a statement is for. X-Wallet-Id selects within the
// caller's read scope (404 outside it, like every other read); without the header the only visible
// account is used, so single-account tenants and single-account API keys need no header.
func (h ReportsHandler) resolveStatementWallet(ctx context.Context, r *http.Request) (*data.DistributionWallet, *httperror.HTTPError) {
	scope, scopeErr := resolveWalletReadScope(ctx, h.AuthManager, h.Models)
	if scopeErr != nil {
		return nil, scopeErr
	}

	walletID := r.Header.Get(XWalletIDHeader)
	if walletID == "" {
		visible := scope
		if visible == nil { // tenant-wide caller: every account, archived ones included
			wallets, err := h.Models.DistributionWallets.GetAll(ctx, h.DBConnectionPool, true)
			if err != nil {
				return nil, httperror.InternalError(ctx, "Cannot list distribution wallets", err, nil)
			}
			for _, w := range wallets {
				visible = append(visible, w.ID)
			}
		}
		if len(visible) != 1 {
			return nil, httperror.BadRequest("select a distribution account with the X-Wallet-Id header", nil, nil)
		}
		walletID = visible[0]
	} else if !walletInReadScope(scope, walletID) {
		return nil, httperror.NotFound("distribution wallet not found", nil, nil)
	}

	wallet, err := h.Models.DistributionWallets.Get(ctx, h.DBConnectionPool, walletID)
	if err != nil {
		if errors.Is(err, data.ErrRecordNotFound) {
			return nil, httperror.NotFound("distribution wallet not found", nil, nil)
		}
		return nil, httperror.InternalError(ctx, "Cannot load distribution wallet", err, nil)
	}
	return wallet, nil
}

// filenameToken reduces an account name to [a-z0-9-] so it can sit in a Content-Disposition filename.
func filenameToken(name string) string {
	token := strings.ToLower(strings.TrimSpace(name))
	token = nonFilenameChars.ReplaceAllString(token, "-")
	return strings.Trim(token, "-")
}

var nonFilenameChars = regexp.MustCompile(`[^a-z0-9]+`)

// statementAccount is the on-chain identity a statement is read for. Only accounts the SDP holds
// on the Stellar network qualify: a Circle account has no ledger history, and the shared host
// account's history is not this tenant's.
func statementAccount(wallet *data.DistributionWallet) (schema.TransactionAccount, *httperror.HTTPError) {
	switch {
	case wallet.AccountType == schema.DistributionAccountStellarEnv:
		return schema.TransactionAccount{}, httperror.BadRequest(errStatementNotForSharedAccount, nil, nil)
	case wallet.AccountType != schema.DistributionAccountStellarDBVault:
		return schema.TransactionAccount{}, httperror.BadRequest(errStatementOnlySupportedForStellar, nil, nil)
	case wallet.Address == nil || *wallet.Address == "":
		return schema.TransactionAccount{}, httperror.BadRequest(errStatementAccountNotProvisioned, nil, nil)
	}
	return schema.TransactionAccount{Address: *wallet.Address, Type: wallet.AccountType, Status: wallet.AccountStatus}, nil
}

// GetPaymentExport returns the Transaction Notice PDF for a single payment.
func (h ReportsHandler) GetPaymentExport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	paymentID := chi.URLParam(r, "id")

	payment, err := h.Models.Payment.Get(ctx, paymentID, h.DBConnectionPool)
	if err != nil {
		if errors.Is(err, data.ErrRecordNotFound) {
			errorResponse := fmt.Sprintf("Cannot retrieve payment with ID: %s", paymentID)
			httperror.NotFound(errorResponse, err, nil).Render(w)
			return
		}
		msg := fmt.Sprintf("Cannot retrieve payment with id %s", paymentID)
		httperror.InternalError(ctx, msg, err, nil).Render(w)
		return
	}
	if httpErr := ensurePaymentInReadScope(ctx, h.AuthManager, h.Models, payment); httpErr != nil {
		httpErr.Render(w)
		return
	}
	payments := []data.Payment{*payment}
	if err = h.Models.CircleTransferRequests.PopulateCircleTransactionInfo(ctx, h.DBConnectionPool, payments); err != nil {
		httperror.InternalError(ctx, "Cannot retrieve payment circle info", err, nil).Render(w)
		return
	}
	payment = &payments[0]
	if payment.ReceiverWallet != nil {
		receiver, rErr := h.Models.Receiver.Get(ctx, h.DBConnectionPool, payment.ReceiverWallet.Receiver.ID)
		if rErr != nil {
			httperror.InternalError(ctx, "Cannot retrieve payment receiver", rErr, nil).Render(w)
			return
		}
		payment.ReceiverWallet.Receiver.ExternalID = receiver.ExternalID
	}

	internalNotes := strings.TrimSpace(r.URL.Query().Get("internal_notes"))
	if utf8.RuneCountInString(internalNotes) > internalNotesMaxLength {
		internalNotes = string([]rune(internalNotes)[:internalNotesMaxLength])
	}
	var internalNotesPtr *string
	if internalNotes != "" {
		internalNotesPtr = &internalNotes
	}

	operatedByBaseURL := strings.TrimSpace(r.URL.Query().Get("base_url"))

	var orgName string
	var orgLogo []byte
	if h.Models != nil {
		if org, err := h.Models.Organizations.Get(ctx); err == nil {
			orgName = org.Name
			orgLogo = org.Logo
		}
	}

	// The sender is the account the payment was actually sent from: recorded on success and kept
	// through a later rotation. Until then, the source account's current address (none for Circle).
	sourceWallet, err := h.Models.DistributionWallets.Get(ctx, h.DBConnectionPool, payment.SourceWalletID)
	if err != nil {
		httperror.InternalError(ctx, "Cannot retrieve payment source account", err, nil).Render(w)
		return
	}
	senderWalletAddress := payment.SenderAddress
	if senderWalletAddress == "" && sourceWallet.Address != nil {
		senderWalletAddress = *sourceWallet.Address
	}

	var feeCharged string
	var memoText string
	if payment.StellarTransactionID != "" {
		if h.HorizonClient == nil {
			log.Ctx(ctx).Warnf("Horizon client not configured; cannot fetch fee for transaction %s", payment.StellarTransactionID)
		} else {
			hTx, hErr := h.HorizonClient.TransactionDetail(payment.StellarTransactionID)
			if hErr != nil {
				log.Ctx(ctx).Warnf("fetching transaction fee from Horizon for %s: %v", payment.StellarTransactionID, hErr)
			} else {
				// FeeCharged is in stroops (1 XLM = 10^7 stroops)
				whole := hTx.FeeCharged / 1e7
				frac := hTx.FeeCharged % 1e7
				if frac < 0 {
					frac = -frac
				}
				feeCharged = fmt.Sprintf("%d.%07d XLM", whole, frac)
				memoText = hTx.Memo
			}
		}
	}

	enrichment := &transaction.Enrichment{
		SenderName:           orgName,
		SenderAccountName:    sourceWallet.Name,
		SenderWalletAddress:  senderWalletAddress,
		FeeCharged:           feeCharged,
		MemoText:             memoText,
		StellarExpertBaseURL: transaction.GetStellarExpertBaseURL(),
	}
	if payment.Type == data.PaymentTypeDisbursement && payment.Disbursement != nil && len(payment.Disbursement.StatusHistory) > 0 {
		populateDisbursementCreatedApprovedBy(ctx, h.AuthManager, payment.Disbursement.StatusHistory, enrichment)
	}

	pdfBytes, err := transaction.BuildPDF(payment, orgName, orgLogo, enrichment, internalNotesPtr, operatedByBaseURL)
	if err != nil {
		httperror.InternalError(ctx, "Cannot generate transaction notice PDF", err, nil).Render(w)
		return
	}

	filename := fmt.Sprintf("transaction_notice_%s.pdf", paymentID)
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Length", strconv.Itoa(len(pdfBytes)))
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	if _, err := w.Write(pdfBytes); err != nil {
		log.Ctx(ctx).Errorf("writing transaction notice PDF response: %v", err)
		return
	}
}

// populateDisbursementCreatedApprovedBy fills enrichment with Created by / Approved by from the disbursement's status_history.
func populateDisbursementCreatedApprovedBy(ctx context.Context, authManager auth.AuthManager, history data.DisbursementStatusHistory, enrichment *transaction.Enrichment) {
	var draftEntry, startedEntry *data.DisbursementStatusHistoryEntry
	for i := range history {
		e := &history[i]
		if e.Status == data.DraftDisbursementStatus {
			draftEntry = e
		}
		if e.Status == data.StartedDisbursementStatus {
			startedEntry = e
		}
	}
	userIDs := make(map[string]struct{})
	if draftEntry != nil && draftEntry.UserID != "" {
		userIDs[draftEntry.UserID] = struct{}{}
	}
	if startedEntry != nil && startedEntry.UserID != "" {
		userIDs[startedEntry.UserID] = struct{}{}
	}
	if len(userIDs) == 0 {
		if draftEntry != nil {
			enrichment.DisbursementCreatedByTimestamp = draftEntry.Timestamp.UTC().Format(disbursementTimestampFormat)
		}
		if startedEntry != nil {
			enrichment.DisbursementApprovedByTimestamp = startedEntry.Timestamp.UTC().Format(disbursementTimestampFormat)
		}
		return
	}
	ids := make([]string, 0, len(userIDs))
	for id := range userIDs {
		ids = append(ids, id)
	}
	users, err := authManager.GetUsersByID(ctx, ids, false)
	if err != nil {
		log.Ctx(ctx).Warnf("getting users for disbursement created/approved by: %v", err)
		if draftEntry != nil {
			enrichment.DisbursementCreatedByTimestamp = draftEntry.Timestamp.UTC().Format(disbursementTimestampFormat)
		}
		if startedEntry != nil {
			enrichment.DisbursementApprovedByTimestamp = startedEntry.Timestamp.UTC().Format(disbursementTimestampFormat)
		}
		return
	}
	idToName := make(map[string]string)
	for _, u := range users {
		name := strings.TrimSpace(u.FirstName + " " + u.LastName)
		if name == "" {
			name = u.Email
		}
		idToName[u.ID] = name
	}
	if draftEntry != nil {
		enrichment.DisbursementCreatedByUserName = idToName[draftEntry.UserID]
		enrichment.DisbursementCreatedByTimestamp = draftEntry.Timestamp.UTC().Format(disbursementTimestampFormat)
	}
	if startedEntry != nil {
		enrichment.DisbursementApprovedByUserName = idToName[startedEntry.UserID]
		enrichment.DisbursementApprovedByTimestamp = startedEntry.Timestamp.UTC().Format(disbursementTimestampFormat)
	}
}
