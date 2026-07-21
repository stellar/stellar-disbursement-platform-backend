package httphandler

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/dimchansky/utfbom"
	"github.com/go-chi/chi/v5"
	"github.com/gocarina/gocsv"
	"github.com/stellar/go-stellar-sdk/support/log"
	"github.com/stellar/go-stellar-sdk/support/render/httpjson"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	ctxHelper "github.com/stellar/stellar-disbursement-platform-backend/internal/serve/auth"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpresponse"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

type DisbursementHandler struct {
	Models                        *data.Models
	MonitorService                monitor.MonitorServiceInterface
	AuthManager                   auth.AuthManager
	DisbursementManagementService *services.DisbursementManagementService
	DistributionAccountResolver   signing.DistributionAccountResolver
}

const DefaultMaxCSVUploadSizeBytes = 500 * data.MaxInstructionsPerDisbursement // 500 bytes per instruction.

type PostDisbursementRequest struct {
	Name                                string                       `json:"name"`
	WalletID                            string                       `json:"wallet_id"`
	AssetID                             string                       `json:"asset_id"`
	VerificationField                   data.VerificationType        `json:"verification_field"`
	RegistrationContactType             data.RegistrationContactType `json:"registration_contact_type"`
	ReceiverRegistrationMessageTemplate string                       `json:"receiver_registration_message_template"`
}

func (d DisbursementHandler) validateRequest(ctx context.Context, req PostDisbursementRequest) *validators.Validator {
	v := validators.NewValidator()

	v.Check(req.Name != "", "name", "name is required")
	v.Check(req.AssetID != "", "asset_id", "asset_id is required")
	v.Check(
		slices.Contains(data.AllRegistrationContactTypes(), req.RegistrationContactType),
		"registration_contact_type",
		fmt.Sprintf("registration_contact_type must be one of %v", data.AllRegistrationContactTypes()),
	)
	v.CheckError(utils.ValidateNoHTML(req.ReceiverRegistrationMessageTemplate), "receiver_registration_message_template", "receiver_registration_message_template cannot contain HTML, JS or CSS")
	if !req.RegistrationContactType.IncludesWalletAddress {
		trimmedWalletID := strings.TrimSpace(req.WalletID)
		v.Check(trimmedWalletID != "", "wallet_id", "wallet_id is required")
		var wallet *data.Wallet
		if trimmedWalletID != "" {
			if fetchedWallet, err := d.Models.Wallets.Get(ctx, trimmedWalletID); err == nil {
				wallet = fetchedWallet
			} else {
				v.Check(false, "wallet_id", "wallet_id could not be retrieved")
			}
		}
		walletIsEmbedded := wallet != nil && wallet.Embedded
		if !walletIsEmbedded || req.VerificationField != "" {
			v.Check(
				slices.Contains(data.GetAllVerificationTypes(), req.VerificationField),
				"verification_field",
				fmt.Sprintf("verification_field must be one of %v", data.GetAllVerificationTypes()),
			)
		}
	} else {
		v.Check(req.VerificationField == "", "verification_field", "verification_field is not allowed for this registration contact type")
		v.Check(req.WalletID == "", "wallet_id", "wallet_id is not allowed for this registration contact type")
	}

	return v
}

type PatchDisbursementStatusRequest struct {
	Status string `json:"status"`
}

func (d DisbursementHandler) PostDisbursement(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	user, err := ctxHelper.GetUserFromContext(ctx, d.AuthManager)
	if err != nil {
		httperror.InternalError(ctx, "Cannot get user", err, nil).Render(w)
		return
	}

	if httpErr := d.limitCSVUploadRequestSize(w, r); httpErr != nil {
		httpErr.Render(w)
		return
	}

	// Handle request based on content type.
	var disbursement *data.Disbursement
	var httpErr *httperror.HTTPError
	if utils.IsMultipartFormData(r) {
		disbursement, httpErr = d.postDisbursementWithInstructions(ctx, r, user)
	} else {
		var req PostDisbursementRequest
		if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
			httperror.BadRequest(err.Error(), err, nil).Render(w)
			return
		}
		disbursement, httpErr = d.postDisbursementOnly(ctx, r, req, user)
	}

	if httpErr != nil {
		httpErr.Render(w)
		return
	}

	httpjson.RenderStatus(w, http.StatusCreated, disbursement, httpjson.JSON)
}

