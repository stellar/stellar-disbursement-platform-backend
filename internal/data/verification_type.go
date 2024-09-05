package data

type VerificationType string

const (
	VerificationTypeDateOfBirth VerificationType = "DATE_OF_BIRTH"
	VerificationTypeYearMonth   VerificationType = "YEAR_MONTH"
	VerificationTypePin         VerificationType = "PIN"
	VerificationTypeNationalID  VerificationType = "NATIONAL_ID_NUMBER"
)

// GetAllVerificationTypes returns all the available verification types.
func GetAllVerificationTypes() []VerificationType {
	return []VerificationType{
		VerificationTypeDateOfBirth,
		VerificationTypeYearMonth,
		VerificationTypePin,
		VerificationTypeNationalID,
	}
}
