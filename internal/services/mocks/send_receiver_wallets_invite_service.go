package mocks

import (
	"context"

	"github.com/stretchr/testify/mock"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events/schemas"
)

type MockSendReceiverWalletInviteService struct {
	mock.Mock
}

func (s *MockSendReceiverWalletInviteService) SendInvite(ctx context.Context, receiverWalletsReq ...schemas.EventReceiverWalletInvitationData) error {
	args := s.Called(ctx, receiverWalletsReq)
	return args.Error(0)
}

func (s *MockSendReceiverWalletInviteService) GenerateInvitationLinkForPayment(ctx context.Context, payment data.Payment, receiver data.Receiver) (string, error) {
	args := s.Called(ctx, payment, receiver)
	return args.String(0), args.Error(1)
}
