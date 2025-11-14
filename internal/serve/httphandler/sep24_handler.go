package httphandler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/stellar/go/strkey"
	"github.com/stellar/go/support/log"
	"github.com/stellar/go/support/render/httpjson"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sepauth"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
)

const (
	SEP24StatusIncomplete            = "incomplete"
	SEP24StatusPendingUserInfoUpdate = "pending_user_info_update"
	SEP24StatusCompleted             = "completed"
	SEP24StatusError                 = "error"
)

const (
	SEP24OperationKindDeposit = "deposit"
)

const (
	sep24MinAmount = 1
	sep24MaxAmount = 10000
)

type SEP24InfoResponse struct {
	Deposit  map[string]SEP24OperationResponse `json:"deposit"`
	Withdraw map[string]SEP24OperationResponse `json:"withdraw"`
	Fee      SEP24FeeResponse                  `json:"fee"`
	Features SEP24FeatureFlagResponse          `json:"features"`
}

type SEP24OperationResponse struct {
	Enabled   bool `json:"enabled"`
	MinAmount int  `json:"min_amount"`
	MaxAmount int  `json:"max_amount"`
}

type SEP24FeeResponse struct {
	Enabled bool `json:"enabled"`
}

type SEP24FeatureFlagResponse struct {
	AccountCreation   bool `json:"account_creation"`
	ClaimableBalances bool `json:"claimable_balances"`
}

type SEP24Handler struct {
	Models             *data.Models
	SEP24JWTManager    *sepauth.JWTManager
	InteractiveBaseURL string
}

type SEP24TransactionResponse struct {
	Transaction SEP24Transaction `json:"transaction"`
}

type SEP24Transaction struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Status string `json:"status"`
}

type SEP24DepositRequest struct {
	AssetCode                 string `json:"asset_code" schema:"asset_code"`
	Account                   string `json:"account" schema:"account"`
	Lang                      string `json:"lang" schema:"lang"`
	ClaimableBalanceSupported string `json:"claimable_balance_supported" schema:"claimable_balance_supported"`
}

type SEP24InteractiveResponse struct {
	Type          string `json:"type"`
	URL           string `json:"url"`
	TransactionID string `json:"id"`
}

// generateMoreInfoURL creates a secure more_info_url with JWT token for SEP-24 transactions
func (h SEP24Handler) generateMoreInfoURL(sep10Claims *sepauth.Sep10JWTClaims, transactionID, status string) (string, error) {
	sep24Token, err := h.SEP24JWTManager.GenerateSEP24MoreInfoToken(
		sep10Claims.Subject,
		"",
		sep10Claims.ClientDomain,
		sep10Claims.HomeDomain,
		transactionID,
		"en",
		map[string]string{
			"kind":   SEP24OperationKindDeposit,
			"status": status,
		},
	)
	if err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}

	baseURLParsed, err := url.Parse(h.InteractiveBaseURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse base URL: %w", err)
	}

	tenantBaseURL := fmt.Sprintf("%s://%s", baseURLParsed.Scheme, sep10Claims.HomeDomain)
	return fmt.Sprintf("%s/wallet-registration/start?transaction_id=%s&token=%s",
		tenantBaseURL, transactionID, sep24Token), nil
}

