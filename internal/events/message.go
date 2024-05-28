package events

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

type Message struct {
	Topic                string            `json:"topic"`
	Key                  string            `json:"key"`
	TenantID             string            `json:"tenant_id"`
	Type                 string            `json:"type"`
	Data                 any               `json:"data"`
	Errors               []*HandlerError   `json:"errors,omitempty"`
	SuccessfulExecutions []*HandlerSuccess `json:"successful_executions,omitempty"`
}

type HandlerError struct {
	// FailedAt timestamp for the time of failure.
	FailedAt time.Time `json:"failed_at"`
	// ErrorMessage detailed error message. Used for displaying.
	ErrorMessage string `json:"error_message"`
	// HandlerName name of the handler where the error occurred.
	HandlerName string `json:"handler_name"`
	// Err full handler error.
	Err error `json:"-"`
}

// HandlerSuccess represents a successful handling of a message
type HandlerSuccess struct {
	// ExecutedAt timestamp for the time of successful handling
	ExecutedAt time.Time `json:"executed_at"`
	// HandlerName name of the handler that succeeded
	HandlerName string `json:"handler_name"`
}

// NewMessage returns a new message with values passed by parameters. It also parses the `TenantID` from the context and inject it into the message.
// Returns error if the tenant is not found in the context.
func NewMessage(ctx context.Context, topic, key, messageType string, data any) (*Message, error) {
	tnt, err := tenant.GetTenantFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting tenant from context: %w", err)
	}

	return &Message{
		Topic:    topic,
		Key:      key,
		TenantID: tnt.ID,
		Type:     messageType,
		Data:     data,
	}, nil
}

func NewHandlerErrorWrapper(hError error, handlerName string) HandlerError {
	return HandlerError{
		FailedAt:     time.Now(),
		ErrorMessage: hError.Error(),
		HandlerName:  handlerName,
		Err:          hError,
	}
}

func (m Message) String() string {
	return fmt.Sprintf("Message{Topic: %s, Key: %s, Type: %s, TenantID: %s, Data: %v}", m.Topic, m.Key, m.Type, m.TenantID, m.Data)
}

func (m Message) Validate() error {
	if m.Topic == "" {
		return errors.New("message topic is required")
	}

	if m.Key == "" {
		return errors.New("message key is required")
	}

	if m.TenantID == "" {
		return errors.New("message tenant ID is required")
	}

	if m.Type == "" {
		return errors.New("message type is required")
	}

	if m.Data == nil {
		return errors.New("message data is required")
	}

	return nil
}

func (m *Message) RecordError(hError *HandlerError) {
	m.Errors = append(m.Errors, hError)
}
