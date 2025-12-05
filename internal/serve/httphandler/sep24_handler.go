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

	"github.com/stellar/go-stellar-sdk/support/log"
	"github.com/stellar/go-stellar-sdk/support/render/httpjson"

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
func (h SEP24Handler) generateMoreInfoURL(webAuthClaims *sepauth.WebAuthClaims, transactionID, status string) (string, error) {
	sep24Token, err := h.SEP24JWTManager.GenerateSEP24MoreInfoToken(
		webAuthClaims.Subject,
		"",
		webAuthClaims.ClientDomain,
		webAuthClaims.HomeDomain,
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

	tenantBaseURL := fmt.Sprintf("%s://%s", baseURLParsed.Scheme, webAuthClaims.HomeDomain)
	return fmt.Sprintf("%s/wallet-registration/start?transaction_id=%s&token=%s",
		tenantBaseURL, transactionID, sep24Token), nil
}

func (h SEP24Handler) GetTransaction(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	webAuthClaims := sepauth.GetWebAuthClaims(ctx)
	if webAuthClaims == nil {
		httperror.Unauthorized("Missing or invalid authorization header", nil, nil).Render(w)
		return
	}

	transactionID := r.URL.Query().Get("id")
	if transactionID == "" {
		httperror.BadRequest("id parameter is required", nil, nil).Render(w)
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

			moreInfoURL, moreInfoErr := h.generateMoreInfoURL(webAuthClaims, transactionID, SEP24StatusIncomplete)
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

			moreInfoURL, moreInfoErr := h.generateMoreInfoURL(webAuthClaims, transactionID, SEP24StatusPendingUserInfoUpdate)
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

	// Get SEP-10/45 claims from middleware
	webAuthClaims := sepauth.GetWebAuthClaims(ctx)
	if webAuthClaims == nil {
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
		assetCode = r.FormValue("asset_code")
		account = r.FormValue("account")
		lang = r.FormValue("lang")
	}

	if assetCode == "" {
		httperror.BadRequest("asset_code is required", nil, nil).Render(w)
		return
	}

	if account == "" {
		account = webAuthClaims.Subject
		if idx := strings.Index(account, ":"); idx > 0 {
			account = account[:idx]
		}
	}

	if lang == "" {
		lang = "en"
	}

	txnID := uuid.New().String()

	sep24Token, err := h.SEP24JWTManager.GenerateSEP24Token(
		account,
		"",
		webAuthClaims.ClientDomain,
		webAuthClaims.HomeDomain,
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

	tenantBaseURL := fmt.Sprintf("%s://%s", baseURLParsed.Scheme, webAuthClaims.HomeDomain)

	interactiveURL := fmt.Sprintf("%s/wallet-registration/start?transaction_id=%s&token=%s&lang=%s",
		tenantBaseURL, txnID, sep24Token, lang)

	response := map[string]any{
		"type": "interactive_customer_info_needed",
		"url":  interactiveURL,
		"id":   txnID,
	}

	log.Ctx(ctx).Infof("SEP-24 deposit initiated - ID: %s, Account: %s, Asset: %s, ClientDomain: %s",
		txnID, account, assetCode, webAuthClaims.ClientDomain)

	httpjson.Render(w, response, httpjson.JSON)
}
