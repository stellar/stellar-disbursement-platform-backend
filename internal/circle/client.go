package circle

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpclient"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

const (
	pingPath             = "/ping"
	transferPath         = "/v1/transfers"
	payoutPath           = "/v1/payouts"
	businessBalancesPath = "/v1/businessAccount/balances"
	configurationPath    = "/v1/configuration"
	addressRecipientPath = "/v1/addressBook/recipients"
)

var authErrorStatusCodes = []int{http.StatusUnauthorized, http.StatusForbidden}

// ClientInterface defines the interface for interacting with the Circle API.
//
//go:generate mockery --name=ClientInterface --case=underscore --structname=MockClient --filename=client_mock.go --inpackage
type ClientInterface interface {
	Ping(ctx context.Context) (bool, error)
	PostTransfer(ctx context.Context, transferRequest TransferRequest) (*Transfer, error) // TODO: remove this method in https://stellarorg.atlassian.net/browse/SDP-1448
	GetTransferByID(ctx context.Context, id string) (*Transfer, error)                    // TODO: remove this method in https://stellarorg.atlassian.net/browse/SDP-1448
	PostRecipient(ctx context.Context, recipientRequest RecipientRequest) (*Recipient, error)
	GetRecipientByID(ctx context.Context, id string) (*Recipient, error)
	PostPayout(ctx context.Context, payoutRequest PayoutRequest) (*Payout, error)
	GetPayoutByID(ctx context.Context, id string) (*Payout, error)
	GetBusinessBalances(ctx context.Context) (*Balances, error)
	GetAccountConfiguration(ctx context.Context) (*AccountConfiguration, error)
}

// Client provides methods to interact with the Circle API.
type Client struct {
	BasePath       string
	APIKey         string
	httpClient     httpclient.HttpClientInterface
	tenantManager  tenant.ManagerInterface
	monitorService monitor.MonitorServiceInterface
}

// ClientFactory is a function that creates a ClientInterface.
type ClientFactory func(opts ClientOptions) ClientInterface

var _ ClientFactory = NewClient

type ClientOptions struct {
	NetworkType    utils.NetworkType
	APIKey         string
	TenantManager  tenant.ManagerInterface
	MonitorService monitor.MonitorServiceInterface
}

// NewClient creates a new instance of Circle Client.
func NewClient(opts ClientOptions) ClientInterface {
	circleEnv := Sandbox
	if opts.NetworkType == utils.PubnetNetworkType {
		circleEnv = Production
	}

	return &Client{
		BasePath:       string(circleEnv),
		APIKey:         opts.APIKey,
		httpClient:     httpclient.DefaultClient(),
		tenantManager:  opts.TenantManager,
		monitorService: opts.MonitorService,
	}
}

// Ping checks that the service is running.
//
// Circle API documentation: https://developers.circle.com/circle-mint/reference/ping.
func (client *Client) Ping(ctx context.Context) (bool, error) {
	u, err := url.JoinPath(client.BasePath, pingPath)
	if err != nil {
		return false, fmt.Errorf("building path: %w", err)
	}

	resp, err := client.request(ctx, pingPath, u, http.MethodGet, false, nil)
	if err != nil {
		return false, fmt.Errorf("making request: %w", err)
	}
	defer utils.DeferredClose(ctx, resp.Body, "closing response body")

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return true, nil
}

