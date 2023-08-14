package data

import (
	"bytes"
	"context"
	"encoding/csv"
	"image/gif"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Organizations_DatabaseTriggers(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	t.Run("SQL query will trigger an error if you try to have more than one organization", func(t *testing.T) {
		q := `
			INSERT INTO organizations (
				name, stellar_main_address, timezone_utc_offset, are_payments_enabled, sms_registration_message_template
			)
			VALUES (
				'Test name', 'Test Stellar address', '+00:00', false, 'Test template {{.OrganizationName}} {{.RegistrationLink}}.'
			)
		`
		_, err := dbConnectionPool.ExecContext(ctx, q)
		require.EqualError(t, err, "pq: public.organizations can must contain exactly one row")
	})

	t.Run("SQL query will trigger an error if you try to delete the one organization you must have", func(t *testing.T) {
		q := "DELETE FROM organizations"
		_, err := dbConnectionPool.ExecContext(ctx, q)
		require.EqualError(t, err, "pq: public.organizations can must contain exactly one row")
	})

	t.Run("updating sms_registration_message_template without the tags {{.OrganizationName}} and {{.RegistrationLink}} will trigger an error", func(t *testing.T) {
		q := "UPDATE organizations SET sms_registration_message_template = 'Test template'"
		_, err := dbConnectionPool.ExecContext(ctx, q)
		require.EqualError(t, err, `pq: new row for relation "organizations" violates check constraint "organization_sms_registration_message_template_contains_tags_ch"`)
	})
	t.Run("updating sms_registration_message_template with the tags {{.OrganizationName}} and {{.RegistrationLink}} will succeed 🎉", func(t *testing.T) {
		q := "UPDATE organizations SET sms_registration_message_template = 'TAG1: {{.OrganizationName}} and TAG2: {{.RegistrationLink}}.'"
		_, err := dbConnectionPool.ExecContext(ctx, q)
		require.NoError(t, err)
	})
}

func Test_Organizations_Get(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	organizationModel := &OrganizationModel{dbConnectionPool: dbConnectionPool}

	t.Run("returns the single organization", func(t *testing.T) {
		gotOrganization, err := organizationModel.Get(ctx)
		require.NoError(t, err)

		assert.Len(t, gotOrganization.ID, 36)
		assert.Equal(t, "MyCustomAid", gotOrganization.Name)
		assert.Equal(t, "GDA34JZ26FZY64XCSY46CUNSHLX762LHJXQHWWHGL5HSFRWSGBVHUFNI", gotOrganization.StellarMainAddress)
		assert.Equal(t, "+00:00", gotOrganization.TimezoneUTCOffset)
		assert.False(t, gotOrganization.ArePaymentsEnabled)
		assert.Equal(t, "You have a payment waiting for you from the {{.OrganizationName}}. Click {{.RegistrationLink}} to register.", gotOrganization.SMSRegistrationMessageTemplate)
		assert.NotEmpty(t, gotOrganization.CreatedAt)
		assert.NotEmpty(t, gotOrganization.UpdatedAt)
		assert.False(t, gotOrganization.IsApprovalRequired)
	})
}

func Test_Organizations_ArePaymentsEnabled(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	organizationModel := &OrganizationModel{dbConnectionPool: dbConnectionPool}

	t.Run("returns false if it's not enabled", func(t *testing.T) {
		arePaymentsEnabled, err := organizationModel.ArePaymentsEnabled(ctx)
		require.NoError(t, err)
		require.False(t, arePaymentsEnabled)
	})

	t.Run("returns true if it's enabled", func(t *testing.T) {
		q := "UPDATE organizations SET are_payments_enabled = true"
		_, err := dbConnectionPool.ExecContext(ctx, q)
		require.NoError(t, err)

		arePaymentsEnabled, err := organizationModel.ArePaymentsEnabled(ctx)
		require.NoError(t, err)
		require.True(t, arePaymentsEnabled)
	})
}

func Test_OrganizationUpdate_validate(t *testing.T) {
	ou := &OrganizationUpdate{}
	err := ou.validate()
	assert.EqualError(t, err, "name, timezone UTC offset, approval workflow flag or logo is required")

	ou.Name = "My Org Name"
	err = ou.validate()
	assert.Nil(t, err)

	// png
	img := CreateMockImage(t, 300, 300, ImageSizeSmall)
	buf := new(bytes.Buffer)
	err = png.Encode(buf, img)
	require.NoError(t, err)

	ou.Name = ""
	ou.Logo = buf.Bytes()
	err = ou.validate()
	assert.Nil(t, err)

	// jpeg
	img = CreateMockImage(t, 300, 300, ImageSizeSmall)
	buf = new(bytes.Buffer)
	err = jpeg.Encode(buf, img, &jpeg.Options{Quality: jpeg.DefaultQuality})
	require.NoError(t, err)

	ou.Name = ""
	ou.Logo = buf.Bytes()
	err = ou.validate()
	assert.Nil(t, err)

	ou.Name = "My Org Name"
	ou.Logo = buf.Bytes()
	err = ou.validate()
	assert.Nil(t, err)

	// error decoding image
	csvBuf := new(bytes.Buffer)
	csvWriter := csv.NewWriter(csvBuf)
	err = csvWriter.WriteAll([][]string{
		{"name", "age"},
		{"foo", "99"},
		{"bar", "99"},
	})
	require.NoError(t, err)

	ou.Logo = csvBuf.Bytes()
	err = ou.validate()
	assert.EqualError(t, err, "error decoding image bytes: image: unknown format")

	// invalid image type
	img = CreateMockImage(t, 300, 300, ImageSizeSmall)
	buf = new(bytes.Buffer)
	err = gif.Encode(buf, img, &gif.Options{})
	require.NoError(t, err)

	ou.Logo = buf.Bytes()
	err = ou.validate()
	assert.EqualError(t, err, "invalid image type provided. Expect png or jpeg")

	// timezone UTC offset
	ou = &OrganizationUpdate{}

	ou.TimezoneUTCOffset = "0"
	err = ou.validate()
	assert.EqualError(t, err, "invalid timezone UTC offset format. Example: +02:00 or -03:00")

	ou.TimezoneUTCOffset = "+0"
	err = ou.validate()
	assert.EqualError(t, err, "invalid timezone UTC offset format. Example: +02:00 or -03:00")

	ou.TimezoneUTCOffset = "-5:00"
	err = ou.validate()
	assert.EqualError(t, err, "invalid timezone UTC offset format. Example: +02:00 or -03:00")

	ou.TimezoneUTCOffset = "-5:0"
	err = ou.validate()
	assert.EqualError(t, err, "invalid timezone UTC offset format. Example: +02:00 or -03:00")

	ou.TimezoneUTCOffset = "+03:01515515151551515"
	err = ou.validate()
	assert.EqualError(t, err, "invalid timezone UTC offset format. Example: +02:00 or -03:00")

	ou.TimezoneUTCOffset = "03:00"
	err = ou.validate()
	assert.EqualError(t, err, "invalid timezone UTC offset format. Example: +02:00 or -03:00")

	ou.TimezoneUTCOffset = "+05:00"
	err = ou.validate()
	assert.Nil(t, err)

	ou.TimezoneUTCOffset = "-02:00"
	err = ou.validate()
	assert.Nil(t, err)
}

func Test_Organizations_Update(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	resetOrganizationInfo := func(t *testing.T, ctx context.Context) {
		const q = "UPDATE organizations SET name = 'MyCustomAid', logo = NULL, timezone_utc_offset = '+00:00'"
		_, err := dbConnectionPool.ExecContext(ctx, q)
		require.NoError(t, err)
	}

	organizationModel := &OrganizationModel{dbConnectionPool: dbConnectionPool}

	t.Run("returns error with invalid OrganizationUpdate", func(t *testing.T) {
		ou := &OrganizationUpdate{}
		err := organizationModel.Update(ctx, ou)
		assert.EqualError(t, err, "invalid organization update: name, timezone UTC offset, approval workflow flag or logo is required")
	})

	t.Run("updates only organization's name successfully", func(t *testing.T) {
		resetOrganizationInfo(t, ctx)

		o, err := organizationModel.Get(ctx)
		require.NoError(t, err)

		assert.Equal(t, "MyCustomAid", o.Name)
		assert.Equal(t, "+00:00", o.TimezoneUTCOffset)
		assert.Nil(t, o.Logo)

		ou := &OrganizationUpdate{Name: "My Org Name"}

		err = organizationModel.Update(ctx, ou)
		require.NoError(t, err)

		o, err = organizationModel.Get(ctx)
		require.NoError(t, err)

		assert.Equal(t, "My Org Name", o.Name)
		assert.Equal(t, "+00:00", o.TimezoneUTCOffset)
		assert.Nil(t, o.Logo)
	})

	t.Run("updates only organization's timezone UTC offset successfully", func(t *testing.T) {
		resetOrganizationInfo(t, ctx)

		o, err := organizationModel.Get(ctx)
		require.NoError(t, err)

		assert.Equal(t, "+00:00", o.TimezoneUTCOffset)
		assert.Equal(t, "MyCustomAid", o.Name)
		assert.Nil(t, o.Logo)

		ou := &OrganizationUpdate{TimezoneUTCOffset: "+02:00"}

		err = organizationModel.Update(ctx, ou)
		require.NoError(t, err)

		o, err = organizationModel.Get(ctx)
		require.NoError(t, err)

		assert.Equal(t, "+02:00", o.TimezoneUTCOffset)
		assert.Equal(t, "MyCustomAid", o.Name)
		assert.Nil(t, o.Logo)
	})

	t.Run("updates only organization's logo successfully", func(t *testing.T) {
		resetOrganizationInfo(t, ctx)

		o, err := organizationModel.Get(ctx)
		require.NoError(t, err)

		assert.Equal(t, "MyCustomAid", o.Name)
		assert.Nil(t, o.Logo)

		img := CreateMockImage(t, 300, 300, ImageSizeSmall)
		buf := new(bytes.Buffer)
		err = png.Encode(buf, img)
		require.NoError(t, err)

		ou := &OrganizationUpdate{Logo: buf.Bytes()}

		err = organizationModel.Update(ctx, ou)
		require.NoError(t, err)

		o, err = organizationModel.Get(ctx)
		require.NoError(t, err)

		assert.Equal(t, "MyCustomAid", o.Name)
		assert.Equal(t, ou.Logo, o.Logo)
	})

	t.Run("updates only organization's is_approval_required successfully", func(t *testing.T) {
		resetOrganizationInfo(t, ctx)

		o, err := organizationModel.Get(ctx)
		require.NoError(t, err)
		require.False(t, o.IsApprovalRequired)

		isApprovalRequired := true
		ou := &OrganizationUpdate{IsApprovalRequired: &isApprovalRequired}

		err = organizationModel.Update(ctx, ou)
		require.NoError(t, err)

		o, err = organizationModel.Get(ctx)
		require.NoError(t, err)
		require.True(t, o.IsApprovalRequired)
	})

	t.Run("updates organization's name, timezone UTC offset and logo successfully", func(t *testing.T) {
		resetOrganizationInfo(t, ctx)

		o, err := organizationModel.Get(ctx)
		require.NoError(t, err)

		assert.Equal(t, "MyCustomAid", o.Name)
		assert.Equal(t, "+00:00", o.TimezoneUTCOffset)
		assert.Nil(t, o.Logo)

		img := CreateMockImage(t, 300, 300, ImageSizeSmall)
		buf := new(bytes.Buffer)
		err = png.Encode(buf, img)
		require.NoError(t, err)

		ou := &OrganizationUpdate{Name: "My Org Name", Logo: buf.Bytes(), TimezoneUTCOffset: "+02:00"}

		err = organizationModel.Update(ctx, ou)
		require.NoError(t, err)

		o, err = organizationModel.Get(ctx)
		require.NoError(t, err)

		assert.Equal(t, "My Org Name", o.Name)
		assert.Equal(t, "+02:00", o.TimezoneUTCOffset)
		assert.Equal(t, ou.Logo, o.Logo)
	})
}
