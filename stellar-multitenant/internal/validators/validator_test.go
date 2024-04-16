package validators

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_NewValidator(t *testing.T) {
	validator := NewValidator()
	assert.NotNil(t, validator)
	assert.NotNil(t, validator.Errors)
}

func Test_Check(t *testing.T) {
	validator := NewValidator()
	validator.Check(true, "key", "error message")

	assert.Emptyf(t, validator.Errors, "validator should not have errors")

	validator.Check(false, "key", "error message")
	assert.NotEmpty(t, validator.Errors)
	assert.Equal(t, validator.Errors["key"], "error message")
}

func Test_HasErrors(t *testing.T) {
	validator := NewValidator()
	assert.False(t, validator.HasErrors())

	validator.Check(false, "key", "error message")
	assert.True(t, validator.HasErrors())
}

func Test_addError(t *testing.T) {
	validator := NewValidator()
	validator.addError("key", "error message")
	validator.addError("key2", "error message 2")
	assert.Equal(t, len(validator.Errors), 2)
	assert.Equal(t, validator.Errors["key"], "error message")
	assert.Equal(t, validator.Errors["key2"], "error message 2")
}
