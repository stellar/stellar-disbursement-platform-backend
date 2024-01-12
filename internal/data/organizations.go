package data

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"image"
	"regexp"

	// Don't remove the `image/jpeg` and `image/png` packages import unless
	// the `image` package is no longer necessary.
	// It registers the `Decoders` to handle the image decoding - `image.Decode`.
	// See https://pkg.go.dev/image#pkg-overview
	_ "image/jpeg"
	_ "image/png"
	"strings"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
)

const (
	DefaultSMSRegistrationMessageTemplate = "You have a payment waiting for you from the {{.OrganizationName}}. Click {{.RegistrationLink}} to register."
	DefaultOTPMessageTemplate             = "{{.OTP}} is your {{.OrganizationName}} phone verification code."
)

type Organization struct {
	ID                string `json:"id" db:"id"`
	Name              string `json:"name" db:"name"`
	TimezoneUTCOffset string `json:"timezone_utc_offset" db:"timezone_utc_offset"`
	// SMSResendInterval is the time period that SDP will wait to resend the invitation SMS to the receivers that aren't registered.
	// If it's nil means resending the invitation SMS is deactivated.
	SMSResendInterval *int64 `json:"sms_resend_interval" db:"sms_resend_interval"`
	// PaymentCancellationPeriodDays is the number of days for a ready payment to be automatically cancelled.
	PaymentCancellationPeriodDays  *int64 `json:"payment_cancellation_period_days" db:"payment_cancellation_period_days"`
	SMSRegistrationMessageTemplate string `json:"sms_registration_message_template" db:"sms_registration_message_template"`
	// OTPMessageTemplate is the message template to send the OTP code to the receivers validates their identity when registering their wallets.
	// The message may have the template values {{.OTP}} and {{.OrganizationName}}, it will be parsed and the values injected when executing the template.
	// When the {{.OTP}} is not found in the message, it's added at the beginning of the message.
	// Example:
	//	{{.OTP}} OTPMessageTemplate
	OTPMessageTemplate string    `json:"otp_message_template" db:"otp_message_template"`
	Logo               []byte    `db:"logo"`
	IsApprovalRequired bool      `json:"is_approval_required" db:"is_approval_required"`
	CreatedAt          time.Time `json:"created_at" db:"created_at"`
	UpdatedAt          time.Time `json:"updated_at" db:"updated_at"`
}

type OrganizationUpdate struct {
	Name                          string `json:",omitempty"`
	Logo                          []byte `json:",omitempty"`
	TimezoneUTCOffset             string `json:",omitempty"`
	IsApprovalRequired            *bool  `json:",omitempty"`
	SMSResendInterval             *int64 `json:",omitempty"`
	PaymentCancellationPeriodDays *int64 `json:",omitempty"`

	// Using pointers to accept empty strings
	SMSRegistrationMessageTemplate *string `json:",omitempty"`
	OTPMessageTemplate             *string `json:",omitempty"`
}

type LogoType string

const (
	PNGLogoType  LogoType = "png"
	JPEGLogoType LogoType = "jpeg"

	// tzRegexExpression validates the TimezoneUTCOffset value. It expects the following format:
	// 	plus or minus symbol + two numbers + colon symbol + two numbers
	// Example:
	// 	+02:00 or -03:00
	// Any other value will be invalid.
	tzRegexExpression string = `^(\+|-)\d{2}:\d{2}$`
)

var tzRegex *regexp.Regexp

func init() {
	tzRegex = regexp.MustCompile(tzRegexExpression)
}

func (lt LogoType) ToHTTPContentType() string {
	return fmt.Sprintf("image/%s", lt)
}

func (ou *OrganizationUpdate) validate() error {
	if ou.areAllFieldsEmpty() {
		return fmt.Errorf("name, timezone UTC offset, approval workflow flag, SMS Resend Interval, SMS invite template, OTP message template or logo is required")
	}

	if len(ou.Logo) > 0 {
		_, format, err := image.Decode(bytes.NewBuffer(ou.Logo))
		if err != nil {
			return fmt.Errorf("error decoding image bytes: %w", err)
		}

		if !strings.Contains(fmt.Sprintf("%s %s", PNGLogoType, JPEGLogoType), format) {
			return fmt.Errorf("invalid image type provided. Expect %s or %s", PNGLogoType, JPEGLogoType)
		}
	}

	if ou.TimezoneUTCOffset != "" && !tzRegex.MatchString(ou.TimezoneUTCOffset) {
		return fmt.Errorf("invalid timezone UTC offset format. Example: +02:00 or -03:00")
	}

	return nil
}

