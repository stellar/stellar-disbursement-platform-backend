package integrationtests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"mime/multipart"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpclient"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httphandler"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

const (
	loginURL           = "login"
	disbursementURL    = "disbursements"
	organizationURL    = "organization"
	registrationURL    = "sep24-interactive-deposit"
	embeddedWalletsURL = "embedded-wallets"
)

type ServerAPIIntegrationTestsInterface interface {
	Login(ctx context.Context) (*ServerAPIAuthToken, error)
	CreateDisbursement(ctx context.Context, authToken *ServerAPIAuthToken, body *httphandler.PostDisbursementRequest) (*data.Disbursement, error)
	ProcessDisbursement(ctx context.Context, authToken *ServerAPIAuthToken, disbursementID string) error
	StartDisbursement(ctx context.Context, authToken *ServerAPIAuthToken, disbursementID string, body *httphandler.PatchDisbursementStatusRequest) error
	ReceiverRegistration(ctx context.Context, authSEP24Token *SEP24AuthToken, body *data.ReceiverRegistrationRequest) error
	ConfigureCircleAccess(ctx context.Context, authToken *ServerAPIAuthToken, body *httphandler.PatchCircleConfigRequest) error
	CreateEmbeddedWallet(ctx context.Context, req *httphandler.CreateWalletRequest) (*httphandler.WalletResponse, error)
}

type ServerAPIIntegrationTests struct {
	HTTPClient              httpclient.HTTPClientInterface
	ServerAPIBaseURL        string
	TenantName              string
	UserEmail               string
	UserPassword            string
	DisbursementCSVFilePath string
	DisbursementCSVFileName string
}

type ServerAPIAuthToken struct {
	Token string `json:"token"`
}

type SEP24AuthToken struct {
	Token string `json:"token"`
}

