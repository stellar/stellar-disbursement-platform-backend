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
	loginURL        = "login"
	disbursementURL = "disbursements"
	organizationURL = "organization"
	registrationURL = "sep24-interactive-deposit"
)

type ServerApiIntegrationTestsInterface interface {
	Login(ctx context.Context) (*ServerApiAuthToken, error)
	CreateDisbursement(ctx context.Context, authToken *ServerApiAuthToken, body *httphandler.PostDisbursementRequest) (*data.Disbursement, error)
	ProcessDisbursement(ctx context.Context, authToken *ServerApiAuthToken, disbursementID string) error
	StartDisbursement(ctx context.Context, authToken *ServerApiAuthToken, disbursementID string, body *httphandler.PatchDisbursementStatusRequest) error
	ReceiverRegistration(ctx context.Context, authSEP24Token *AnchorPlatformAuthSEP24Token, body *data.ReceiverRegistrationRequest) error
	ConfigureCircleAccess(ctx context.Context, authToken *ServerApiAuthToken, body *httphandler.PatchCircleConfigRequest) error
}

type ServerApiIntegrationTests struct {
	HttpClient              httpclient.HttpClientInterface
	ServerApiBaseURL        string
	TenantName              string
	UserEmail               string
	UserPassword            string
	DisbursementCSVFilePath string
	DisbursementCSVFileName string
}

type ServerApiAuthToken struct {
	Token string `json:"token"`
}

// Login login the integration test user on SDP server API.
func (sa *ServerApiIntegrationTests) Login(ctx context.Context) (*ServerApiAuthToken, error) {
	reqURL, err := url.JoinPath(sa.ServerApiBaseURL, loginURL)
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

	resp, err := sa.HttpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request to server API post LOGIN: %w", err)
	}

	if resp.StatusCode/100 != 2 {
		logErrorResponses(ctx, resp.Body)
		return nil, fmt.Errorf("error trying to login on the server API")
	}

	at := &ServerApiAuthToken{}
	err = json.NewDecoder(resp.Body).Decode(at)
	if err != nil {
		return nil, fmt.Errorf("error decoding response body: %w", err)
	}

	return at, nil
}

// CreateDisbursement creates a new disbursement using the SDP server API.
func (sa *ServerApiIntegrationTests) CreateDisbursement(ctx context.Context, authToken *ServerApiAuthToken, body *httphandler.PostDisbursementRequest) (*data.Disbursement, error) {
	reqURL, err := url.JoinPath(sa.ServerApiBaseURL, disbursementURL)
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

	resp, err := sa.HttpClient.Do(req)
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
func (sa *ServerApiIntegrationTests) ProcessDisbursement(ctx context.Context, authToken *ServerApiAuthToken, disbursementID string) error {
	reqURL, err := url.JoinPath(sa.ServerApiBaseURL, disbursementURL, disbursementID, "instructions")
	if err != nil {
		return fmt.Errorf("error creating url: %w", err)
	}

	req, err := createInstructionsRequest(ctx, sa.TenantName, reqURL, sa.DisbursementCSVFilePath, sa.DisbursementCSVFileName)
	if err != nil {
		return fmt.Errorf("error creating instructions request with multipart form-data: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+authToken.Token)
	req.Header.Set("SDP-Tenant-Name", sa.TenantName)

	resp, err := sa.HttpClient.Do(req)
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
func (sa *ServerApiIntegrationTests) StartDisbursement(ctx context.Context, authToken *ServerApiAuthToken, disbursementID string, body *httphandler.PatchDisbursementStatusRequest) error {
	reqURL, err := url.JoinPath(sa.ServerApiBaseURL, disbursementURL, disbursementID, "status")
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

	resp, err := sa.HttpClient.Do(req)
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
func (sa *ServerApiIntegrationTests) ReceiverRegistration(ctx context.Context, authSEP24Token *AnchorPlatformAuthSEP24Token, body *data.ReceiverRegistrationRequest) error {
	reqURL, err := url.JoinPath(sa.ServerApiBaseURL, registrationURL, "verification")
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

	resp, err := sa.HttpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error making request to server API post WALLET REGISTRATION VERIFICATION: %w", err)
	}

	if resp.StatusCode/100 != 2 {
		logErrorResponses(ctx, resp.Body)
		return fmt.Errorf("trying to complete receiver registration on the server API (statusCode=%d)", resp.StatusCode)
	}

	return nil
}

func (sa *ServerApiIntegrationTests) ConfigureCircleAccess(ctx context.Context, authToken *ServerApiAuthToken, body *httphandler.PatchCircleConfigRequest) error {
	reqURL, err := url.JoinPath(sa.ServerApiBaseURL, organizationURL, "circle-config")
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

	resp, err := sa.HttpClient.Do(req)
	if err != nil {
		return fmt.Errorf("making request to server API patch CIRCLE CONFIG: %w", err)
	}

	if resp.StatusCode/100 != 2 {
		logErrorResponses(ctx, resp.Body)
		return fmt.Errorf("statusCode %d when trying to configure Circle access on the server API", resp.StatusCode)
	}

	return nil
}

// Ensuring that ServerApiIntegrationTests is implementing ServerApiIntegrationTestsInterface.
var _ ServerApiIntegrationTestsInterface = (*ServerApiIntegrationTests)(nil)