// PostTransfer creates a new transfer.
//
// Circle API documentation: https://developers.circle.com/circle-mint/reference/createtransfer.
func (client *Client) PostTransfer(ctx context.Context, transferReq TransferRequest) (*Transfer, error) {
	err := transferReq.validate()
	if err != nil {
		return nil, fmt.Errorf("validating transfer request: %w", err)
	}

	u, err := url.JoinPath(client.BasePath, transferPath)
	if err != nil {
		return nil, fmt.Errorf("building path: %w", err)
	}

	transferData, err := json.Marshal(transferReq)
	if err != nil {
		return nil, fmt.Errorf("marshalling transfer request: %w", err)
	}

	resp, err := client.request(ctx, transferPath, u, http.MethodPost, true, transferData)
	if err != nil {
		return nil, fmt.Errorf("making request: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		handleErr := client.handleError(ctx, resp)
		if handleErr != nil {
			return nil, fmt.Errorf("handling API response error: %w", handleErr)
		}
	}

	return parseTransferResponse(resp)
}

// GetTransferByID retrieves a transfer by its ID.
//
// Circle API documentation: https://developers.circle.com/circle-mint/reference/gettransfer.
func (client *Client) GetTransferByID(ctx context.Context, id string) (*Transfer, error) {
	u, err := url.JoinPath(client.BasePath, transferPath, id)
	if err != nil {
		return nil, fmt.Errorf("building path: %w", err)
	}

	resp, err := client.request(ctx, transferPath, u, http.MethodGet, true, nil)
	if err != nil {
		return nil, fmt.Errorf("making request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		handleErr := client.handleError(ctx, resp)
		if handleErr != nil {
			return nil, fmt.Errorf("handling API response error: %w", handleErr)
		}
	}

	return parseTransferResponse(resp)
}

// PostRecipient registers a new recipient in Circle's address book. This is needed in order to send a payout to that
// recipient.
//
// Circle API documentation:
// https://developers.circle.com/api-reference/circle-mint/payouts/create-address-book-recipient.
func (client *Client) PostRecipient(ctx context.Context, recipientRequest RecipientRequest) (*Recipient, error) {
	err := recipientRequest.validate()
	if err != nil {
		return nil, fmt.Errorf("validating recipient request: %w", err)
	}

	u, err := url.JoinPath(client.BasePath, addressRecipientPath)
	if err != nil {
		return nil, fmt.Errorf("building path: %w", err)
	}

	recipientData, err := json.Marshal(recipientRequest)
	if err != nil {
		return nil, fmt.Errorf("marshalling recipient request: %w", err)
	}

	resp, err := client.request(ctx, addressRecipientPath, u, http.MethodPost, true, recipientData)
	if err != nil {
		return nil, fmt.Errorf("making request: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		handleErr := client.handleError(ctx, resp)
		if handleErr != nil {
			return nil, fmt.Errorf("handling API response error: %w", handleErr)
		}
	}

	return parseRecipientResponse(resp)
}

// GetRecipientByID retrieves a recipient by its ID.
//
// Circle API documentation: https://developers.circle.com/api-reference/circle-mint/payouts/get-address-book-recipient.
func (client *Client) GetRecipientByID(ctx context.Context, id string) (*Recipient, error) {
	u, err := url.JoinPath(client.BasePath, addressRecipientPath, id)
	if err != nil {
		return nil, fmt.Errorf("building path: %w", err)
	}

	resp, err := client.request(ctx, addressRecipientPath, u, http.MethodGet, true, nil)
	if err != nil {
		return nil, fmt.Errorf("making request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		handleErr := client.handleError(ctx, resp)
		if handleErr != nil {
			return nil, fmt.Errorf("handling API response error: %w", handleErr)
		}
	}

	return parseRecipientResponse(resp)
}

// PostPayout creates a new payout to a recipient.
//
// Circle API documentation:
// https://developers.circle.com/api-reference/circle-mint/payouts/create-payout.
func (client *Client) PostPayout(ctx context.Context, payoutRequest PayoutRequest) (*Payout, error) {
	err := payoutRequest.validate()
	if err != nil {
		return nil, fmt.Errorf("validating payout request: %w", err)
	}

	u, err := url.JoinPath(client.BasePath, payoutPath)
	if err != nil {
		return nil, fmt.Errorf("building path: %w", err)
	}

	payload, err := json.Marshal(payoutRequest)
	if err != nil {
		return nil, fmt.Errorf("marshalling request: %w", err)
	}

	resp, err := client.request(ctx, payoutPath, u, http.MethodPost, true, payload)
	if err != nil {
		return nil, fmt.Errorf("making request: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		handleErr := client.handleError(ctx, resp)
		if handleErr != nil {
			return nil, fmt.Errorf("handling API response error: %w", handleErr)
		}
	}

	return parsePayoutResponse(resp)
}

// DestinationAddressErrorCodes are the error codes that indicate an issue with the destination address. If they show up
// when sending a payout, the Circle recipient will likely become unusable and will need to be recreated.
//
// Circle API documentation: https://developers.circle.com/circle-mint/circle-apis-api-errors.
var DestinationAddressErrorCodes = []int{5003, 5004, 5011}

// GetPayoutByID retrieves a payout by its ID.
//
// Circle API documentation: https://developers.circle.com/api-reference/circle-mint/payouts/get-payout.
func (client *Client) GetPayoutByID(ctx context.Context, id string) (*Payout, error) {
	u, err := url.JoinPath(client.BasePath, payoutPath, id)
	if err != nil {
		return nil, fmt.Errorf("building path: %w", err)
	}

	resp, err := client.request(ctx, payoutPath, u, http.MethodGet, true, nil)
	if err != nil {
		return nil, fmt.Errorf("making request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		handleErr := client.handleError(ctx, resp)
		if handleErr != nil {
			return nil, fmt.Errorf("handling API response error: %w", handleErr)
		}
	}

	return parsePayoutResponse(resp)
}

// GetBusinessBalances retrieves the available and unsettled balances for different currencies.
func (client *Client) GetBusinessBalances(ctx context.Context) (*Balances, error) {
	url, err := url.JoinPath(client.BasePath, businessBalancesPath)
	if err != nil {
		return nil, fmt.Errorf("building path: %w", err)
	}

	resp, err := client.request(ctx, businessBalancesPath, url, http.MethodGet, true, nil)
	if err != nil {
		return nil, fmt.Errorf("making request: %w", err)
	}
	defer utils.DeferredClose(ctx, resp.Body, "closing response body")

	if resp.StatusCode != http.StatusOK {
		handleErr := client.handleError(ctx, resp)
		if handleErr != nil {
			return nil, fmt.Errorf("handling API response error: %w", handleErr)
		}
	}

	return parseBusinessBalancesResponse(resp)
}

// GetAccountConfiguration retrieves the configuration of the Circle Account.
func (client *Client) GetAccountConfiguration(ctx context.Context) (*AccountConfiguration, error) {
	url, err := url.JoinPath(client.BasePath, configurationPath)
	if err != nil {
		return nil, fmt.Errorf("building path: %w", err)
	}

	resp, err := client.request(ctx, configurationPath, url, http.MethodGet, true, nil)
	if err != nil {
		return nil, fmt.Errorf("making request: %w", err)
	}
	defer utils.DeferredClose(ctx, resp.Body, "closing response body")

	if resp.StatusCode != http.StatusOK {
		handleErr := client.handleError(ctx, resp)
		if handleErr != nil {
			return nil, fmt.Errorf("handling API response error: %w", handleErr)
		}
	}

	return parseAccountConfigurationResponse(resp)
}

type RetryableError struct {
	err        error
	retryAfter time.Duration
}

func (re RetryableError) Error() string {
	retryableErr := fmt.Errorf("retryable error: %w", re.err)
	return retryableErr.Error()
}

// request makes an HTTP request to the Circle API.
func (client *Client) request(ctx context.Context, path, u, method string, isAuthed bool, bodyBytes []byte) (*http.Response, error) {
	var resp *http.Response
	err := retry.Do(
		func() error {
			startTime := time.Now()
			bodyReader := bytes.NewReader(bodyBytes)
			req, err := http.NewRequestWithContext(ctx, method, u, bodyReader)
			if err != nil {
				return fmt.Errorf("creating request: %w", err)
			}

			if isAuthed {
				req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", client.APIKey))
			}

			if bodyReader != nil {
				req.Header.Set("Content-Type", "application/json")
			}

			resp, err = client.httpClient.Do(req)
			client.recordCircleAPIMetrics(ctx, method, path, startTime, resp, err)

			if err != nil {
				return fmt.Errorf("submitting request to %s: %w", u, err)
			}

			if resp.StatusCode == http.StatusTooManyRequests {
				retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
				log.Ctx(ctx).Warnf("CircleClient - Request to %s is rate limited, retry after: %s", u, retryAfter)
				return RetryableError{
					err:        fmt.Errorf("rate limited, retry after: %s", retryAfter),
					retryAfter: retryAfter,
				}
			}
			return nil
		},
		retry.DelayType(func(n uint, err error, config *retry.Config) time.Duration {
			// if err is RetryableError, return retryAfter
			var retryableErr RetryableError
			ok := errors.As(err, &retryableErr)
			if ok {
				return retryableErr.retryAfter
			}
			// default is back-off delay
			return retry.BackOffDelay(n, err, config)
		}),
		retry.Context(ctx), // Respect the context's cancellation
		retry.Attempts(4),
		retry.MaxDelay(time.Second*600),
		retry.RetryIf(func(err error) bool {
			return errors.As(err, &RetryableError{})
		}),
		retry.OnRetry(func(n uint, err error) {
			log.Ctx(ctx).Warnf("🔄 CircleClient - Request to %s is rate limited, attempt %d failed with error: %v", u, n+1, err)
		}),
		retry.LastErrorOnly(true),
	)
	if err != nil {
		return nil, fmt.Errorf("unsuccessful after multiple attempts: %w", err)
	}

	return resp, nil
}

func parseRetryAfter(retryAfter string) time.Duration {
	if retryAfter == "" {
		return 0
	}
	seconds, err := strconv.Atoi(retryAfter)
	if err != nil {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

func (client *Client) recordCircleAPIMetrics(ctx context.Context, method, endpoint string, startTime time.Time, resp *http.Response, reqErr error) {
	t, err := sdpcontext.GetTenantFromContext(ctx)
	if err != nil {
		log.Ctx(ctx).Errorf("getting tenant from context: %v", err)
		return
	}

	duration := time.Since(startTime)
	status, statusCode := monitor.ParseHTTPResponseStatus(resp, reqErr)

	labels := monitor.CircleLabels{
		Method:     method,
		Endpoint:   endpoint,
		Status:     status,
		StatusCode: statusCode,
		TenantName: t.Name,
	}.ToMap()

	if err = client.monitorService.MonitorHistogram(duration.Seconds(), monitor.CircleAPIRequestDurationTag, labels); err != nil {
		log.Ctx(ctx).Errorf("monitoring histogram: %v", err)
	}

	if err = client.monitorService.MonitorCounters(monitor.CircleAPIRequestsTotalTag, labels); err != nil {
		log.Ctx(ctx).Errorf("monitoring counter: %v", err)
	}
}

func (client *Client) handleError(ctx context.Context, resp *http.Response) error {
	if slices.Contains(authErrorStatusCodes, resp.StatusCode) {
		tnt, getCtxTntErr := sdpcontext.GetTenantFromContext(ctx)
		if getCtxTntErr != nil {
			return fmt.Errorf("getting tenant from context: %w", getCtxTntErr)
		}

		deactivateTntErr := client.tenantManager.DeactivateTenantDistributionAccount(ctx, tnt.ID)
		if deactivateTntErr != nil {
			return fmt.Errorf("deactivating tenant distribution account: %w", deactivateTntErr)
		}
	}

	apiError, err := parseAPIError(resp)
	if err != nil {
		return fmt.Errorf("parsing API error: %w", err)
	}

	return fmt.Errorf("circle API error: %w", apiError)
}

var _ ClientInterface = (*Client)(nil)
