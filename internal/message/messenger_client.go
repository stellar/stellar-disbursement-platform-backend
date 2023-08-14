package message

type MessengerClient interface {
	SendMessage(message Message) error
	MessengerType() MessengerType
}
