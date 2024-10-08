package circle

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// ConfigurationResponse represents the response containing account configuration.
type ConfigurationResponse struct {
	Data AccountConfiguration `json:"data,omitempty"`
}

// AccountConfiguration represents the configuration settings of an account.
type AccountConfiguration struct {
	Payments WalletConfig `json:"payments,omitempty"`
}

// WalletConfig represents the wallet configuration with details such as the master wallet ID.
type WalletConfig struct {
	MasterWalletID string `json:"masterWalletId,omitempty"`
}

// parseAccountConfigurationResponse parses the response containing account configuration.
func parseAccountConfigurationResponse(resp *http.Response) (*AccountConfiguration, error) {
	var configurationResponse ConfigurationResponse
	if err := json.NewDecoder(resp.Body).Decode(&configurationResponse); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &configurationResponse.Data, nil
}
