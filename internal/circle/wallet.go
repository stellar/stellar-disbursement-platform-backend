package circle

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type WalletResponse struct {
	Data Wallet `json:"data"`
}

type Wallet struct {
	WalletID    string    `json:"walletId"`
	EntityID    string    `json:"entityId"`
	Type        string    `json:"type"`
	Description string    `json:"description"`
	Balances    []Balance `json:"balances"`
}

// parseWalletResponse parses the response from the Circle API into a Wallet struct.
func parseWalletResponse(resp *http.Response) (*Wallet, error) {
	var walletResponse WalletResponse
	if err := json.NewDecoder(resp.Body).Decode(&walletResponse); err != nil {
		return nil, fmt.Errorf("unmarshalling Circle HTTP response: %w", err)
	}

	return &walletResponse.Data, nil
}
