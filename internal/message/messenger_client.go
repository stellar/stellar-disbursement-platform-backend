package message

//go:generate mockery --name=MessengerClient  --case=underscore --structname=MessengerClientMock --inpackage
type MessengerClient interface {
	SendMessage(message Message) error
	MessengerType() MessengerType
}
