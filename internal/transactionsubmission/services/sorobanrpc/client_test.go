package sorobanrpc

import (
	"context"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stellar/go/support/log"
	"github.com/stretchr/testify/require"
)

func Test_Ping(t *testing.T) {
	ctx := context.Background()
	log.DefaultLogger = log.New()
	log.DefaultLogger.SetLevel(logrus.TraceLevel)

	// Create a new Bearer token authenticator
	auth := &BearerTokenAuthenticator{Token: "TODO: Insert your token here"}
	sorobanRPCURL := "https://soroban-testnet.stellar.org"
	client := NewClient(sorobanRPCURL, auth)

	resp, err := client.Call(ctx, 1, "getVersionInfo", nil)
	require.NoError(t, err)

	t.Logf("Soroban RPC version: %s", resp.Result)
}
