package integrationtests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"

	"github.com/stellar/go/keypair"
	"github.com/stellar/go/txnbuild"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpclient"
)

const (
	authURL         = "auth"
	sep24DepositURL = "sep24/transactions/deposit/interactive"
)

type AnchorPlatformIntegrationTestsInterface interface {
	StartChallengeTransaction() (*ChallengeTransaction, error)
	SignChallengeTransaction(challengeTx *ChallengeTransaction) (*SignedChallengeTransaction, error)
	SendSignedChallengeTransaction(signedChallengeTx *SignedChallengeTransaction) (*AnchorPlatformAuthToken, error)
	CreateSep24DepositTransaction(authToken *AnchorPlatformAuthToken) (*AnchorPlatformAuthSEP24Token, *AnchorPlatformDepositResponse, error)
}

type AnchorPlatformIntegrationTests struct {
	HttpClient                httpclient.HttpClientInterface
	AnchorPlatformBaseSepURL  string
	ReceiverAccountPublicKey  string
	ReceiverAccountPrivateKey string
	Sep10SigningPublicKey     string
	DisbursedAssetCode        string
}

type ChallengeTransaction struct {
	TransactionStr    string `json:"transaction"`
	NetworkPassphrase string `json:"network_passphrase"`
	Transaction       *txnbuild.Transaction
}

type SignedChallengeTransaction struct {
	*ChallengeTransaction
	SignedTransaction *txnbuild.Transaction
}

type AnchorPlatformAuthToken struct {
	Token string `json:"token"`
}

type AnchorPlatformDepositResponse struct {
	URL           string `json:"url"`
	TransactionID string `json:"id"`
}

type AnchorPlatformAuthSEP24Token struct {
	Token string `query:"token"`
}

// StartChallengeTransaction create a new challenge transaction through the anchor platform.
func (ap AnchorPlatformIntegrationTests) StartChallengeTransaction() (*ChallengeTransaction, error) {
	authURL, err := url.JoinPath(ap.AnchorPlatformBaseSepURL, authURL)
	if err != nil {
		return nil, fmt.Errorf("error creating url: %w", err)
	}

	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, authURL, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating new request: %w", err)
	}

	// hosts domain to be used in txnbuild.ReadChallengeTx
	homeDomain := "localhost:8080"
	webAuthDomain := "localhost:8080"

	// create query params 'account' and 'home_domain'
	q := req.URL.Query()
	q.Add("account", ap.ReceiverAccountPublicKey)
	q.Add("home_domain", homeDomain)
	req.URL.RawQuery = q.Encode()

	resp, err := ap.HttpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request to anchor platform get AUTH: %w", err)
	}

	if resp.StatusCode/100 != 2 {
		logErrorResponses(ctx, resp.Body)
		return nil, fmt.Errorf("error creating challenge transaction on anchor platform")
	}

	ct := &ChallengeTransaction{}
	err = json.NewDecoder(resp.Body).Decode(ct)
	if err != nil {
		return nil, fmt.Errorf("error decoding response body: %w", err)
	}

	// read the challenge transaction created by the anchor platform and assign it to the ChallengeTransaction object.
	tx, _, _, _, err := txnbuild.ReadChallengeTx(
		ct.TransactionStr,
		ap.Sep10SigningPublicKey,
		ct.NetworkPassphrase,
		webAuthDomain,
		[]string{homeDomain},
	)
	if err != nil {
		return nil, fmt.Errorf("error reading challenge transaction: %w", err)
	}
	ct.Transaction = tx

	return ct, nil
}

// SignChallengeTransaction signs a challenge transaction with the ReceiverAccountPrivateKey.
func (ap AnchorPlatformIntegrationTests) SignChallengeTransaction(challengeTx *ChallengeTransaction) (*SignedChallengeTransaction, error) {
	// get the receiver account keypair
	kp, err := keypair.ParseFull(ap.ReceiverAccountPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("error getting receiver keypair: %w", err)
	}

	// sign the challenge transaction with the receiver account keypair
	st := &SignedChallengeTransaction{ChallengeTransaction: challengeTx}
	signedTx, err := challengeTx.Transaction.Sign(challengeTx.NetworkPassphrase, kp)
	if err != nil {
		return nil, fmt.Errorf("error signing challenge transaction: %w", err)
	}

	// attributes signedTx to the SignedChallengeTransaction object
	st.SignedTransaction = signedTx

	return st, nil
}

