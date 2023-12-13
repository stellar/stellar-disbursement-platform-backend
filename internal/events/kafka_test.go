package events

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type MockEventHandler struct {
	mock.Mock
}

var _ EventHandler = new(MockEventHandler)

func (h *MockEventHandler) Name() string {
	return "MockEventHandler"
}

func (h *MockEventHandler) CanHandleMessage(ctx context.Context, message *Message) bool {
	args := h.Called(ctx, message)
	return args.Bool(0)
}

func (h *MockEventHandler) Handle(ctx context.Context, message *Message) {
	h.Called(ctx, message)
}

func Test_KafkaEventManager_RegisterEventHandler(t *testing.T) {
	ctx := context.Background()

	t.Run("register handler successfully", func(t *testing.T) {
		k := KafkaEventManager{}
		assert.Empty(t, k.handlers)
		eh := MockEventHandler{}
		err := k.RegisterEventHandler(ctx, &eh)
		require.NoError(t, err)
		assert.Equal(t, []EventHandler{&eh}, k.handlers)
	})

	t.Run("no handler duplicated", func(t *testing.T) {
		k := KafkaEventManager{}
		assert.Empty(t, k.handlers)
		eh := MockEventHandler{}
		err := k.RegisterEventHandler(ctx, &eh, &eh)
		require.NoError(t, err)
		assert.Equal(t, []EventHandler{&eh}, k.handlers)
	})
}
