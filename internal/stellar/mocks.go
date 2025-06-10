package stellar

import (
	"context"

	"github.com/stellar/stellar-rpc/protocol"
	"github.com/stretchr/testify/mock"
)

type MockRPCClient struct {
	mock.Mock
}

var _ RPCClient = (*MockRPCClient)(nil)

func (m *MockRPCClient) SimulateTransaction(ctx context.Context, request protocol.SimulateTransactionRequest) (protocol.SimulateTransactionResponse, error) {
	args := m.Called(ctx, request)
	if args.Get(0) == nil {
		return protocol.SimulateTransactionResponse{}, args.Error(1)
	}
	return args.Get(0).(protocol.SimulateTransactionResponse), args.Error(1)
}