func (d DisbursementHandler) createNewDisbursement(ctx context.Context, httpReq *http.Request, sqlExec db.SQLExecuter, userID string, req PostDisbursementRequest) (*data.Disbursement, *httperror.HTTPError) {
	var wallet *data.Wallet
	var err error

	if req.RegistrationContactType.IncludesWalletAddress {
		wallets, findWalletErr := d.Models.Wallets.FindWallets(ctx,
			data.NewFilter(data.FilterUserManaged, true),
			data.NewFilter(data.FilterEnabledWallets, true))

		if findWalletErr != nil {
			return nil, httperror.InternalError(ctx, "Cannot get wallets", findWalletErr, nil)
		}
		if len(wallets) == 0 {
			return nil, httperror.BadRequest("No User Managed Wallets found", nil, nil)
		}
		wallet = &wallets[0]
	} else {
		// Get Wallet
		wallet, err = d.Models.Wallets.Get(ctx, req.WalletID)
		if err != nil {
			return nil, httperror.BadRequest("Wallet ID could not be retrieved", err, nil)
		}
	}

	if !wallet.Enabled {
		return nil, httperror.BadRequest("Wallet is not enabled", errors.New("wallet is not enabled"), nil)
	}

	// Get Asset
	asset, err := d.Models.Assets.Get(ctx, req.AssetID)
	if err != nil {
		return nil, httperror.BadRequest("asset ID could not be retrieved", err, nil)
	}

	//  routing: the source wallet is explicit via X-Wallet-Id (400 absent on multi-wallet
	// tenants, 403 unentitled); single-wallet tenants legitimately omit it and use their
	// default wallet. Entitlement and archived-status checks happen inside.
	sourceWallet, walletErr := resolveSourceWalletForWrite(ctx, httpReq, d.AuthManager, d.Models,
		data.FinancialControllerUserRole, data.InitiatorUserRole)
	if walletErr != nil {
		return nil, walletErr
	}

	// Insert disbursement
	disbursement := data.Disbursement{
		SourceWalletID:                      sourceWallet.ID,
		Asset:                               asset,
		Name:                                req.Name,
		ReceiverRegistrationMessageTemplate: req.ReceiverRegistrationMessageTemplate,
		RegistrationContactType:             req.RegistrationContactType,
		VerificationField:                   req.VerificationField,
		Wallet:                              wallet,
		Status:                              data.DraftDisbursementStatus,
		StatusHistory: []data.DisbursementStatusHistoryEntry{{
			Timestamp: time.Now(),
			Status:    data.DraftDisbursementStatus,
			UserID:    userID,
		}},
	}
	newID, err := d.Models.Disbursements.Insert(ctx, sqlExec, &disbursement)
	if err != nil {
		if errors.Is(err, data.ErrRecordAlreadyExists) {
			return nil, httperror.Conflict("disbursement already exists", err, nil)
		} else {
			return nil, httperror.BadRequest("could not create disbursement", err, nil)
		}
	}

	// Outbox: disbursement.created, same executor as the insert.
	if err = events.Write(ctx, sqlExec, events.DisbursementCreated, sourceWallet.ID, map[string]any{
		"disbursement_id": newID,
		"name":            disbursement.Name,
		"actor_user_id":   userID,
	}); err != nil {
		return nil, httperror.InternalError(ctx, "Cannot write disbursement.created event", err, nil)
	}
	newDisbursement, err := d.Models.Disbursements.Get(ctx, sqlExec, newID)
	if err != nil {
		msg := fmt.Sprintf("Cannot retrieve disbursement for ID: %s", newID)
		return nil, httperror.InternalError(ctx, msg, err, nil)
	}

	d.recordCreateDisbursementMetrics(ctx, newDisbursement)

	return newDisbursement, nil
}

func (d DisbursementHandler) recordCreateDisbursementMetrics(ctx context.Context, disbursement *data.Disbursement) {
	labels := monitor.DisbursementLabels{
		Asset:    disbursement.Asset.Code,
		Wallet:   disbursement.Wallet.Name,
		WalletID: disbursement.SourceWalletID,
		CommonLabels: monitor.CommonLabels{
			TenantName: sdpcontext.MustGetTenantNameFromContext(ctx),
		},
	}

	if err := d.MonitorService.MonitorCounters(monitor.DisbursementsCounterTag, labels.ToMap()); err != nil {
		log.Ctx(ctx).Errorf("Error trying to monitor disbursement counter: %s", err)
	}
}

