package validators

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_UpdateReceiverValidator_ValidateReceiver2(t *testing.T) {
	testCases := []struct {
		name           string
		request        UpdateReceiverRequest
		expectedErrors map[string]interface{}
	}{
		{
			name:    "Empty request",
			request: UpdateReceiverRequest{},
			expectedErrors: map[string]interface{}{
				"body": "request body is empty",
			},
		},
		{
			name: "[DATE_OF_BIRTH] ValidationField is invalid",
			request: UpdateReceiverRequest{
				DateOfBirth: "invalid",
			},
			expectedErrors: map[string]interface{}{
				"date_of_birth": "invalid date of birth format. Correct format: 1990-01-30",
			},
		},
		{
			name: "[YEAR_MONTH] ValidationField is invalid",
			request: UpdateReceiverRequest{
				YearMonth: "invalid",
			},
			expectedErrors: map[string]interface{}{
				"year_month": "invalid year/month format. Correct format: 1990-12",
			},
		},
		{
			name: "[PIN] ValidationField is invalid",
			request: UpdateReceiverRequest{
				Pin: "   ",
			},
			expectedErrors: map[string]interface{}{
				"pin": "invalid pin length. Cannot have less than 4 or more than 8 characters in pin",
			},
		},
		{
			name: "[NATIONAL_ID_NUMBER] ValidationField is invalid",
			request: UpdateReceiverRequest{
				NationalID: "   ",
			},
			expectedErrors: map[string]interface{}{
				"national_id": "national id cannot be empty",
			},
		},
		{
			name: "e-mail is invalid",
			request: UpdateReceiverRequest{
				Email: "invalid",
			},
			expectedErrors: map[string]interface{}{
				"email": "invalid email format",
			},
		},
		{
			name: "phone number is invalid",
			request: UpdateReceiverRequest{
				PhoneNumber: "invalid",
			},
			expectedErrors: map[string]interface{}{
				"phone_number": "invalid phone number format",
			},
		},
		{
			name: "external ID is invalid",
			request: UpdateReceiverRequest{
				ExternalID: "    ",
			},
			expectedErrors: map[string]interface{}{
				"external_id": "external_id cannot be set to empty",
			},
		},
		{
			name: "🎉 Valid receiver values",
			request: UpdateReceiverRequest{
				DateOfBirth: "1999-01-01",
				YearMonth:   "1999-01",
				Pin:         "1234   ",
				NationalID:  "   12345CODE",
				Email:       "receiver@email.com",
				PhoneNumber: "+14155556666",
				ExternalID:  "externalID",
			},
			expectedErrors: map[string]interface{}{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			validator := NewUpdateReceiverValidator()
			validator.ValidateReceiver(&tc.request)

			assert.Equal(t, len(tc.expectedErrors), len(validator.Errors))
			assert.Equal(t, tc.expectedErrors, validator.Errors)
			for key, value := range tc.expectedErrors {
				assert.Equal(t, value, validator.Errors[key])
			}
		})
	}
}
