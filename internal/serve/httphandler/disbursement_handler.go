package httphandler

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gocarina/gocsv"
	"github.com/stellar/go/support/log"
	"github.com/stellar/go/support/render/httpjson"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpresponse"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/middleware"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

type DisbursementHandler struct {
	Models           *data.Models
	MonitorService   monitor.MonitorServiceInterface
	DBConnectionPool db.DBConnectionPool
	AuthManager      auth.AuthManager
}

type PostDisbursementRequest struct {
	Name                           string                 `json:"name"`
	CountryCode                    string                 `json:"country_code"`
	WalletID                       string                 `json:"wallet_id"`
	AssetID                        string                 `json:"asset_id"`
	VerificationField              data.VerificationField `json:"verification_field"`
	SMSRegistrationMessageTemplate string                 `json:"sms_registration_message_template"`
}

type PatchDisbursementStatusRequest struct {
	Status string `json:"status"`
}

func (d DisbursementHandler) PostDisbursement(w http.ResponseWriter, r *http.Request) {
	var disbursementRequest PostDisbursementRequest

	err := json.NewDecoder(r.Body).Decode(&disbursementRequest)
	if err != nil {
		httperror.BadRequest("invalid request body", err, nil).Render(w)
		return
	}

	v := validators.NewDisbursementRequestValidator(disbursementRequest.VerificationField)
	v.Check(disbursementRequest.Name != "", "name", "name is required")
	v.Check(disbursementRequest.CountryCode != "", "country_code", "country_code is required")
	v.Check(disbursementRequest.WalletID != "", "wallet_id", "wallet_id is required")
	v.Check(disbursementRequest.AssetID != "", "asset_id", "asset_id is required")

	if v.HasErrors() {
		httperror.BadRequest("Request invalid", err, v.Errors).Render(w)
		return
	}

	verificationField := v.ValidateAndGetVerificationType()

	if v.HasErrors() {
		httperror.BadRequest("Verification field invalid", err, v.Errors).Render(w)
		return
	}

	ctx := r.Context()
	wallet, err := d.Models.Wallets.Get(ctx, disbursementRequest.WalletID)
	if err != nil {
		httperror.BadRequest("wallet ID is invalid", err, nil).Render(w)
		return
	}
	if !wallet.Enabled {
		httperror.BadRequest("wallet is not enabled", errors.New("wallet is not enabled"), nil).Render(w)
		return
	}
	asset, err := d.Models.Assets.Get(ctx, disbursementRequest.AssetID)
	if err != nil {
		httperror.BadRequest("asset ID is invalid", err, nil).Render(w)
		return
	}
	country, err := d.Models.Countries.Get(ctx, disbursementRequest.CountryCode)
	if err != nil {
		httperror.BadRequest("country code is invalid", err, nil).Render(w)
		return
	}

	token, ok := ctx.Value(middleware.TokenContextKey).(string)
	if !ok {
		msg := fmt.Sprintf("Cannot get token from context when inserting disbursement %s", disbursementRequest.Name)
		httperror.InternalError(ctx, msg, nil, nil).Render(w)
		return
	}
	user, err := d.AuthManager.GetUser(ctx, token)
	if err != nil {
		msg := fmt.Sprintf("Cannot insert disbursement %s", disbursementRequest.Name)
		httperror.InternalError(ctx, msg, err, nil).Render(w)
		return
	}

	disbursement := data.Disbursement{
		Name:   disbursementRequest.Name,
		Status: data.DraftDisbursementStatus,
		StatusHistory: []data.DisbursementStatusHistoryEntry{{
			Timestamp: time.Now(),
			Status:    data.DraftDisbursementStatus,
			UserID:    user.ID,
		}},
		Wallet:                         wallet,
		Asset:                          asset,
		Country:                        country,
		VerificationField:              verificationField,
		SMSRegistrationMessageTemplate: disbursementRequest.SMSRegistrationMessageTemplate,
	}

	newId, err := d.Models.Disbursements.Insert(ctx, &disbursement)
	if err != nil {
		if errors.Is(data.ErrRecordAlreadyExists, err) {
			httperror.Conflict("disbursement already exists", err, nil).Render(w)
		} else {
			httperror.BadRequest("could not create disbursement", err, nil).Render(w)
		}
		return
	}

	newDisbursement, err := d.Models.Disbursements.Get(ctx, d.DBConnectionPool, newId)
	if err != nil {
		msg := fmt.Sprintf("Cannot retrieve disbursement for ID: %s", newId)
		httperror.InternalError(ctx, msg, err, nil).Render(w)
		return
	}

	labels := monitor.DisbursementLabels{
		Asset:   newDisbursement.Asset.Code,
		Country: newDisbursement.Country.Code,
		Wallet:  newDisbursement.Wallet.Name,
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
	disbursementManagementService := services.NewDisbursementManagementService(d.Models, d.DBConnectionPool, d.AuthManager)
	resultWithTotal, err := disbursementManagementService.GetDisbursementsWithCount(ctx, queryParams)
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
	disbursement, err := d.Models.Disbursements.Get(ctx, d.DBConnectionPool, disbursementID)
	if err != nil {
		httperror.BadRequest("disbursement ID is invalid", err, nil).Render(w)
		return
	}

	// check if disbursement is in draft, ready status
	if disbursement.Status != data.DraftDisbursementStatus && disbursement.Status != data.ReadyDisbursementStatus {
		httperror.BadRequest("disbursement is not in draft or ready status", nil, nil).Render(w)
		return
	}

	// Parse uploaded CSV file
	file, header, err := r.FormFile("file")
	if err != nil {
		httperror.BadRequest("could not parse file", err, nil).Render(w)
		return
	}
	defer file.Close()

	// TeeReader is used to read multiple times from the same reader (file)
	// We read once to process the instructions, and then again to persist the file to the database
	var buf bytes.Buffer
	reader := io.TeeReader(file, &buf)

	instructions, v := parseInstructionsFromCSV(reader, disbursement.VerificationField)
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

	if err = d.Models.DisbursementInstructions.ProcessAll(ctx, user.ID, instructions, disbursement, disbursementUpdate, data.MaxInstructionsPerDisbursement); err != nil {
		switch {
		case errors.Is(err, data.ErrMaxInstructionsExceeded):
			httperror.BadRequest(fmt.Sprintf("number of instructions exceeds maximum of : %d", data.MaxInstructionsPerDisbursement), err, nil).Render(w)
		case errors.Is(err, data.ErrReceiverVerificationMismatch):
			httperror.BadRequest(errors.Unwrap(err).Error(), err, nil).Render(w)
		default:
			httperror.InternalError(ctx, fmt.Sprintf("Cannot process instructions for disbursement with ID: %s", disbursementID), err, nil).Render(w)
		}
		return
	}

	response := map[string]string{
		"message": "File uploaded successfully",
	}

	httpjson.Render(w, response, httpjson.JSON)
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

	disbursementManagementService := services.NewDisbursementManagementService(d.Models, d.DBConnectionPool, d.AuthManager)
	response, err := disbursementManagementService.AppendUserMetadata(ctx, []*data.Disbursement{disbursement})
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

	disbursementManagementService := services.NewDisbursementManagementService(d.Models, d.DBConnectionPool, d.AuthManager)
	resultWithTotal, err := disbursementManagementService.GetDisbursementReceiversWithCount(ctx, disbursementID, queryParams)
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

	disbursementManagementService := services.NewDisbursementManagementService(d.Models, d.DBConnectionPool, d.AuthManager)
	response := UpdateDisbursementStatusResponseBody{}

	ctx := r.Context()
	disbursementID := chi.URLParam(r, "id")
	switch toStatus {
	case data.StartedDisbursementStatus:
		err = disbursementManagementService.StartDisbursement(ctx, disbursementID)
		response.Message = "Disbursement started"
	case data.PausedDisbursementStatus:
		err = disbursementManagementService.PauseDisbursement(ctx, disbursementID)
		response.Message = "Disbursement paused"
	default:
		err = services.ErrDisbursementStatusCantBeChanged
	}

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
		default:
			msg := fmt.Sprintf("Cannot update disbursement ID %s with status: %s", disbursementID, toStatus)
			httperror.InternalError(ctx, msg, err, nil).Render(w)
		}
		return
	}

	httpjson.RenderStatus(w, http.StatusOK, response, httpjson.JSON)
}