// DeleteDisbursement deletes a draft or ready disbursement and its associated payments
func (d DisbursementHandler) DeleteDisbursement(w http.ResponseWriter, r *http.Request) {
	disbursementID := chi.URLParam(r, "id")
	ctx := r.Context()

	ErrDisbursementStarted := errors.New("can't delete disbursement that has started")

	disbursement, err := db.RunInTransactionWithResult(ctx, d.Models.DBConnectionPool, nil, func(tx db.DBTransaction) (*data.Disbursement, error) {
		// Check if disbursement exists and is in draft or ready status
		disbursement, err := d.Models.Disbursements.Get(ctx, tx, disbursementID)
		if err != nil {
			return nil, fmt.Errorf("getting disbursement: %w", err)
		}

		if !slices.Contains(data.NotStartedDisbursementStatuses, disbursement.Status) {
			return nil, ErrDisbursementStarted
		}

		// deletion is a state transition — gate on the disbursement's source wallet.
		if httpErr := ensureWalletActionAllowed(ctx, d.AuthManager, d.Models, disbursement.SourceWalletID,
			data.FinancialControllerUserRole, data.InitiatorUserRole); httpErr != nil {
			return nil, httpErr
		}

		// Delete associated payments
		err = d.Models.Payment.DeleteAllDraftForDisbursement(ctx, tx, disbursementID)
		if err != nil {
			return nil, fmt.Errorf("deleting payments: %w", err)
		}

		// Delete disbursement
		err = d.Models.Disbursements.Delete(ctx, tx, disbursementID)
		if err != nil {
			return nil, fmt.Errorf("deleting draft or ready disbursement: %w", err)
		}

		// Outbox: a deleted draft is a rejected disbursement, same transaction.
		if err = events.Write(ctx, tx, events.DisbursementRejected, disbursement.SourceWalletID, map[string]any{
			"disbursement_id": disbursementID,
			"name":            disbursement.Name,
			"reason":          "draft_deleted",
		}); err != nil {
			return nil, fmt.Errorf("writing disbursement.rejected event: %w", err)
		}

		return disbursement, nil
	})
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			httperror.NotFound("Disbursement not found", err, nil).Render(w)
		case errors.Is(err, ErrDisbursementStarted):
			httperror.BadRequest("Cannot delete a disbursement that has started", err, nil).Render(w)
		default:
			var httpErr *httperror.HTTPError
			if errors.As(err, &httpErr) {
				httpErr.Render(w)
				return
			}
			httperror.InternalError(ctx, "Cannot delete disbursement", err, nil).Render(w)
		}
		return
	}

	httpjson.RenderStatus(w, http.StatusOK, disbursement, httpjson.JSON)
}

// GetDisbursements returns a paginated list of disbursements
func (d DisbursementHandler) GetDisbursements(w http.ResponseWriter, r *http.Request) {
	validator := validators.NewDisbursementQueryValidator()
	queryParams := validator.ParseParametersFromRequest(r)

	if validator.HasErrors() {
		httperror.BadRequest("request invalid", nil, validator.Errors).Render(w)
		return
	}

	queryParams.Filters = validator.ValidateAndGetDisbursementFilters(queryParams.Filters)
	if validator.HasErrors() {
		httperror.BadRequest("request invalid", nil, validator.Errors).Render(w)
		return
	}

	ctx := r.Context()

	// Per-account list scope: narrow to the selected account (X-Wallet-Id) when set, else full
	// visibility (owner tenant-wide, member = their wallets).
	scope, scopeErr := resolveWalletListScope(ctx, r, d.AuthManager, d.Models)
	if scopeErr != nil {
		scopeErr.Render(w)
		return
	}
	if scope != nil {
		queryParams.Filters[data.FilterKeySourceWalletIDs] = scope
	}

	resultWithTotal, err := d.DisbursementManagementService.GetDisbursementsWithCount(ctx, queryParams)
	if err != nil {
		httperror.InternalError(ctx, "Cannot retrieve disbursements", err, nil).Render(w)
		return
	}
	if resultWithTotal.Total == 0 {
		httpjson.RenderStatus(w, http.StatusOK, httpresponse.NewEmptyPaginatedResponse(), httpjson.JSON)
		return
	}

	response, errGet := httpresponse.NewPaginatedResponse(r, resultWithTotal.Result, queryParams.Page, queryParams.PageLimit, resultWithTotal.Total)
	if errGet != nil {
		httperror.InternalError(ctx, "Cannot write paginated response for disbursements", errGet, nil).Render(w)
		return
	}

	httpjson.RenderStatus(w, http.StatusOK, response, httpjson.JSON)
}

