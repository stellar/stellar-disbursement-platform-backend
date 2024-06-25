package dependencyinjection

import (
	"context"
	"fmt"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpclient"
)

const HorizonClientInstanceName = "horizon_client_instance"

// NewHorizonClient creates a new horizon client instance, or retrives an instance that was already
// created before.
func NewHorizonClient(ctx context.Context, horizonURL string) (horizonclient.ClientInterface, error) {
	instanceName := HorizonClientInstanceName

	// Already initialized
	if instance, ok := GetInstance(instanceName); ok {
		if castedInstance, ok2 := instance.(horizonclient.ClientInterface); ok2 {
			return castedInstance, nil
		}
		return nil, fmt.Errorf("trying to cast an existing horizon client instance")
	}

	// Setup a new instance
	log.Ctx(ctx).Infof("⚙️ Setting up Horizon Client")
	newInstance := &horizonclient.Client{
		HorizonURL: horizonURL,
		HTTP:       httpclient.DefaultClient(),
	}

	SetInstance(instanceName, newInstance)

	return newInstance, nil
}
