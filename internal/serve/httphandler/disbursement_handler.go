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

	"github.com/go-chi/chi/v5"
	"github.com/gocarina/gocsv"
	"github.com/stellar/go/support/log"
	"github.com/stellar/go/support/render/httpjson"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpresponse"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/middleware"
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

type PostDisbursementRequest struct {
	Name                                string                       `json:"name"`
	WalletID                            string                       `json:"wallet_id"`
	AssetID                             string                       `json:"asset_id"`
	VerificationField                   data.VerificationType        `json:"verification_field"`
	RegistrationContactType             data.RegistrationContactType `json:"registration_contact_type"`
	ReceiverRegistrationMessageTemplate string                       `json:"receiver_registration_message_template"`
}

func (d DisbursementHandler) validateRequest(req PostDisbursementRequest) *validators.Validator {
	v := validators.NewValidator()

	v.Check(req.Name != "", "name", "name is required")
	v.Check(req.WalletID != "", "wallet_id", "wallet_id is required")
	v.Check(req.AssetID != "", "asset_id", "asset_id is required")
	v.Check(
		slices.Contains(data.AllRegistrationContactTypes(), req.RegistrationContactType),
		"registration_contact_type",
		fmt.Sprintf("registration_contact_type must be one of %v", data.AllRegistrationContactTypes()),
	)
	if !req.RegistrationContactType.IncludesWalletAddress {
		v.Check(
			slices.Contains(data.GetAllVerificationTypes(), req.VerificationField),
			"verification_field",
			fmt.Sprintf("verification_field must be one of %v", data.GetAllVerificationTypes()),
		)
	}

	return v
}

type PatchDisbursementStatusRequest struct {
	Status string `json:"status"`
}

func (d DisbursementHandler) PostDisbursement(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Grab token and user
	token, ok := ctx.Value(middleware.TokenContextKey).(string)
	if !ok {
		httperror.Unauthorized("", nil, nil).Render(w)
		return
	}
	user, err := d.AuthManager.GetUser(ctx, token)
	if err != nil {
		httperror.InternalError(ctx, "Cannot get user", err, nil).Render(w)
		return
	}

	// Decode and validate body
	var req PostDisbursementRequest
	err = json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		httperror.BadRequest(err.Error(), err, nil).Render(w)
		return
	}
	v := d.validateRequest(req)
	if v.HasErrors() {
		httperror.BadRequest("", err, v.Errors).Render(w)
		return
	}

	// Get Wallet
	wallet, err := d.Models.Wallets.Get(ctx, req.WalletID)
	if err != nil {
		httperror.BadRequest("wallet ID could not be retrieved", err, nil).Render(w)
		return
	}
	if !wallet.Enabled {
		httperror.BadRequest("wallet is not enabled", errors.New("wallet is not enabled"), nil).Render(w)
		return
	}

	// Get Asset
	asset, err := d.Models.Assets.Get(ctx, req.AssetID)
	if err != nil {
		httperror.BadRequest("asset ID could not be retrieved", err, nil).Render(w)
		return
	}

	// Insert disbursement
	disbursement := data.Disbursement{
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
			UserID:    user.ID,
		}},
	}
	newId, err := d.Models.Disbursements.Insert(ctx, &disbursement)
	if err != nil {
		if errors.Is(err, data.ErrRecordAlreadyExists) {
			httperror.Conflict("disbursement already exists", err, nil).Render(w)
		} else {
			httperror.BadRequest("could not create disbursement", err, nil).Render(w)
		}
		return
	}
	newDisbursement, err := d.Models.Disbursements.Get(ctx, d.Models.DBConnectionPool, newId)
	if err != nil {
		msg := fmt.Sprintf("Cannot retrieve disbursement for ID: %s", newId)
		httperror.InternalError(ctx, msg, err, nil).Render(w)
		return
	}

	// Monitor disbursement creation
	labels := monitor.DisbursementLabels{
		Asset:  newDisbursement.Asset.Code,
		Wallet: newDisbursement.Wallet.Name,
	}
	err = d.MonitorService.MonitorCounters(monitor.DisbursementsCounterTag, labels.ToMap())
	if err != nil {
		log.Ctx(ctx).Errorf("Error trying to monitor disbursement counter: %s", err)
	}

	httpjson.RenderStatus(w, http.StatusCreated, newDisbursement, httpjson.JSON)
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

	// check if disbursement exists
	ctx := r.Context()
	disbursement, err := d.Models.Disbursements.Get(ctx, d.Models.DBConnectionPool, disbursementID)
	if err != nil {
		httperror.BadRequest("disbursement ID is invalid", err, nil).Render(w)
		return
	}

	// check if disbursement is in draft, ready status
	if !slices.Contains([]data.DisbursementStatus{data.DraftDisbursementStatus, data.ReadyDisbursementStatus}, disbursement.Status) {
		httperror.BadRequest("disbursement is not in draft or ready status", nil, nil).Render(w)
		return
	}

	buf, header, httpErr := parseCsvFromMultipartRequest(r)
	if httpErr != nil {
		httpErr.Render(w)
		return
	}

	contactType, err := resolveReceiverContactType(bytes.NewReader(buf.Bytes()))
	if err != nil {
		errMsg := fmt.Sprintf("could not determine contact information type: %s", err)
		httperror.BadRequest(errMsg, err, nil).Render(w)
		return
	}

	instructions, v := parseInstructionsFromCSV(ctx, bytes.NewReader(buf.Bytes()), disbursement.VerificationField)
	if v != nil && v.HasErrors() {
		httperror.BadRequest("could not parse csv file", err, v.Errors).Render(w)
		return
	}

	disbursementUpdate := &data.DisbursementUpdate{
		ID:          disbursementID,
		FileName:    header.Filename,
		FileContent: buf.Bytes(),
	}

	token, ok := ctx.Value(middleware.TokenContextKey).(string)
	if !ok {
		msg := fmt.Sprintf("Cannot get token from context when processing instructions for disbursement with ID %s", disbursementID)
		httperror.InternalError(ctx, msg, err, nil).Render(w)
		return
	}
	user, err := d.AuthManager.GetUser(ctx, token)
	if err != nil {
		msg := fmt.Sprintf("Cannot get token from context when processing instructions for disbursement with ID %s", disbursementID)
		httperror.InternalError(ctx, msg, err, nil).Render(w)
		return
	}

	if err = d.Models.DisbursementInstructions.ProcessAll(ctx, data.DisbursementInstructionsOpts{
		UserID:                  user.ID,
		Instructions:            instructions,
		ReceiverContactType:     contactType,
		Disbursement:            disbursement,
		DisbursementUpdate:      disbursementUpdate,
		MaxNumberOfInstructions: data.MaxInstructionsPerDisbursement,
	}); err != nil {
		switch {
		case errors.Is(err, data.ErrMaxInstructionsExceeded):
			httperror.BadRequest(fmt.Sprintf("number of instructions exceeds maximum of %d", data.MaxInstructionsPerDisbursement), err, nil).Render(w)
		case errors.Is(err, data.ErrReceiverVerificationMismatch):
			httperror.BadRequest(errors.Unwrap(err).Error(), err, nil).Render(w)
		default:
			httperror.InternalError(ctx, fmt.Sprintf("Cannot process instructions for disbursement with ID %s", disbursementID), err, nil).Render(w)
		}
		return
	}

	response := map[string]string{
		"message": "File uploaded successfully",
	}

	httpjson.Render(w, response, httpjson.JSON)
}

