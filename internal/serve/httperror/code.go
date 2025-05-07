package httperror

const (
	Code400_0 = "400_0" // Invalid request body.
	Code400_1 = "400_1" // ReCAPTCHA token is invalid.
	Code400_2 = "400_2" // The information you provided could not be found.
	Code400_3 = "400_3" // The number of attempts to confirm the verification value exceeded the max attempts.
	Code401_0 = "401_0" // Not authorized.
	Code500_0 = "500_0" // An internal error occurred while processing this request.
	Code500_1 = "500_1" // Cannot get organization.
	Code500_2 = "500_2" // Cannot retrieve the tenant from the context.
	Code500_3 = "500_3" // Cannot get logo URL.
	Code500_4 = "500_4" // Cannot register receiver wallet.
	Code500_5 = "500_5" // Cannot validate reCAPTCHA token.
	Code500_6 = "500_6" // Unexpected contact info.
	Code500_7 = "500_7" // Cannot generate OTP for receiver wallet.
	Code500_8 = "500_8" // Cannot update OTP for receiver wallet.
	Code500_9 = "500_9" // Failed to send OTP message.

	Extra_0 = "EXTRA_0" // field cannot be empty
	Extra_1 = "EXTRA_1" // Invalid OTP format. Needs to be a 6 digit value.
	Extra_2 = "EXTRA_2" // Invalid date of birth format. Correct format: 1990-01-30.
	Extra_3 = "EXTRA_3" // Invalid year/month format. Correct format: 1990-12.
	Extra_4 = "EXTRA_4" // Date cannot be in the future.
	Extra_5 = "EXTRA_5" // Invalid pin length. Cannot have less than 4 or more than 8 characters in pin.
	Extra_6 = "EXTRA_6" // Invalid national id. Cannot have more than 50 characters in national id.
)