func (d DisbursementHandler) PostDisbursementInstructions(w http.ResponseWriter, r *http.Request) {
	disbursementID := chi.URLParam(r, "id")
	ctx := r.Context()

	user, err := ctxHelper.GetUserFromContext(ctx, d.AuthManager)
	if err != nil {
		msg := fmt.Sprintf("Cannot get user from context token when processing instructions for disbursement with ID %s", disbursementID)
		httperror.InternalError(ctx, msg, err, nil).Render(w)
		return
	}

	if httpErr := d.limitCSVUploadRequestSize(w, r); httpErr != nil {
		httpErr.Render(w)
		return
	}

	if err = db.RunInTransaction(ctx, d.Models.DBConnectionPool, nil, func(dbTx db.DBTransaction) error {
		// check if disbursement exists
		disbursement, getErr := d.Models.Disbursements.Get(ctx, dbTx, disbursementID)
		if getErr == nil {
			// instruction upload routes funds from the disbursement's wallet — gate it.
			if httpErr := ensureWalletActionAllowed(ctx, d.AuthManager, d.Models, disbursement.SourceWalletID,
				data.FinancialControllerUserRole, data.InitiatorUserRole); httpErr != nil {
				return httpErr
			}
		}
		if getErr != nil {
			return httperror.BadRequest("disbursement ID is invalid", getErr, nil)
		}

		// check if disbursement is in draft, ready status
		if !slices.Contains([]data.DisbursementStatus{data.DraftDisbursementStatus, data.ReadyDisbursementStatus}, disbursement.Status) {
			return httperror.BadRequest("disbursement is not in draft or ready status", nil, nil)
		}

		return d.validateAndProcessInstructions(ctx, r, dbTx, user, disbursement)
	}); err != nil {
		var httpErr *httperror.HTTPError
		if errors.As(err, &httpErr) {
			httpErr.Render(w)
		} else {
			httperror.InternalError(ctx, "Cannot process instructions for disbursement", err, nil).Render(w)
		}
		return
	}

	response := map[string]string{
		"message": "File uploaded successfully",
	}

	httpjson.Render(w, response, httpjson.JSON)
}

func (d DisbursementHandler) limitCSVUploadRequestSize(w http.ResponseWriter, r *http.Request) *httperror.HTTPError {
	if !utils.IsMultipartFormData(r) {
		return nil
	}

	r.Body = http.MaxBytesReader(w, r.Body, DefaultMaxCSVUploadSizeBytes)
	if err := r.ParseMultipartForm(DefaultMaxCSVUploadSizeBytes); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			err = fmt.Errorf("request body too large: %w", err)
			log.Ctx(r.Context()).Error(err)
			return httperror.BadRequest("could not parse multipart form data", err, map[string]interface{}{
				"details": fmt.Sprintf("request too large. Max size %d bytes.", DefaultMaxCSVUploadSizeBytes),
			})
		}

		err = fmt.Errorf("parsing multipart form: %w", err)
		return httperror.BadRequest("could not parse multipart form data", err, nil)
	}

	return nil
}

func (d DisbursementHandler) validateAndProcessInstructions(ctx context.Context, r *http.Request, dbTx db.DBTransaction, authUser *auth.User, disbursement *data.Disbursement) error {
	buf, header, parseHTTPErr := parseCsvFromMultipartRequest(r)
	if parseHTTPErr != nil {
		return fmt.Errorf("could not parse csv file: %w", parseHTTPErr)
	}

	skipVerification := disbursement.Wallet != nil && disbursement.Wallet.Embedded && disbursement.VerificationField == ""
	if err := validateCSVHeaders(bytes.NewReader(buf.Bytes()), disbursement.RegistrationContactType, skipVerification); err != nil {
		errMsg := fmt.Sprintf("CSV columns are not valid for registration contact type %s: %s",
			disbursement.RegistrationContactType,
			err)
		return httperror.BadRequest(errMsg, err, nil)
	}

	instructions, v := parseInstructionsFromCSV(ctx, bytes.NewReader(buf.Bytes()), disbursement.RegistrationContactType, disbursement.VerificationField)
	if v != nil && v.HasErrors() {
		return httperror.BadRequest("could not parse csv file", nil, v.Errors)
	}

	disbursementUpdate := &data.DisbursementUpdate{
		ID:          disbursement.ID,
		FileName:    header.Filename,
		FileContent: buf.Bytes(),
	}

	if err := d.Models.DisbursementInstructions.ProcessAll(ctx, dbTx, data.DisbursementInstructionsOpts{
		UserID:                  authUser.ID,
		Instructions:            instructions,
		Disbursement:            disbursement,
		DisbursementUpdate:      disbursementUpdate,
		MaxNumberOfInstructions: data.MaxInstructionsPerDisbursement,
	}); err != nil {
		switch {
		case errors.Is(err, data.ErrMaxInstructionsExceeded):
			return httperror.BadRequest(fmt.Sprintf("number of instructions exceeds maximum of %d", data.MaxInstructionsPerDisbursement), err, nil)
		case errors.Is(err, data.ErrReceiverVerificationMismatch):
			return httperror.BadRequest(errors.Unwrap(err).Error(), err, nil)
		case errors.Is(err, data.ErrReceiverWalletAddressMismatch):
			return httperror.BadRequest(errors.Unwrap(err).Error(), err, nil)
		case errors.Is(err, data.ErrDuplicateWalletAddress):
			return httperror.Conflict(err.Error(), err, nil)
		default:
			return httperror.InternalError(ctx, fmt.Sprintf("Cannot process instructions for disbursement with ID %s", disbursement.ID), err, nil)
		}
	}

	return nil
}

