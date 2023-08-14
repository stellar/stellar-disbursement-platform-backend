package message

import (
	"github.com/stretchr/testify/mock"
)

type MessengerClientMock struct {
	mock.Mock
}

func (mc *MessengerClientMock) SendMessage(message Message) error {
	args := mc.Called(message)
	return args.Error(0)
}

func (mc *MessengerClientMock) MessengerType() MessengerType {
	args := mc.Called()
	return args.Get(0).(MessengerType)
}

var _ MessengerClient = (*MessengerClientMock)(nil)