func (d DisbursementHandler) GetDisbursementInstructions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	disbursementID := chi.URLParam(r, "id")

	disbursement, err := d.Models.Disbursements.Get(ctx, d.DBConnectionPool, disbursementID)
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

	// `attachment` returns a file-download prompt. change that to `inline` to open in browser
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, disbursement.FileName))
	w.Header().Set("Content-Type", "text/csv")
	_, err = w.Write(disbursement.FileContent)
	if err != nil {
		httperror.InternalError(ctx, "Cannot write disbursement instructions to response", err, nil).Render(w)
	}
}

func parseInstructionsFromCSV(file io.Reader, verificationField data.VerificationField) ([]*data.DisbursementInstruction, *validators.DisbursementInstructionsValidator) {
	validator := validators.NewDisbursementInstructionsValidator(verificationField)

	instructions := []*data.DisbursementInstruction{}
	if err := gocsv.Unmarshal(file, &instructions); err != nil {
		log.Errorf("error parsing csv file: %s", err.Error())
		validator.Errors["file"] = "could not parse file"
		return nil, validator
	}

	for i, instruction := range instructions {
		lineNumber := i + 2 // +1 for header row, +1 for 0-index
		validator.ValidateInstruction(instruction, lineNumber)
	}

	validator.Check(len(instructions) > 0, "instructions", "no valid instructions found")

	if validator.HasErrors() {
		return nil, validator
	}

	return instructions, nil
}