// parseCsvFromMultipartRequest parses the CSV file from a multipart request and returns the file content and header,
// or an error if the file is not a valid CSV or the MIME type is not text/csv.
func parseCsvFromMultipartRequest(r *http.Request) (*bytes.Buffer, *multipart.FileHeader, *httperror.HTTPError) {
	// Parse uploaded CSV file
	file, header, err := r.FormFile("file")
	if err != nil {
		return nil, nil, httperror.BadRequest("could not parse file", err, nil)
	}
	defer utils.DeferredClose(r.Context(), file, "closing file")

	if err = utils.ValidatePathIsNotTraversal(header.Filename); err != nil {
		return nil, nil, httperror.BadRequest("file name contains invalid traversal pattern", nil, nil)
	}

	if filepath.Ext(header.Filename) != ".csv" {
		return nil, nil, httperror.BadRequest("the file extension should be .csv", nil, nil)
	}

	var buf bytes.Buffer
	if _, err = io.Copy(&buf, file); err != nil {
		return nil, nil, httperror.BadRequest("could not read file", err, nil)
	}

	return &buf, header, nil
}

func (d DisbursementHandler) GetDisbursement(w http.ResponseWriter, r *http.Request) {
	disbursementID := chi.URLParam(r, "id")

	ctx := r.Context()
	disbursement, err := d.Models.Disbursements.GetWithStatistics(ctx, disbursementID)
	if err != nil {
		if errors.Is(err, data.ErrRecordNotFound) {
			httperror.NotFound("disbursement not found", err, nil).Render(w)
		} else {
			msg := fmt.Sprintf("Cannot get receivers for disbursement with ID: %s", disbursementID)
			httperror.InternalError(ctx, msg, err, nil).Render(w)
		}
		return
	}

	// Membership-filtered visibility: respond 404 outside the caller's scope —
	// existence is never disclosed (read-leakage rules).
	scope, scopeErr := resolveWalletReadScope(ctx, d.AuthManager, d.Models)
	if scopeErr != nil {
		scopeErr.Render(w)
		return
	}
	if !walletInReadScope(scope, disbursement.SourceWalletID) {
		httperror.NotFound("disbursement not found", nil, nil).Render(w)
		return
	}

	response, err := d.DisbursementManagementService.AppendUserMetadata(ctx, []*data.Disbursement{disbursement})
	if err != nil {
		httperror.NotFound("disbursement user metadata not found", err, nil).Render(w)
	}
	if len(response) != 1 {
		httperror.InternalError(
			ctx, fmt.Sprintf("Size of response is unexpected: %d", len(response)), nil, nil,
		).Render(w)
	}

	httpjson.Render(w, response[0], httpjson.JSON)
}

func (d DisbursementHandler) GetDisbursementReceivers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	disbursementID := chi.URLParam(r, "id")

	validator := validators.NewReceiverQueryValidator()
	queryParams := validator.ParseParametersFromRequest(r)

	if validator.HasErrors() {
		httperror.BadRequest("request invalid", nil, validator.Errors).Render(w)
		return
	}

	// Membership-filtered visibility: 404 outside the caller's scope.
	if httpErr := d.ensureDisbursementInReadScope(ctx, disbursementID); httpErr != nil {
		httpErr.Render(w)
		return
	}

	resultWithTotal, err := d.DisbursementManagementService.GetDisbursementReceiversWithCount(ctx, disbursementID, queryParams)
	if err != nil {
		if errors.Is(err, services.ErrDisbursementNotFound) {
			httperror.NotFound("disbursement not found", err, nil).Render(w)
		} else {
			msg := fmt.Sprintf("Cannot find disbursement with ID: %s", disbursementID)
			httperror.InternalError(ctx, msg, err, nil).Render(w)
		}
		return
	}

	if resultWithTotal.Total == 0 {
		httpjson.RenderStatus(w, http.StatusOK, httpresponse.NewEmptyPaginatedResponse(), httpjson.JSON)
		return
	}

	response, err := httpresponse.NewPaginatedResponse(r, resultWithTotal.Result, queryParams.Page, queryParams.PageLimit, resultWithTotal.Total)
	if err != nil {
		msg := fmt.Sprintf("Cannot write paginated response for disbursement with ID: %s", disbursementID)
		httperror.InternalError(ctx, msg, err, nil).Render(w)
		return
	}

	httpjson.RenderStatus(w, http.StatusOK, response, httpjson.JSON)
}

