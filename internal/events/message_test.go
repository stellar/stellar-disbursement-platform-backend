package events

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_Message_Validate(t *testing.T) {
	m := Message{}

	err := m.Validate()
	assert.ErrorIs(t, err, ErrTopicRequired)

	m.Topic = "test-topic"
	err = m.Validate()
	assert.ErrorIs(t, err, ErrKeyRequired)

	m.Key = "test-key"
	err = m.Validate()
	assert.ErrorIs(t, err, ErrTenantIDRequired)

	m.TenantID = "tenant-ID"
	err = m.Validate()
	assert.ErrorIs(t, err, ErrTypeRequired)

	m.Type = "test-type"
	err = m.Validate()
	assert.ErrorIs(t, err, ErrDataRequired)

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
