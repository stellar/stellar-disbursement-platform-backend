package dependencyinjection

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
)

func Test_NewMessageDispatcher(t *testing.T) {
	ctx := context.Background()
	t.Run("should return the same instance when called twice", func(t *testing.T) {
		defer ClearInstancesTestHelper(t)

		opts := MessageDispatcherOpts{
			EmailOpts: &EmailClientOptions{
				EmailType: message.MessengerTypeDryRun,
			},
			SMSOpts: &SMSClientOptions{
				SMSType: message.MessengerTypeDryRun,
			},
		}

		dispatcher1, err := NewMessageDispatcher(ctx, opts)
		require.NoError(t, err)
		dispatcher2, err := NewMessageDispatcher(ctx, opts)
		require.NoError(t, err)
		assert.Equal(t, dispatcher1, dispatcher2)
	})

	t.Run("should create dispatcher with email client only", func(t *testing.T) {
		defer ClearInstancesTestHelper(t)

		opts := MessageDispatcherOpts{
			EmailOpts: &EmailClientOptions{
				EmailType: message.MessengerTypeDryRun,
			},
		}

		dispatcher, err := NewMessageDispatcher(ctx, opts)
		require.NoError(t, err)

		emailClient, err := dispatcher.GetClient(message.MessageChannelEmail)
		require.NoError(t, err)
		assert.NotNil(t, emailClient)

		smsClient, err := dispatcher.GetClient(message.MessageChannelSMS)
		assert.EqualError(t, err, "no client registered for channel \"SMS\"")
		assert.Nil(t, smsClient)
	})

	t.Run("should create dispatcher with SMS client only", func(t *testing.T) {
		defer ClearInstancesTestHelper(t)

		opts := MessageDispatcherOpts{
			SMSOpts: &SMSClientOptions{
				SMSType: message.MessengerTypeDryRun,
			},
		}

		dispatcher, err := NewMessageDispatcher(ctx, opts)
		require.NoError(t, err)

		smsClient, err := dispatcher.GetClient(message.MessageChannelSMS)
		require.NoError(t, err)
		assert.NotNil(t, smsClient)

		emailClient, err := dispatcher.GetClient(message.MessageChannelEmail)
		assert.EqualError(t, err, "no client registered for channel \"EMAIL\"")
		assert.Nil(t, emailClient)
	})

	t.Run("should return an error on invalid email client creation", func(t *testing.T) {
		defer ClearInstancesTestHelper(t)

		opts := MessageDispatcherOpts{
			EmailOpts: &EmailClientOptions{
				EmailType: "invalid-type",
			},
		}

		dispatcher, err := NewMessageDispatcher(ctx, opts)
		assert.ErrorContains(t, err, `trying to create a Email client with a non-supported Email type: "invalid-type"`)
		assert.Nil(t, dispatcher)
	})

	t.Run("should return an error on invalid SMS client creation", func(t *testing.T) {
		defer ClearInstancesTestHelper(t)

		opts := MessageDispatcherOpts{
			SMSOpts: &SMSClientOptions{
				SMSType: "invalid-type",
			},
		}

		dispatcher, err := NewMessageDispatcher(ctx, opts)
		assert.ErrorContains(t, err, `trying to create a SMS client with a non-supported SMS type: "invalid-type"`)
		assert.Nil(t, dispatcher)
	})

	t.Run("should return an error on invalid pre-existing instance", func(t *testing.T) {
		defer ClearInstancesTestHelper(t)

		preExistingDispatcherWithInvalidType := struct{}{}
		SetInstance(MessageDispatcherInstanceName, preExistingDispatcherWithInvalidType)

		opts := MessageDispatcherOpts{}

		gotDispatcher, err := NewMessageDispatcher(ctx, opts)
		assert.Nil(t, gotDispatcher)
		assert.EqualError(t, err, "trying to cast pre-existing MessageDispatcher for dependency injection")
	})
}
