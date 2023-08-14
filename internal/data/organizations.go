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

type Organization struct {
	ID                             string    `json:"id" db:"id"`
	Name                           string    `json:"name" db:"name"`
	StellarMainAddress             string    `json:"stellar_main_address" db:"stellar_main_address"`
	TimezoneUTCOffset              string    `json:"timezone_utc_offset" db:"timezone_utc_offset"`
	ArePaymentsEnabled             bool      `json:"are_payments_enabled" db:"are_payments_enabled"`
	SMSRegistrationMessageTemplate string    `json:"sms_registration_message_template" db:"sms_registration_message_template"`
	Logo                           []byte    `db:"logo"`
	IsApprovalRequired             bool      `json:"is_approval_required" db:"is_approval_required"`
	CreatedAt                      time.Time `json:"created_at" db:"created_at"`
	UpdatedAt                      time.Time `json:"updated_at" db:"updated_at"`
}

type OrganizationUpdate struct {
	Name               string
	Logo               []byte
	TimezoneUTCOffset  string
	IsApprovalRequired *bool
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
	if ou.Name == "" && len(ou.Logo) == 0 && ou.TimezoneUTCOffset == "" && ou.IsApprovalRequired == nil {
		return fmt.Errorf("name, timezone UTC offset, approval workflow flag or logo is required")
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

func (om *OrganizationModel) ArePaymentsEnabled(ctx context.Context) (bool, error) {
	var arePaymentsEnabled bool
	query := `
		SELECT
			o.are_payments_enabled
		FROM 
		    organizations o
		LIMIT 1
	`

	err := om.dbConnectionPool.GetContext(ctx, &arePaymentsEnabled, query)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, ErrRecordNotFound
		}
		return false, fmt.Errorf("error querying organization table: %w", err)
	}

	return arePaymentsEnabled, nil
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

	query = om.dbConnectionPool.Rebind(fmt.Sprintf(query, strings.Join(fields, ", ")))

	_, err := om.dbConnectionPool.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("error updating organization: %w", err)
	}

	return nil
}
