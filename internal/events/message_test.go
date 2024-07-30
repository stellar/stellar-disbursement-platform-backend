package events

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

func Test_Message_Validate(t *testing.T) {
	m := Message{}

	err := m.Validate()
	assert.EqualError(t, err, "message topic is required")

	m.Topic = "test-topic"
	err = m.Validate()
	assert.EqualError(t, err, "message key is required")

	m.Key = "test-key"
	err = m.Validate()
	assert.EqualError(t, err, "message tenant ID is required")

	m.TenantID = "tenant-ID"
	err = m.Validate()
	assert.EqualError(t, err, "message type is required")

	m.Type = "test-type"
	err = m.Validate()
	assert.EqualError(t, err, "message data is required")

	m.Data = "test"
	err = m.Validate()
	assert.NoError(t, err)

	m.Data = nil
	m.Data = map[string]string{"test": "test"}
	err = m.Validate()
	assert.NoError(t, err)

	m.Data = nil
	m.Data = struct{ Name string }{Name: "test"}
	err = m.Validate()
	assert.NoError(t, err)
}

func Test_Message_RecordError(t *testing.T) {
	t.Run("empty when message is created", func(t *testing.T) {
		m := Message{}
		assert.Empty(t, m.Errors)
	})

	t.Run("record error", func(t *testing.T) {
		m := Message{}
		m.RecordError("test-handler", errors.New("test-error"))
		assert.Len(t, m.Errors, 1)
		assert.Equal(t, "test-error", m.Errors[0].ErrorMessage)
		assert.Equal(t, "test-handler", m.Errors[0].HandlerName)
		assert.NotZero(t, m.Errors[0].FailedAt)

		m.RecordError("test-handler-2", errors.New("test-error-2"))
		assert.Len(t, m.Errors, 2)
		assert.Equal(t, "test-error-2", m.Errors[1].ErrorMessage)
		assert.NotZero(t, m.Errors[1].FailedAt)
		assert.Equal(t, "test-handler-2", m.Errors[1].HandlerName)
	})
}

func Test_NewPaymentReadyToPayMessage(t *testing.T) {
	tenantID := "test-tenant"
	key := "test-key"
	messageType := "test-type"

	ctxWithTenant := tenant.SaveTenantInContext(context.Background(), &tenant.Tenant{ID: tenantID})

	t.Run("unsupported platform", func(t *testing.T) {
		_, err := NewPaymentReadyToPayMessage(ctxWithTenant, "unsupported-platform", key, messageType)
		assert.EqualError(t, err, "unsupported platform: unsupported-platform")
	})

	t.Run("stellar platform", func(t *testing.T) {
		m, err := NewPaymentReadyToPayMessage(ctxWithTenant, schema.StellarPlatform, key, messageType)
		assert.NoError(t, err)
		assert.Equal(t, PaymentReadyToPayTopic, m.Topic)
		assert.Equal(t, tenantID, m.TenantID)
	})

	t.Run("circle platform", func(t *testing.T) {
		m, err := NewPaymentReadyToPayMessage(ctxWithTenant, schema.CirclePlatform, key, messageType)
		assert.NoError(t, err)
		assert.Equal(t, CirclePaymentReadyToPayTopic, m.Topic)
		assert.Equal(t, key, m.Key)
		assert.Equal(t, tenantID, m.TenantID)
	})
}