func (h SEP24Handler) GetTransaction(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	sep10Claims := sepauth.GetSEP10Claims(ctx)
	if sep10Claims == nil {
		httperror.Unauthorized("Missing or invalid authorization header", nil, nil).Render(w)
		return
	}

	// Only one of the following parameters should be present:
	//   - r.URL.Query().Get("external_transaction_id")
	//   - r.URL.Query().Get("stellar_transaction_id")
	//   - r.URL.Query().Get("id")
	// If more than one is present, return a 400 error
	if r.URL.Query().Get("external_transaction_id") != "" && r.URL.Query().Get("stellar_transaction_id") != "" && r.URL.Query().Get("id") != "" {
		httperror.BadRequest("Only one of the following parameters should be present: external_transaction_id, stellar_transaction_id, or id", nil, nil).Render(w)
		return
	}

	if r.URL.Query().Get("external_transaction_id") != "" {
		// Implement this when there is a use case for it.
		httperror.NotFound("Get a transaction by the external transaction ID not supported", nil, nil).Render(w)
		return
	}

	if r.URL.Query().Get("stellar_transaction_id") != "" {
		// Implement this when there is a use case for it.
		httperror.NotFound("Get a transaction by the stellar transaction ID is not supported", nil, nil).Render(w)
		return
	}

	transactionID := r.URL.Query().Get("id")
	if transactionID == "" {
		httperror.BadRequest("id parameter is required", nil, nil).Render(w)
		return
	}

	// Check if the transaction ID was created as a SEP-24 transaction in the database
	_, err := h.Models.SEP24Transactions.GetByID(ctx, transactionID)
	if err != nil {
		if errors.Is(err, data.ErrRecordNotFound) {
			httperror.NotFound("Transaction not found", nil, nil).Render(w)
			return
		}
		httperror.InternalError(ctx, "Failed to get transaction", err, nil).Render(w)
		return
	}

	transaction := map[string]any{
		"id":       transactionID,
		"kind":     SEP24OperationKindDeposit,
		"refunded": false, // Always false for registration
	}

	receiverWallet, err := h.Models.ReceiverWallet.GetBySEP24TransactionID(ctx, transactionID)
	if err != nil {
		if errors.Is(err, data.ErrRecordNotFound) {
			transaction["status"] = SEP24StatusIncomplete
			transaction["started_at"] = time.Now().UTC().Format(time.RFC3339)
			// The sep10Claims.Subject is the Stellar address. It is either the account of the deposit request or the account of the SEP-10 challenge.
			transaction["to"] = sep10Claims.Subject

			moreInfoURL, moreInfoErr := h.generateMoreInfoURL(sep10Claims, transactionID, SEP24StatusIncomplete)
			if moreInfoErr != nil {
				httperror.InternalError(ctx, "Failed to generate more info URL", moreInfoErr, nil).Render(w)
				return
			}
			transaction["more_info_url"] = moreInfoURL
		} else {
			httperror.InternalError(ctx, "Failed to get transaction", err, nil).Render(w)
			return
		}
	} else {
		switch receiverWallet.Status {
		case data.RegisteredReceiversWalletStatus:
			transaction["status"] = SEP24StatusCompleted
			transaction["completed_at"] = receiverWallet.UpdatedAt.UTC().Format(time.RFC3339)
			transaction["stellar_transaction_id"] = ""
			transaction["to"] = receiverWallet.StellarAddress
			if receiverWallet.StellarMemo != "" {
				transaction["deposit_memo"] = receiverWallet.StellarMemo
				transaction["deposit_memo_type"] = receiverWallet.StellarMemoType
			}
		case data.ReadyReceiversWalletStatus:
			transaction["status"] = SEP24StatusPendingUserInfoUpdate

			moreInfoURL, moreInfoErr := h.generateMoreInfoURL(sep10Claims, transactionID, SEP24StatusPendingUserInfoUpdate)
			if moreInfoErr != nil {
				httperror.InternalError(ctx, "Failed to generate more info URL", moreInfoErr, nil).Render(w)
				return
			}
			transaction["more_info_url"] = moreInfoURL
		default:
			transaction["status"] = SEP24StatusError
		}

		transaction["started_at"] = receiverWallet.CreatedAt.UTC().Format(time.RFC3339)

		if receiverWallet.StellarAddress != "" {
			transaction["to"] = receiverWallet.StellarAddress
		}
	}

	response := map[string]any{
		"transaction": transaction,
	}

	httpjson.Render(w, response, httpjson.JSON)
}