// Login login the integration test user on SDP server API.
func (sa *ServerAPIIntegrationTests) Login(ctx context.Context) (*ServerAPIAuthToken, error) {
	reqURL, err := url.JoinPath(sa.ServerAPIBaseURL, loginURL)
	if err != nil {
		return nil, fmt.Errorf("error creating url: %w", err)
	}

	reqBody, err := json.Marshal(&httphandler.LoginRequest{
		Email:    sa.UserEmail,
		Password: sa.UserPassword,
	})
	if err != nil {
		return nil, fmt.Errorf("error creating json post body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, strings.NewReader(string(reqBody)))
	if err != nil {
		return nil, fmt.Errorf("error creating new request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("SDP-Tenant-Name", sa.TenantName)

	resp, err := sa.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request to server API post LOGIN: %w", err)
	}

	if resp.StatusCode/100 != 2 {
		logErrorResponses(ctx, resp.Body)
		return nil, fmt.Errorf("error trying to login on the server API")
	}

	at := &ServerAPIAuthToken{}
	err = json.NewDecoder(resp.Body).Decode(at)
	if err != nil {
		return nil, fmt.Errorf("error decoding response body: %w", err)
	}

	return at, nil
}

// CreateDisbursement creates a new disbursement using the SDP server API.
func (sa *ServerAPIIntegrationTests) CreateDisbursement(ctx context.Context, authToken *ServerAPIAuthToken, body *httphandler.PostDisbursementRequest) (*data.Disbursement, error) {
	reqURL, err := url.JoinPath(sa.ServerAPIBaseURL, disbursementURL)
	if err != nil {
		return nil, fmt.Errorf("error creating url: %w", err)
	}

	reqBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("error creating json post body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, strings.NewReader(string(reqBody)))
	if err != nil {
		return nil, fmt.Errorf("error creating new request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+authToken.Token)
	req.Header.Set("SDP-Tenant-Name", sa.TenantName)

	resp, err := sa.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request to server API post DISBURSEMENT: %w", err)
	}

	if resp.StatusCode/100 != 2 {
		logErrorResponses(ctx, resp.Body)
		return nil, fmt.Errorf("trying to create a new disbursement on the server API")
	}

	disbursement := &data.Disbursement{}
	err = json.NewDecoder(resp.Body).Decode(disbursement)
	if err != nil {
		return nil, fmt.Errorf("error decoding response body: %w", err)
	}

	return disbursement, nil
}

// createInstructionsRequest creates the request with multipart formdata to process disbursement on SDP server API.
func createInstructionsRequest(ctx context.Context, tenantName, reqURL, disbursementCSVFilePath, disbursementCSVFileName string) (*http.Request, error) {
	filePath := path.Join(disbursementCSVFilePath, disbursementCSVFileName)

	csvBytes, err := fs.ReadFile(DisbursementCSVFiles, filePath)
	if err != nil {
		return nil, fmt.Errorf("reading csv file: %w", err)
	}

	b := &bytes.Buffer{}
	w := multipart.NewWriter(b)
	defer utils.DeferredClose(ctx, w, "closing multipart writer")

	fileWriter, err := w.CreateFormFile("file", disbursementCSVFileName)
	if err != nil {
		return nil, fmt.Errorf("creating form file with disbursement csv file: %w", err)
	}

	_, err = io.Copy(fileWriter, bytes.NewReader(csvBytes))
	if err != nil {
		return nil, fmt.Errorf("copying file: %w", err)
	}
	// we need to close *multipart.Writter before pass as parameter in http.NewRequestWithContext
	err = w.Close()
	if err != nil {
		return nil, fmt.Errorf("closing multipart writer: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, b)
	if err != nil {
		return nil, fmt.Errorf("creating new request: %w", err)
	}

	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("SDP-Tenant-Name", tenantName)

	return req, nil
}

// ProcessDisbursement process the disbursement using the SDP server API.
func (sa *ServerAPIIntegrationTests) ProcessDisbursement(ctx context.Context, authToken *ServerAPIAuthToken, disbursementID string) error {
	reqURL, err := url.JoinPath(sa.ServerAPIBaseURL, disbursementURL, disbursementID, "instructions")
	if err != nil {
		return fmt.Errorf("error creating url: %w", err)
	}

	req, err := createInstructionsRequest(ctx, sa.TenantName, reqURL, sa.DisbursementCSVFilePath, sa.DisbursementCSVFileName)
	if err != nil {
		return fmt.Errorf("error creating instructions request with multipart form-data: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+authToken.Token)
	req.Header.Set("SDP-Tenant-Name", sa.TenantName)

	resp, err := sa.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("error making request to server API post DISBURSEMENT INSTRUCTIONS: %w", err)
	}

	if resp.StatusCode/100 != 2 {
		logErrorResponses(ctx, resp.Body)
		return fmt.Errorf("error trying to process the disbursement CSV file on the server API")
	}

	return nil
}

// StartDisbursement starts the disbursement using the SDP server API.
func (sa *ServerAPIIntegrationTests) StartDisbursement(ctx context.Context, authToken *ServerAPIAuthToken, disbursementID string, body *httphandler.PatchDisbursementStatusRequest) error {
	reqURL, err := url.JoinPath(sa.ServerAPIBaseURL, disbursementURL, disbursementID, "status")
	if err != nil {
		return fmt.Errorf("error creating url: %w", err)
	}

	reqBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("error creating json post body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, reqURL, strings.NewReader(string(reqBody)))
	if err != nil {
		return fmt.Errorf("error creating new request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+authToken.Token)
	req.Header.Set("SDP-Tenant-Name", sa.TenantName)

	resp, err := sa.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("error making request to server API patch DISBURSEMENT: %w", err)
	}

	if resp.StatusCode/100 != 2 {
		logErrorResponses(ctx, resp.Body)
		return fmt.Errorf("error trying to start the disbursement on the server API (statusCode=%d)", resp.StatusCode)
	}

	return nil
}

// ReceiverRegistration completes the receiver registration using SDP server API and the anchor platform.
func (sa *ServerAPIIntegrationTests) ReceiverRegistration(ctx context.Context, authSEP24Token *SEP24AuthToken, body *data.ReceiverRegistrationRequest) error {
	reqURL, err := url.JoinPath(sa.ServerAPIBaseURL, registrationURL, "verification")
	if err != nil {
		return fmt.Errorf("error creating url: %w", err)
	}

	reqBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("error creating json post body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, strings.NewReader(string(reqBody)))
	if err != nil {
		return fmt.Errorf("error creating new request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+authSEP24Token.Token)
	req.Header.Set("SDP-Tenant-Name", sa.TenantName)

	resp, err := sa.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("error making request to server API post WALLET REGISTRATION VERIFICATION: %w", err)
	}

	if resp.StatusCode/100 != 2 {
		logErrorResponses(ctx, resp.Body)
		return fmt.Errorf("trying to complete receiver registration on the server API (statusCode=%d)", resp.StatusCode)
	}

	return nil
}

func (sa *ServerAPIIntegrationTests) ConfigureCircleAccess(ctx context.Context, authToken *ServerAPIAuthToken, body *httphandler.PatchCircleConfigRequest) error {
	reqURL, err := url.JoinPath(sa.ServerAPIBaseURL, organizationURL, "circle-config")
	if err != nil {
		return fmt.Errorf("creating url: %w", err)
	}

	reqBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("creating json post body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, reqURL, strings.NewReader(string(reqBody)))
	if err != nil {
		return fmt.Errorf("creating new request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+authToken.Token)
	req.Header.Set("SDP-Tenant-Name", sa.TenantName)

	resp, err := sa.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("making request to server API patch CIRCLE CONFIG: %w", err)
	}

	if resp.StatusCode/100 != 2 {
		logErrorResponses(ctx, resp.Body)
		return fmt.Errorf("statusCode %d when trying to configure Circle access on the server API", resp.StatusCode)
	}

	return nil
}

// CreateEmbeddedWallet creates a new embedded wallet using POST /embedded-wallets.
func (sa *ServerAPIIntegrationTests) CreateEmbeddedWallet(ctx context.Context, req *httphandler.CreateWalletRequest) (*httphandler.WalletResponse, error) {
	reqURL, err := url.JoinPath(sa.ServerAPIBaseURL, embeddedWalletsURL)
	if err != nil {
		return nil, fmt.Errorf("creating url: %w", err)
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("creating json post body: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, strings.NewReader(string(reqBody)))
	if err != nil {
		return nil, fmt.Errorf("creating new request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("SDP-Tenant-Name", sa.TenantName)

	resp, err := sa.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("making request to POST /embedded-wallets: %w", err)
	}

	if resp.StatusCode/100 != 2 {
		logErrorResponses(ctx, resp.Body)
		return nil, fmt.Errorf("error registering embedded wallet (statusCode=%d)", resp.StatusCode)
	}

	walletResp := &httphandler.WalletResponse{}
	if err = json.NewDecoder(resp.Body).Decode(walletResp); err != nil {
		return nil, fmt.Errorf("decoding response body: %w", err)
	}

	return walletResp, nil
}

// Ensuring that ServerAPIIntegrationTests is implementing ServerAPIIntegrationTestsInterface.
var _ ServerAPIIntegrationTestsInterface = (*ServerAPIIntegrationTests)(nil)
