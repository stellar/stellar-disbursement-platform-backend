package integrationtests

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpclient"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

type AdminApiIntegrationTests struct {
	HttpClient      httpclient.HttpClientInterface
	AdminApiBaseURL string
	AccountId       string
	ApiKey          string
}

type AdminApiIntegrationTestsInterface interface {
	CreateTenant(ctx context.Context, body CreateTenantRequest) (*tenant.Tenant, error)
}

type CreateTenantRequest struct {
	Name                string `json:"name"`
	OwnerEmail          string `json:"owner_email"`
	OwnerFirstName      string `json:"owner_first_name"`
	OwnerLastName       string `json:"owner_last_name"`
	OrganizationName    string `json:"organization_name"`
	EmailSenderType     string `json:"email_sender_type"`
	SMSSenderType       string `json:"sms_sender_type"`
	BaseURL             string `json:"base_url"`
	SDPUIBaseURL        string `json:"sdp_ui_base_url"`
	DistributionAccount string `json:"distribution_account"`
}

func (aa AdminApiIntegrationTests) CreateTenant(ctx context.Context, body CreateTenantRequest) (*tenant.Tenant, error) {
	reqURL, err := url.JoinPath(aa.AdminApiBaseURL, "tenants")
	if err != nil {
		return nil, fmt.Errorf("building url to create tenant: %w", err)
	}

	reqBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshalling body for CreateTenantRequest: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("building request to create tenant: %w", err)
	}

	req.Header.Set("Authorization", aa.AuthHeader())
	req.Header.Set("Content-Type", "application/json")

	resp, err := aa.HttpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("making request to create tenant: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		var httpErr httperror.HTTPError
		if err = json.NewDecoder(resp.Body).Decode(&httpErr); err == nil {
			return nil, fmt.Errorf("unexpected status code when creating tenant: %d, error: %s", resp.StatusCode, httpErr.Message)
		}
		return nil, fmt.Errorf("unexpected status code when creating tenant: %d", resp.StatusCode)
	}

	var t tenant.Tenant
	if err = json.NewDecoder(resp.Body).Decode(&t); err != nil {
		return nil, fmt.Errorf("decoding response when creating tenant: %w", err)
	}

	return &t, nil
}

// AuthHeader returns the auth header using base64 encoding of the account id and api key
func (aa AdminApiIntegrationTests) AuthHeader() string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(aa.AccountId+":"+aa.ApiKey))
}