func (h SEP24Handler) GetInfo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	assets, err := h.Models.Assets.GetAll(ctx)
	if err != nil {
		httperror.InternalError(ctx, "Cannot retrieve assets", err, nil).Render(w)
		return
	}

	deposit := make(map[string]SEP24OperationResponse)

	for _, asset := range assets {
		assetCode := asset.Code
		if asset.IsNative() {
			assetCode = "native"
		}

		deposit[assetCode] = SEP24OperationResponse{
			Enabled:   true,
			MinAmount: sep24MinAmount,
			MaxAmount: sep24MaxAmount,
		}
	}

	response := SEP24InfoResponse{
		Deposit:  deposit,
		Withdraw: make(map[string]SEP24OperationResponse),
		Fee: SEP24FeeResponse{
			Enabled: false,
		},
		Features: SEP24FeatureFlagResponse{
			AccountCreation:   false,
			ClaimableBalances: false,
		},
	}

	httpjson.RenderStatus(w, http.StatusOK, response, httpjson.JSON)
}

func (h SEP24Handler) PostDepositInteractive(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get SEP-10 claims from middleware
	sep10Claims := sepauth.GetSEP10Claims(ctx)
	if sep10Claims == nil {
		httperror.Unauthorized("Missing or invalid authorization header", nil, nil).Render(w)
		return
	}

	var assetCode, account, lang string
	contentType := r.Header.Get("Content-Type")

	if strings.Contains(contentType, "application/json") {
		var req map[string]string
		if decodeErr := json.NewDecoder(r.Body).Decode(&req); decodeErr != nil {
			httperror.BadRequest("Invalid JSON", decodeErr, nil).Render(w)
			return
		}
		assetCode = req["asset_code"]
		account = req["account"]
		lang = req["lang"]
	} else {
		if parseErr := r.ParseForm(); parseErr != nil {
			httperror.BadRequest("Invalid form data", parseErr, nil).Render(w)
			return
		}
		assetCode = r.FormValue("asset_code")
		account = r.FormValue("account")
		lang = r.FormValue("lang")
	}

	if assetCode == "" {
		httperror.BadRequest("asset_code is required", nil, nil).Render(w)
		return
	}

	if account == "" {
		account = sep10Claims.Subject
		if idx := strings.Index(account, ":"); idx > 0 {
			account = account[:idx]
		}
	}

	if lang == "" {
		lang = "en"
	}

	txnID := uuid.New().String()

	// Check if the account is a valid Stellar address
	if !strkey.IsValidEd25519PublicKey(account) {
		httperror.BadRequest("Invalid account", nil, nil).Render(w)
		return
	}

	// Check if assetCode is defined in the assets table
	exists, err := h.Models.Assets.ExistsByCodeOrID(ctx, assetCode)
	if !exists || err != nil {
		httperror.BadRequest("Asset not found", err, nil).Render(w)
		return
	}

	sep24Token, err := h.SEP24JWTManager.GenerateSEP24Token(
		account,
		"",
		sep10Claims.ClientDomain,
		sep10Claims.HomeDomain,
		txnID,
	)
	if err != nil {
		httperror.InternalError(ctx, "Failed to generate token", err, nil).Render(w)
		return
	}

	baseURLParsed, err := url.Parse(h.InteractiveBaseURL)
	if err != nil {
		httperror.InternalError(ctx, "Failed to parse base URL", err, nil).Render(w)
		return
	}

	tenantBaseURL := fmt.Sprintf("%s://%s", baseURLParsed.Scheme, sep10Claims.HomeDomain)

	interactiveURL := fmt.Sprintf("%s/wallet-registration/start?transaction_id=%s&token=%s&lang=%s",
		tenantBaseURL, txnID, sep24Token, lang)

	response := map[string]any{
		"type": "interactive_customer_info_needed",
		"url":  interactiveURL,
		"id":   txnID,
	}

	// Save the transaction ID to the database
	_, err = h.Models.SEP24Transactions.Insert(ctx, txnID)
	if err != nil {
		if errors.Is(err, data.ErrRecordAlreadyExists) {
			httperror.BadRequest("Transaction ID collision detected. Please try again.", err, nil).Render(w)
			return
		}
		httperror.InternalError(ctx, "Failed to save transaction", err, nil).Render(w)
		return
	}

	log.Ctx(ctx).Infof("SEP-24 deposit initiated - ID: %s, Account: %s, Asset: %s, ClientDomain: %s",
		txnID, account, assetCode, sep10Claims.ClientDomain)

	httpjson.Render(w, response, httpjson.JSON)
}