type UpdateDisbursementStatusResponseBody struct {
	Message string `json:"message"`
}

// ensureDisbursementInReadScope loads the disbursement and verifies the caller's wallet
// scope covers its source wallet, returning 404 (never 403) outside the scope.
func (d DisbursementHandler) ensureDisbursementInReadScope(ctx context.Context, disbursementID string) *httperror.HTTPError {
	disbursement, err := d.Models.Disbursements.Get(ctx, d.Models.DBConnectionPool, disbursementID)
	if err != nil {
		if errors.Is(err, data.ErrRecordNotFound) {
			return httperror.NotFound("disbursement not found", err, nil)
		}
		return httperror.InternalError(ctx, "Cannot load disbursement", err, nil)
	}

	scope, scopeErr := resolveWalletReadScope(ctx, d.AuthManager, d.Models)
	if scopeErr != nil {
		return scopeErr
	}
	if !walletInReadScope(scope, disbursement.SourceWalletID) {
		return httperror.NotFound("disbursement not found", nil, nil)
	}
	return nil
}

// PatchDisbursementStatus updates the status of a disbursement
func (d DisbursementHandler) PatchDisbursementStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var patchRequest PatchDisbursementStatusRequest
	err := json.NewDecoder(r.Body).Decode(&patchRequest)
	if err != nil {
		httperror.BadRequest("invalid request body", err, nil).Render(w)
		return
	}

	// validate request
	toStatus, err := data.ToDisbursementStatus(patchRequest.Status)
	if err != nil {
		httperror.BadRequest("invalid status", err, nil).Render(w)
		return
	}

	response := UpdateDisbursementStatusResponseBody{}

	disbursementID := chi.URLParam(r, "id")

	user, err := ctxHelper.GetUserFromContext(ctx, d.AuthManager)
	if err != nil {
		httperror.InternalError(ctx, "Cannot get user from context", err, nil).Render(w)
		return
	}

	switch toStatus {
	case data.StartedDisbursementStatus:
		// The balance check must run against THIS disbursement's own source wallet, not the
		// tenant's legacy single distribution account (DistributionAccountFromContext) - on a
		// multi-wallet tenant those are frequently different accounts, and checking the wrong
		// one either wrongly blocks a valid disbursement or (worse) validates against an
		// account with no relation to the funds actually being spent.
		disbursement, getErr := d.Models.Disbursements.Get(ctx, d.Models.DBConnectionPool, disbursementID)
		if getErr != nil {
			if errors.Is(getErr, data.ErrRecordNotFound) {
				httperror.NotFound("disbursement not found", getErr, nil).Render(w)
				return
			}
			httperror.InternalError(ctx, "Cannot get disbursement", getErr, nil).Render(w)
			return
		}

		sourceWallet, walletErr := d.Models.DistributionWallets.Get(ctx, d.Models.DBConnectionPool, disbursement.SourceWalletID)
		if walletErr != nil {
			httperror.InternalError(ctx, "Cannot get distribution account", walletErr, nil).Render(w)
			return
		}
		if sourceWallet.Address == nil || *sourceWallet.Address == "" {
			httperror.BadRequest("the disbursement's source wallet has no funded distribution account yet", nil, nil).Render(w)
			return
		}

		distributionAccount := schema.TransactionAccount{
			Address: *sourceWallet.Address,
			Type:    sourceWallet.AccountType,
			Status:  sourceWallet.AccountStatus,
		}

		err = d.DisbursementManagementService.StartDisbursement(ctx, disbursementID, user, &distributionAccount)
		response.Message = "Disbursement started"
	case data.PausedDisbursementStatus:
		err = d.DisbursementManagementService.PauseDisbursement(ctx, disbursementID, user)
		response.Message = "Disbursement paused"
	default:
		err = services.ErrDisbursementStatusCantBeChanged
	}

	var insufficientBalanceErr services.InsufficientBalanceError
	if err != nil {
		switch {
		case errors.Is(err, services.ErrDisbursementNotFound):
			httperror.NotFound(services.ErrDisbursementNotFound.Error(), err, nil).Render(w)
		case errors.Is(err, services.ErrDisbursementNotReadyToStart):
			httperror.BadRequest(services.ErrDisbursementNotReadyToStart.Error(), err, nil).Render(w)
		case errors.Is(err, services.ErrDisbursementNotReadyToPause):
			httperror.BadRequest(services.ErrDisbursementNotReadyToPause.Error(), err, nil).Render(w)
		case errors.Is(err, services.ErrDisbursementStatusCantBeChanged):
			httperror.BadRequest(services.ErrDisbursementStatusCantBeChanged.Error(), err, nil).Render(w)
		case errors.Is(err, services.ErrDisbursementStartedByCreator):
			httperror.Forbidden("Disbursement can't be started by its creator. Approval by another user is required.", err, nil).Render(w)
		case errors.Is(err, services.ErrWalletActionForbidden):
			httperror.Forbidden(services.ErrWalletActionForbidden.Error(), err, nil).Render(w)
		case errors.Is(err, services.ErrDisbursementWalletDisabled):
			httperror.BadRequest(services.ErrDisbursementWalletDisabled.Error(), err, nil).Render(w)
		case errors.As(err, &insufficientBalanceErr):
			log.Ctx(ctx).Error(insufficientBalanceErr)
			httperror.Conflict(insufficientBalanceErr.Error(), err, nil).Render(w)
		default:
			msg := fmt.Sprintf("Cannot update disbursementID=%s with status=%s: %v", disbursementID, toStatus, err)
			httperror.InternalError(ctx, msg, err, nil).Render(w)
		}
		return
	}

	httpjson.RenderStatus(w, http.StatusOK, response, httpjson.JSON)
}