// parseCsvFromMultipartRequest parses the CSV file from a multipart request and returns the file content and header,
// or an error if the file is not a valid CSV or the MIME type is not text/csv.
func parseCsvFromMultipartRequest(r *http.Request) (*bytes.Buffer, *multipart.FileHeader, *httperror.HTTPError) {
	// Parse uploaded CSV file
	file, header, err := r.FormFile("file")
	if err != nil {
		return nil, nil, httperror.BadRequest("could not parse file", err, nil)
	}
	defer file.Close()

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

	token, ok := ctx.Value(middleware.TokenContextKey).(string)
	if !ok {
		httperror.InternalError(ctx, "Cannot get token from context", err, nil).Render(w)
	}
	user, err := d.AuthManager.GetUser(ctx, token)
	if err != nil {
		httperror.InternalError(ctx, "Cannot get user from token", err, nil).Render(w)
	}

	switch toStatus {
	case data.StartedDisbursementStatus:
		var distributionAccount schema.TransactionAccount
		if distributionAccount, err = d.DistributionAccountResolver.DistributionAccountFromContext(ctx); err != nil {
			httperror.InternalError(ctx, "Cannot get distribution account", err, nil).Render(w)
			return
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

// parseInstructionsFromCSV parses the CSV file and returns a list of DisbursementInstructions
func parseInstructionsFromCSV(ctx context.Context, reader io.Reader, verificationField data.VerificationType) ([]*data.DisbursementInstruction, *validators.DisbursementInstructionsValidator) {
	validator := validators.NewDisbursementInstructionsValidator(verificationField)

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

	if validator.HasErrors() {
		return nil, validator
	}

	return sanitizedInstructions, nil
}

// resolveReceiverContactType determines the type of contact information in the CSV file
func resolveReceiverContactType(file io.Reader) (data.ReceiverContactType, error) {
	headers, err := csv.NewReader(file).Read()
	if err != nil {
		return "", fmt.Errorf("reading csv headers: %w", err)
	}

	var hasPhone, hasEmail bool
	for _, header := range headers {
		switch strings.ToLower(strings.TrimSpace(header)) {
		case "phone":
			hasPhone = true
		case "email":
			hasEmail = true
		}
	}

	switch {
	case !hasPhone && !hasEmail:
		return "", fmt.Errorf("csv file must contain at least one of the following columns [phone, email]")
	case hasPhone && hasEmail:
		return "", fmt.Errorf("csv file must contain either a phone or email column, not both")
	case hasPhone:
		return data.ReceiverContactTypeSMS, nil
	case hasEmail:
		return data.ReceiverContactTypeEmail, nil
	default:
		return "", fmt.Errorf("csv file must contain either a phone or email column")
	}
}
