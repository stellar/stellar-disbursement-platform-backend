package validators

import (
	"fmt"
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

func Test_Validator_CheckError(t *testing.T) {
	testCases := []struct {
		name           string
		err            error
		key            string
		message        string
		expectedErrors map[string]interface{}
	}{
		{
			name:    "error is not nil and message is not empty",
			err:     fmt.Errorf("error message"),
			key:     "key",
			message: "real error message",
			expectedErrors: map[string]interface{}{
				"key": "real error message",
			},
		},
		{
			name:    "error is not nil and message is empty",
			err:     fmt.Errorf("error message"),
			key:     "key",
			message: "",
			expectedErrors: map[string]interface{}{
				"key": "error message",
			},
		},
		{
			name:           "error is nil",
			err:            nil,
			key:            "key",
			message:        "real error message",
			expectedErrors: map[string]interface{}{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			validator := NewValidator()
			validator.CheckError(tc.err, tc.key, tc.message)
			assert.Equal(t, tc.expectedErrors, validator.Errors)
		})
	}
}