func (ou *OrganizationUpdate) areAllFieldsEmpty() bool {
	return (ou.Name == "" &&
		len(ou.Logo) == 0 &&
		ou.TimezoneUTCOffset == "" &&
		ou.IsApprovalRequired == nil &&
		ou.SMSRegistrationMessageTemplate == nil &&
		ou.OTPMessageTemplate == nil &&
		ou.SMSResendInterval == nil &&
		ou.PaymentCancellationPeriodDays == nil)
}

type OrganizationModel struct {
	dbConnectionPool db.DBConnectionPool
}

func (om *OrganizationModel) Get(ctx context.Context) (*Organization, error) {
	var organization Organization
	query := `
		SELECT
			*
		FROM 
			organizations o
		LIMIT 1
	`

	err := om.dbConnectionPool.GetContext(ctx, &organization, query)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("error querying organization table: %w", err)
	}

	return &organization, nil
}

func (om *OrganizationModel) Update(ctx context.Context, ou *OrganizationUpdate) error {
	if err := ou.validate(); err != nil {
		return fmt.Errorf("invalid organization update: %w", err)
	}

	query := `
		WITH org_cte AS (
			SELECT id FROM organizations LIMIT 1
		)
		UPDATE
			organizations
		SET
			%s
		FROM org_cte
		WHERE organizations.id = org_cte.id
	`

	args := []interface{}{}
	fields := []string{}
	if ou.Name != "" {
		fields = append(fields, "name = ?")
		args = append(args, ou.Name)
	}

	if len(ou.Logo) > 0 {
		fields = append(fields, "logo = ?")
		args = append(args, ou.Logo)
	}

	if ou.TimezoneUTCOffset != "" {
		fields = append(fields, "timezone_utc_offset = ?")
		args = append(args, ou.TimezoneUTCOffset)
	}

	if ou.IsApprovalRequired != nil {
		fields = append(fields, "is_approval_required = ?")
		args = append(args, *ou.IsApprovalRequired)
	}

	if ou.SMSRegistrationMessageTemplate != nil {
		if *ou.SMSRegistrationMessageTemplate != "" {
			fields = append(fields, "sms_registration_message_template = ?")
			args = append(args, *ou.SMSRegistrationMessageTemplate)
		} else {
			// When empty value is passed by parameter we set the DEFAULT value for the column.
			fields = append(fields, "sms_registration_message_template = DEFAULT")
		}
	}

	if ou.OTPMessageTemplate != nil {
		if *ou.OTPMessageTemplate != "" {
			fields = append(fields, "otp_message_template = ?")
			args = append(args, *ou.OTPMessageTemplate)
		} else {
			// When empty value is passed by parameter we set the DEFAULT value for the column.
			fields = append(fields, "otp_message_template = DEFAULT")
		}
	}

	if ou.SMSResendInterval != nil {
		if *ou.SMSResendInterval > 0 {
			fields = append(fields, "sms_resend_interval = ?")
			args = append(args, *ou.SMSResendInterval)
		} else {
			// When 0 (zero) is passed by parameter we set it as NULL.
			fields = append(fields, "sms_resend_interval = NULL")
		}
	}

	if ou.PaymentCancellationPeriodDays != nil {
		if *ou.PaymentCancellationPeriodDays > 0 {
			fields = append(fields, "payment_cancellation_period_days = ?")
			args = append(args, *ou.PaymentCancellationPeriodDays)
		} else {
			fields = append(fields, "payment_cancellation_period_days = NULL")
		}
	}

	query = om.dbConnectionPool.Rebind(fmt.Sprintf(query, strings.Join(fields, ", ")))

	_, err := om.dbConnectionPool.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("error updating organization: %w", err)
	}

	return nil
}