func (d DisbursementHandler) GetDisbursementInstructions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	disbursementID := chi.URLParam(r, "id")

	disbursement, err := d.Models.Disbursements.Get(ctx, d.Models.DBConnectionPool, disbursementID)
	if err != nil {
		httperror.NotFound("disbursement not found", err, nil).Render(w)
		return
	}

	// Membership-filtered visibility: 404 outside the caller's scope.
	scope, scopeErr := resolveWalletReadScope(ctx, d.AuthManager, d.Models)
	if scopeErr != nil {
		scopeErr.Render(w)
		return
	}
	if !walletInReadScope(scope, disbursement.SourceWalletID) {
		httperror.NotFound("disbursement not found", nil, nil).Render(w)
		return
	}

	if len(disbursement.FileContent) == 0 {
		err = fmt.Errorf("disbursement %s has no instructions file", disbursementID)
		log.Ctx(ctx).Error(err)
		httperror.NotFound(err.Error(), err, nil).Render(w)
		return
	}

	filename := disbursement.FileName
	if filepath.Ext(filename) != ".csv" { // add .csv extension if missing
		filename = filename + ".csv"
	}

	// `attachment` returns a file-download prompt. change that to `inline` to open in browser
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Content-Type", "text/csv")
	_, err = w.Write(disbursement.FileContent)
	if err != nil {
		httperror.InternalError(ctx, "Cannot write disbursement instructions to response", err, nil).Render(w)
	}
}

func (d DisbursementHandler) postDisbursementWithInstructions(ctx context.Context, r *http.Request, user *auth.User) (*data.Disbursement, *httperror.HTTPError) {
	var req PostDisbursementRequest
	dd := r.FormValue("data")
	if err := json.Unmarshal([]byte(dd), &req); err != nil {
		return nil, httperror.BadRequest(err.Error(), err, nil)
	}

	disbursement, err := db.RunInTransactionWithResult(ctx, d.Models.DBConnectionPool, nil, func(tx db.DBTransaction) (*data.Disbursement, error) {
		// 1. Create the Disbursement
		disbursement, httpErr := d.createNewDisbursement(ctx, r, tx, user.ID, req)
		if httpErr != nil {
			return nil, httpErr
		}

		// 2. Process the instructions
		if err := d.validateAndProcessInstructions(ctx, r, tx, user, disbursement); err != nil {
			return nil, fmt.Errorf("could not process instructions: %w", err)
		}

		return disbursement, nil
	})
	if err != nil {
		var httpErr *httperror.HTTPError
		if errors.As(err, &httpErr) {
			return nil, httpErr
		} else {
			return nil, httperror.InternalError(ctx, "Cannot create disbursement", err, nil)
		}
	}
	return disbursement, nil
}

