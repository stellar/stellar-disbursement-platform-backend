package message

import "context"

//go:generate mockery --name=MessengerClient  --case=underscore --structname=MessengerClientMock --inpackage
type MessengerClient interface {
	SendMessage(ctx context.Context, message Message) error
	MessengerType() MessengerType
}
