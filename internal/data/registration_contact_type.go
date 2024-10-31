package data

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// RegistrationContactType represents the type of contact information to be used when creating and validating a disbursement.
type RegistrationContactType struct {
	ReceiverContactType   ReceiverContactType `json:"registration_contact_type"`
	IncludesWalletAddress bool                `json:"includes_wallet_address"`
}

var (
	RegistrationContactTypeEmail                 = RegistrationContactType{ReceiverContactTypeEmail, false}
	RegistrationContactTypePhone                 = RegistrationContactType{ReceiverContactTypeSMS, false}
	RegistrationContactTypeEmailAndWalletAddress = RegistrationContactType{ReceiverContactTypeEmail, true}
	RegistrationContactTypePhoneAndWalletAddress = RegistrationContactType{ReceiverContactTypeSMS, true}
)

func (rct RegistrationContactType) String() string {
	if rct.IncludesWalletAddress {
		return fmt.Sprintf("%s_AND_WALLET_ADDRESS", rct.ReceiverContactType)
	}
	return string(rct.ReceiverContactType)
}

// parseFromString parses the string, setting ReceiverContactType and IncludesWalletAddress based on suffix.
func (rct *RegistrationContactType) parseFromString(input string) error {
	rct.IncludesWalletAddress = strings.HasSuffix(input, "_AND_WALLET_ADDRESS")
	rct.ReceiverContactType = ReceiverContactType(strings.TrimSuffix(input, "_AND_WALLET_ADDRESS"))

	if !slices.Contains(GetAllReceiverContactTypes(), rct.ReceiverContactType) {
		return fmt.Errorf("unknown ReceiverContactType = %s", rct.ReceiverContactType)
	}

	return nil
}

func AllRegistrationContactTypes() []RegistrationContactType {
	return []RegistrationContactType{
		RegistrationContactTypeEmail,
		RegistrationContactTypeEmailAndWalletAddress,
		RegistrationContactTypePhone,
		RegistrationContactTypePhoneAndWalletAddress,
	}
}

func (rct RegistrationContactType) MarshalJSON() ([]byte, error) {
	return json.Marshal(rct.String())
}

func (rct *RegistrationContactType) UnmarshalJSON(data []byte) error {
	var typeStr string
	if err := json.Unmarshal(data, &typeStr); err != nil {
		return err
	}

	return rct.parseFromString(typeStr)
}

var _ json.Marshaler = (*RegistrationContactType)(nil)
var _ json.Unmarshaler = (*RegistrationContactType)(nil)

func (rct RegistrationContactType) Value() (driver.Value, error) {
	return rct.String(), nil
}

func (rct *RegistrationContactType) Scan(value interface{}) error {
	if value == nil {
		return nil
	}

	byteValue, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("unexpected type for RegistrationContactType %T", value)
	}

	return rct.parseFromString(string(byteValue))
}

var _ driver.Valuer = (*RegistrationContactType)(nil)
var _ sql.Scanner = (*RegistrationContactType)(nil)
