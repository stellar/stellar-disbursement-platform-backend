package events

import (
	"context"
	"fmt"
)

type EventHandler interface {
	Name() string
	CanHandleMessage(ctx context.Context, message *Message) bool
	Handle(ctx context.Context, message *Message) error
}

type PingPongRequest struct {
	Message string `json:"message"`
}

// PingPongEventHandler is a example of event handler
type PingPongEventHandler struct{}

var _ EventHandler = new(PingPongEventHandler)

func (h *PingPongEventHandler) Name() string {
	return "PingPong.EventHandler"
}

func (h *PingPongEventHandler) CanHandleMessage(ctx context.Context, message *Message) bool {
	return message.Topic == "ping-pong"
}

func (h *PingPongEventHandler) Handle(ctx context.Context, message *Message) error {
	if message.Type == "ping" {
		fmt.Println("pong")
	} else {
		fmt.Println("ping")
	}

	return nil
}