func (d DisbursementHandler) postDisbursementOnly(ctx context.Context, r *http.Request, req PostDisbursementRequest, user *auth.User) (*data.Disbursement, *httperror.HTTPError) {
	v := d.validateRequest(ctx, req)
	if v.HasErrors() {
		return nil, httperror.BadRequest("", nil, v.Errors)
	}

	return d.createNewDisbursement(ctx, r, d.Models.DBConnectionPool, user.ID, req)
}

// parseInstructionsFromCSV parses the CSV file and returns a list of DisbursementInstructions
func parseInstructionsFromCSV(ctx context.Context, reader io.Reader, contactType data.RegistrationContactType, verificationField data.VerificationType) ([]*data.DisbursementInstruction, *validators.DisbursementInstructionsValidator) {
	validator := validators.NewDisbursementInstructionsValidator(contactType, verificationField)

	instructions := []*data.DisbursementInstruction{}
	if err := gocsv.Unmarshal(reader, &instructions); err != nil {
		log.Ctx(ctx).Errorf("error parsing csv file: %s", err.Error())
		validator.Errors["file"] = "could not parse file"
		return nil, validator
	}

	var sanitizedInstructions []*data.DisbursementInstruction
	for i, instruction := range instructions {
		sanitizedInstruction := validator.SanitizeInstruction(instruction)
		lineNumber := i + 2 // +1 for header row, +1 for 0-index
		validator.ValidateInstruction(sanitizedInstruction, lineNumber)
		sanitizedInstructions = append(sanitizedInstructions, sanitizedInstruction)
	}

	validator.Check(len(sanitizedInstructions) > 0, "instructions", "no valid instructions found")

	validator.CheckForDuplicateContacts(instructions)

	if validator.HasErrors() {
		return nil, validator
	}

	return sanitizedInstructions, nil
}

// validateCSVHeaders validates the headers of the CSV file to make sure we're passing the correct columns.
func validateCSVHeaders(file io.Reader, registrationContactType data.RegistrationContactType, skipVerification bool) error {
	const (
		phoneHeader             = "phone"
		emailHeader             = "email"
		walletAddressHeader     = "walletAddress"
		walletAddressMemoHeader = "walletAddressMemo"
		verificationHeader      = "verification"
	)

	headers, err := csv.NewReader(utfbom.SkipOnly(file)).Read()
	if err != nil {
		return fmt.Errorf("reading csv headers: %w", err)
	}

	hasHeaders := map[string]bool{
		phoneHeader:             false,
		emailHeader:             false,
		walletAddressHeader:     false,
		walletAddressMemoHeader: false,
		verificationHeader:      false,
	}

	// Populate header presence map
	for _, header := range headers {
		if _, exists := hasHeaders[header]; exists {
			hasHeaders[header] = true
		}
	}

	// establish the header rules. Each registration contact type has its own rules.
	type headerRules struct {
		required   []string
		disallowed []string
	}

	rules := map[data.RegistrationContactType]headerRules{
		data.RegistrationContactTypePhone: {
			required:   []string{phoneHeader, verificationHeader},
			disallowed: []string{emailHeader, walletAddressHeader, walletAddressMemoHeader},
		},
		data.RegistrationContactTypeEmail: {
			required:   []string{emailHeader, verificationHeader},
			disallowed: []string{phoneHeader, walletAddressHeader, walletAddressMemoHeader},
		},
		data.RegistrationContactTypeEmailAndWalletAddress: {
			required:   []string{emailHeader, walletAddressHeader},
			disallowed: []string{phoneHeader, verificationHeader},
		},
		data.RegistrationContactTypePhoneAndWalletAddress: {
			required:   []string{phoneHeader, walletAddressHeader},
			disallowed: []string{emailHeader, verificationHeader},
		},
	}

	rule := rules[registrationContactType]
	if skipVerification {
		// filter out the verification header from required headers
		filtered := rule.required[:0]
		for _, header := range rule.required {
			if header != verificationHeader {
				filtered = append(filtered, header)
			}
		}
		rule.required = filtered
		// And add it to disallowed
		rule.disallowed = append(rule.disallowed, verificationHeader)
	}

	// Validate headers according to the rules
	for _, req := range rule.required {
		if !hasHeaders[req] {
			return fmt.Errorf("%s column is required", req)
		}
	}
	for _, dis := range rule.disallowed {
		if hasHeaders[dis] {
			return fmt.Errorf("%s column is not allowed for this registration contact type", dis)
		}
	}

	return nil
}
