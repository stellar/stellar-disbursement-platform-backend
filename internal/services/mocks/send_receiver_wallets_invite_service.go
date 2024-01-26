package mocks

import (
	"context"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/events/schemas"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stretchr/testify/mock"
)

type MockSendReceiverWalletInviteService struct {
	mock.Mock
}

var _ services.SendReceiverWalletInviteServiceInterface = new(MockSendReceiverWalletInviteService)

func (s *MockSendReceiverWalletInviteService) SendInvite(ctx context.Context, receiverWalletsReq ...schemas.EventReceiverWalletSMSInvitationData) error {
	args := s.Called(ctx, receiverWalletsReq)
	return args.Error(0)
}