// SendSignedChallengeTransaction sends the signed transaction to the anchor platform to get the authorization token.
func (ap AnchorPlatformIntegrationTests) SendSignedChallengeTransaction(signedChallengeTx *SignedChallengeTransaction) (*AnchorPlatformAuthToken, error) {
	authURL, err := url.JoinPath(ap.AnchorPlatformBaseSepURL, authURL)
	if err != nil {
		return nil, fmt.Errorf("error creating url: %w", err)
	}

	// get the transaction object in base 64 format
	txBase64, err := signedChallengeTx.SignedTransaction.Base64()
	if err != nil {
		return nil, fmt.Errorf("error converting signed transaction to base 64: %w", err)
	}
	// sets transaction base 64 in request body
	data := url.Values{}
	data.Set("transaction", txBase64)

	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, authURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("error creating new request: %w", err)
	}

	// POST auth endpoint on anchor platform expects the content-type to be x-www-form-urlencoded
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := ap.HttpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request to anchor platform post AUTH: %w", err)
	}

	if resp.StatusCode/100 != 2 {
		logErrorResponses(ctx, resp.Body)
		return nil, fmt.Errorf("error sending signed challenge transaction on anchor platform")
	}

	at := &AnchorPlatformAuthToken{}
	err = json.NewDecoder(resp.Body).Decode(at)
	if err != nil {
		return nil, fmt.Errorf("error decoding response body: %w", err)
	}

	return at, nil
}

// CreateSep24DepositTransaction creates a new sep24 deposit transaction on the anchor platform.
// To make this request, an auth token is required and it needs to be obtained through SEP-10.
func (ap AnchorPlatformIntegrationTests) CreateSep24DepositTransaction(authToken *AnchorPlatformAuthToken) (*AnchorPlatformAuthSEP24Token, *AnchorPlatformDepositResponse, error) {
	depositUrl, err := url.JoinPath(ap.AnchorPlatformBaseSepURL, sep24DepositURL)
	if err != nil {
		return nil, nil, fmt.Errorf("error creating url: %w", err)
	}

	// creates the multipart/form-data with the necessary fields to complete the request on the anchor platform
	b := &bytes.Buffer{}
	w := multipart.NewWriter(b)
	defer w.Close()
	formValues := map[string]string{
		"asset_code":                  ap.DisbursedAssetCode,
		"account":                     ap.ReceiverAccountPublicKey,
		"lang":                        "en",
		"claimable_balance_supported": "false",
	}
	for k, v := range formValues {
		err = w.WriteField(k, v)
		if err != nil {
			return nil, nil, fmt.Errorf("error writing %q field to form data: %w", k, err)
		}
	}
	// we need to close *multipart.Writter before pass as parameter in http.NewRequestWithContext
	w.Close()

	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, depositUrl, b)
	if err != nil {
		return nil, nil, fmt.Errorf("error creating new request: %w", err)
	}

	// POST sep24/transactions/deposit/interactive endpoint on anchor platform expects the content-type to be multipart/form-data
	req.Header.Set("Content-Type", w.FormDataContentType())
	// sets in the header the authorization token received in SendSignedChallengeTransaction
	req.Header.Set("Authorization", "Bearer "+authToken.Token)

	resp, err := ap.HttpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("error making request to anchor platform post SEP24 Deposit: %w", err)
	}

	if resp.StatusCode/100 != 2 {
		logErrorResponses(ctx, resp.Body)
		return nil, nil, fmt.Errorf("error creating sep24 deposit transaction on anchor platform")
	}

	dr := &AnchorPlatformDepositResponse{}
	err = json.NewDecoder(resp.Body).Decode(dr)
	if err != nil {
		return nil, nil, fmt.Errorf("error decoding response body: %w", err)
	}

	registerURL, err := url.Parse(dr.URL)
	if err != nil {
		return nil, nil, fmt.Errorf("error parsing url from AnchorPlatformDepositResponse: %w", err)
	}

	queryParams, err := url.ParseQuery(registerURL.RawQuery)
	if err != nil {
		return nil, nil, fmt.Errorf("error parsing query params from register url: %w", err)
	}

	if _, ok := queryParams["token"]; !ok {
		return nil, nil, fmt.Errorf("error register url not have a valid token")
	}

	at := &AnchorPlatformAuthSEP24Token{
		Token: queryParams.Get("token"),
	}

	return at, dr, nil
}

// Ensuring that AnchorPlatformIntegrationTests is implementing AnchorPlatformIntegrationTestsInterface.
var _ AnchorPlatformIntegrationTestsInterface = (*AnchorPlatformIntegrationTests)(nil)
