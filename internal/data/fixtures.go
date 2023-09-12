package data

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/csv"
	"fmt"
	"image"
	"image/color"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/stellar/go/keypair"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"

	"github.com/stretchr/testify/require"
)

const (
	FixtureCountryUSA = "USA"
	FixtureCountryUKR = "UKR"
	FixtureAssetUSDC  = "USDC"
)

func CreateAssetFixture(t *testing.T, ctx context.Context, sqlExec db.SQLExecuter, code, issuer string) *Asset {
	issuerAddress := issuer

	if issuerAddress == "" && strings.ToUpper(code) != "XLM" {
		issuer, err := utils.RandomString(56)
		require.NoError(t, err)
		issuerAddress = issuer
	}

	const query = `
		INSERT INTO assets
			(code, issuer)
		VALUES
			($1, $2)
		RETURNING
			id, created_at, updated_at
	`

	asset := &Asset{
		Code:   code,
		Issuer: issuerAddress,
	}

	err := sqlExec.QueryRowxContext(ctx, query, asset.Code, asset.Issuer).Scan(&asset.ID, &asset.CreatedAt, &asset.UpdatedAt)
	require.NoError(t, err)

	return asset
}

func GetAssetFixture(t *testing.T, ctx context.Context, sqlExec db.SQLExecuter, code string) *Asset {
	const query = `
		SELECT
			*
		FROM
			assets a
		WHERE
			a.code = $1
	`

	asset := &Asset{}
	err := sqlExec.GetContext(ctx, asset, query, code)
	require.NoError(t, err)

	return asset
}

// AssociateAssetWithWalletFixture associates an asset with a wallet
func AssociateAssetWithWalletFixture(t *testing.T, ctx context.Context, sqlExec db.SQLExecuter, assetID, walletID string) {
	const query = `
		INSERT INTO wallets_assets
			(wallet_id, asset_id)
		VALUES
			($1, $2)
	`

	_, err := sqlExec.ExecContext(ctx, query, walletID, assetID)
	require.NoError(t, err)
}

// DeleteAllAssetFixtures deletes all assets in the database
func DeleteAllAssetFixtures(t *testing.T, ctx context.Context, sqlExec db.SQLExecuter) {
	const query = "DELETE FROM assets"
	_, err := sqlExec.ExecContext(ctx, query)
	require.NoError(t, err)
}

// ClearAndCreateAssetFixtures deletes all assets in the database then creates new assets for testing
func ClearAndCreateAssetFixtures(t *testing.T, ctx context.Context, sqlExec db.SQLExecuter) []Asset {
	DeleteAllAssetFixtures(t, ctx, sqlExec)
	expected := []Asset{
		*CreateAssetFixture(t, ctx, sqlExec, "EURT", "GA62MH5RDXFWAIWHQEFNMO2SVDDCQLWOO3GO36VQB5LHUXL22DQ6IQAU"),
		*CreateAssetFixture(t, ctx, sqlExec, "USDC", "GABC65XJDMXTGPNZRCI6V3KOKKWVK55UEKGQLONRIVYPMEJNNQ45YOEE"),
	}
	return expected
}

func CreateDefaultWalletFixture(t *testing.T, ctx context.Context, sqlExec db.SQLExecuter) *Wallet {
	return CreateWalletFixture(t, ctx, sqlExec, "Demo Wallet",
		"https://demo-wallet.stellar.org",
		"https://demo-wallet.stellar.org",
		"demo-wallet-server.stellar.org")
}

func CreateWalletFixture(t *testing.T, ctx context.Context, sqlExec db.SQLExecuter, name, homepage, sep10ClientDomain, deepLinkSchema string) *Wallet {
	const query = `
		INSERT INTO wallets
			(name, homepage, sep_10_client_domain, deep_link_schema)
		VALUES
			($1, $2, $3, $4)
		ON CONFLICT DO NOTHING
		RETURNING
			id, created_at, updated_at
		
	`

	_, err := sqlExec.ExecContext(ctx, query, name, homepage, sep10ClientDomain, deepLinkSchema)
	require.NoError(t, err)

	return GetWalletFixture(t, ctx, sqlExec, name)
}

func CreateWalletAssets(t *testing.T, ctx context.Context, sqlExec db.SQLExecuter, walletID string, assetsIDs []string) []Asset {
	const query = `
		WITH assets_cte AS (
			SELECT UNNEST($1::text[]) AS asset_id
		)
		INSERT INTO wallets_assets
			(wallet_id, asset_id)
		SELECT
			$2, a.asset_id
		FROM
			assets_cte a
	`

	_, err := sqlExec.ExecContext(ctx, query, pq.Array(assetsIDs), walletID)
	require.NoError(t, err)

	return GetWalletAssetsFixture(t, ctx, sqlExec, walletID)
}

func GetWalletFixture(t *testing.T, ctx context.Context, sqlExec db.SQLExecuter, name string) *Wallet {
	const query = `
		SELECT
			w.*,
			jsonb_agg(
				DISTINCT to_jsonb(a)
			) FILTER (WHERE a.id IS NOT NULL) AS assets
		FROM
			wallets w
			LEFT JOIN wallets_assets wa ON w.id = wa.wallet_id
			LEFT JOIN assets a ON a.id = wa.asset_id
		WHERE 
		    w.name = $1
		GROUP BY
			w.id
	`

	wallet := &Wallet{}
	err := sqlExec.GetContext(ctx, wallet, query, name)
	require.NoError(t, err)

	return wallet
}

func GetWalletAssetsFixture(t *testing.T, ctx context.Context, sqlExec db.SQLExecuter, walletID string) []Asset {
	const query = `
		SELECT
			a.*
		FROM
			wallets_assets wa
			INNER JOIN assets a ON a.id = wa.asset_id
		WHERE
			wa.wallet_id = $1
		ORDER BY
			code
	`

	assets := make([]Asset, 0)
	err := sqlExec.SelectContext(ctx, &assets, query, walletID)
	require.NoError(t, err)

	return assets
}

// DeleteAllWalletFixtures deletes all wallets in the database
func DeleteAllWalletFixtures(t *testing.T, ctx context.Context, sqlExec db.SQLExecuter) {
	query := "DELETE FROM wallets_assets"
	_, err := sqlExec.ExecContext(ctx, query)
	require.NoError(t, err)

	query = "DELETE FROM wallets"
	_, err = sqlExec.ExecContext(ctx, query)
	require.NoError(t, err)
}

// ClearAndCreateWalletFixtures deletes all wallets in the database then creates new wallets for testing
func ClearAndCreateWalletFixtures(t *testing.T, ctx context.Context, sqlExec db.SQLExecuter) []Wallet {
	DeleteAllWalletFixtures(t, ctx, sqlExec)
	expected := []Wallet{
		*CreateWalletFixture(t, ctx, sqlExec, "BOSS Money", "https://www.walletbyboss.com", "www.walletbyboss.com", "https://www.walletbyboss.com"),
		*CreateWalletFixture(t, ctx, sqlExec, "Vibrant Assist", "https://vibrantapp.com", "vibrantapp.com", "vibrantapp://"),
	}
	return expected
}

func GetCountryFixture(t *testing.T, ctx context.Context, sqlExec db.SQLExecuter, code string) *Country {
	const query = `
		SELECT
			*
		FROM
			countries
		WHERE
			code = $1
	`

	country := &Country{}
	err := sqlExec.GetContext(ctx, country, query, code)
	require.NoError(t, err)

	return country
}

func CreateCountryFixture(t *testing.T, ctx context.Context, sqlExec db.SQLExecuter, code, name string) *Country {
	const query = `
		WITH create_country AS (
			INSERT INTO countries
				(code, name)
			VALUES
				($1, $2)
			ON CONFLICT DO NOTHING
			RETURNING *
		)
		SELECT created_at, updated_at FROM create_country
		UNION ALL
		SELECT created_at, updated_at FROM countries WHERE code = $1 AND name = $2
	`

	country := &Country{
		Code: code,
		Name: name,
	}

	err := sqlExec.QueryRowxContext(ctx, query, code, name).Scan(&country.CreatedAt, &country.UpdatedAt)
	require.NoError(t, err)

	return country
}

// DeleteAllCountryFixtures deletes all countries in the database
func DeleteAllCountryFixtures(t *testing.T, ctx context.Context, sqlExec db.SQLExecuter) {
	const query = "DELETE FROM countries"
	_, err := sqlExec.ExecContext(ctx, query)
	require.NoError(t, err)
}

// ClearAndCreateCountryFixtures deletes all countries in the database then creates new countries for testing
func ClearAndCreateCountryFixtures(t *testing.T, ctx context.Context, sqlExec db.SQLExecuter) []Country {
	DeleteAllCountryFixtures(t, ctx, sqlExec)
	expected := []Country{
		*CreateCountryFixture(t, ctx, sqlExec, "BRA", "Brazil"),
		*CreateCountryFixture(t, ctx, sqlExec, "UKR", "Ukraine"),
	}
	return expected
}

func CreateReceiverFixture(t *testing.T, ctx context.Context, sqlExec db.SQLExecuter, r *Receiver) *Receiver {
	randomSuffix, err := utils.RandomString(5)
	require.NoError(t, err)

	if r.Email == nil {
		email := fmt.Sprintf("email%s@randomemail.com", randomSuffix)
		r.Email = &email
	}

	if r.PhoneNumber == "" {
		r.PhoneNumber = "+141555" + randomSuffix
	}

	if r.ExternalID == "" {
		r.ExternalID, err = utils.RandomString(56)
		require.NoError(t, err)
	}

	if r.CreatedAt == nil {
		now := time.Now()
		r.CreatedAt = &now
	}

	if r.UpdatedAt == nil {
		now := time.Now()
		r.UpdatedAt = &now
	}

	const query = `
		INSERT INTO receivers
			(email, phone_number, external_id, created_at, updated_at)
		VALUES
			($1, $2, $3, $4, $5)
		RETURNING
			id, email, phone_number, external_id, created_at, updated_at
	`

	var receiver Receiver
	err = sqlExec.QueryRowxContext(ctx, query, r.Email, r.PhoneNumber, r.ExternalID, r.CreatedAt, r.UpdatedAt).Scan(
		&receiver.ID,
		&receiver.Email,
		&receiver.PhoneNumber,
		&receiver.ExternalID,
		&receiver.CreatedAt,
		&receiver.UpdatedAt,
	)
	require.NoError(t, err)

	return &receiver
}

func DeleteAllReceiversFixtures(t *testing.T, ctx context.Context, sqlExec db.SQLExecuter) {
	const query = "DELETE FROM receivers"
	_, err := sqlExec.ExecContext(ctx, query)
	require.NoError(t, err)
}

func CreateReceiverVerificationFixture(t *testing.T, ctx context.Context, sqlExec db.SQLExecuter, insert ReceiverVerificationInsert) *ReceiverVerification {
	const query = `
		INSERT INTO receiver_verifications
			(receiver_id, verification_field, hashed_value)
		VALUES
			($1, $2, $3)
		RETURNING
			receiver_id, verification_field, hashed_value, attempts, created_at, confirmed_at, updated_at, failed_at
	`

	var verification ReceiverVerification
	verificationValue, err := HashVerificationValue(insert.VerificationValue)
	require.NoError(t, err)

	err = sqlExec.GetContext(ctx, &verification, query, insert.ReceiverID, insert.VerificationField, verificationValue)
	require.NoError(t, err)

	return &verification
}

func DeleteAllReceiverVerificationFixtures(t *testing.T, ctx context.Context, sqlExec db.SQLExecuter) {
	const query = "DELETE FROM receiver_verifications"
	_, err := sqlExec.ExecContext(ctx, query)
	require.NoError(t, err)
}

func CreateReceiverWalletFixture(t *testing.T, ctx context.Context, sqlExec db.SQLExecuter, receiverID, walletID string, status ReceiversWalletStatus) *ReceiverWallet {
	kp, err := keypair.Random()
	require.NoError(t, err)
	stellarAddress := kp.Address()

	randNumber, err := rand.Int(rand.Reader, big.NewInt(90000))
	require.NoError(t, err)

	stellarMemo := fmt.Sprint(randNumber.Int64() + 10000)
	stellarMemoType := "id"

	const query = `
		WITH inserted_receiver_wallet AS (
			INSERT INTO receiver_wallets
				(receiver_id, wallet_id, stellar_address, stellar_memo, stellar_memo_type, status)
			VALUES
				($1, $2, $3, $4, $5, $6)
			RETURNING
				id, receiver_id, wallet_id, stellar_address, stellar_memo, stellar_memo_type, status, status_history, created_at, updated_at
		)
		SELECT
			rw.id, rw.stellar_address, rw.stellar_memo, rw.stellar_memo_type, rw.status, rw.status_history, rw.created_at, rw.updated_at,
			r.id, r.email, r.phone_number, r.external_id, r.created_at, r.updated_at,
			w.id, w.name, w.homepage, w.deep_link_schema, w.created_at, w.updated_at
		FROM
			inserted_receiver_wallet AS rw
			JOIN receivers AS r ON rw.receiver_id = r.id
			JOIN wallets AS w ON rw.wallet_id = w.id
	`

	var statusHistoryJSON pq.ByteaArray
	var receiverWallet ReceiverWallet
	err = sqlExec.QueryRowxContext(ctx, query, receiverID, walletID, stellarAddress, stellarMemo, stellarMemoType, status).Scan(
		&receiverWallet.ID,
		&receiverWallet.StellarAddress,
		&receiverWallet.StellarMemo,
		&receiverWallet.StellarMemoType,
		&receiverWallet.Status,
		&statusHistoryJSON,
		&receiverWallet.CreatedAt,
		&receiverWallet.UpdatedAt,
		&receiverWallet.Receiver.ID,
		&receiverWallet.Receiver.Email,
		&receiverWallet.Receiver.PhoneNumber,
		&receiverWallet.Receiver.ExternalID,
		&receiverWallet.Receiver.CreatedAt,
		&receiverWallet.Receiver.UpdatedAt,
		&receiverWallet.Wallet.ID,
		&receiverWallet.Wallet.Name,
		&receiverWallet.Wallet.Homepage,
		&receiverWallet.Wallet.DeepLinkSchema,
		&receiverWallet.Wallet.CreatedAt,
		&receiverWallet.Wallet.UpdatedAt,
	)
	require.NoError(t, err)

	err = receiverWallet.statusHistoryFromByteArray(statusHistoryJSON)
	require.NoError(t, err)

	return &receiverWallet
}

func DeleteAllReceiverWalletsFixtures(t *testing.T, ctx context.Context, sqlExec db.SQLExecuter) {
	const query = `
		DELETE FROM receiver_wallets
	`
	_, err := sqlExec.ExecContext(ctx, query)
	require.NoError(t, err)
}

func CreatePaymentFixture(t *testing.T, ctx context.Context, sqlExec db.SQLExecuter, model *PaymentModel, p *Payment) *Payment {
	if p.StatusHistory == nil {
		p.StatusHistory = []PaymentStatusHistoryEntry{{
			Timestamp:     time.Now(),
			Status:        p.Status,
			StatusMessage: "",
		}}
	}

	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now()
	}

	if p.UpdatedAt.IsZero() {
		p.UpdatedAt = time.Now()
	}

	const query = `
		INSERT INTO payments
			(receiver_id, disbursement_id, receiver_wallet_id, asset_id, amount, status, status_history,
			stellar_transaction_id, stellar_operation_id, created_at, updated_at)
		VALUES
			($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING
			id
	`
	var newId string
	err := sqlExec.GetContext(ctx, &newId, query,
		p.ReceiverWallet.Receiver.ID,
		p.Disbursement.ID,
		p.ReceiverWallet.ID,
		p.Asset.ID,
		p.Amount,
		p.Status,
		p.StatusHistory,
		p.StellarTransactionID,
		p.StellarOperationID,
		p.CreatedAt,
		p.UpdatedAt,
	)
	require.NoError(t, err)

	// get payment
	payment, err := model.Get(ctx, newId, sqlExec)
	require.NoError(t, err)
	return payment
}

func DeleteAllPaymentsFixtures(t *testing.T, ctx context.Context, sqlExec db.SQLExecuter) {
	const query = "DELETE FROM payments"
	_, err := sqlExec.ExecContext(ctx, query)
	require.NoError(t, err)
}

func CreateDisbursementFixture(t *testing.T, ctx context.Context, sqlExec db.SQLExecuter, model *DisbursementModel, d *Disbursement) *Disbursement {
	if d == nil {
		d = &Disbursement{}
	}
	if d.Name == "" {
		randomName, err := utils.RandomString(10)
		require.NoError(t, err)
		d.Name = randomName
	}
	if d.Status == "" {
		d.Status = DraftDisbursementStatus
	}
	if d.Wallet == nil {
		d.Wallet = CreateDefaultWalletFixture(t, ctx, sqlExec)
	}
	if d.Asset == nil {
		d.Asset = GetAssetFixture(t, ctx, sqlExec, FixtureAssetUSDC)
	}
	if d.Country == nil {
		d.Country = GetCountryFixture(t, ctx, sqlExec, FixtureCountryUKR)
	}
	// insert disbursement
	if d.StatusHistory == nil {
		d.StatusHistory = []DisbursementStatusHistoryEntry{{
			Timestamp: time.Now(),
			Status:    d.Status,
		}}
	}
	id, err := model.Insert(ctx, d)
	require.NoError(t, err)

	// update created_at
	const query = `
		UPDATE disbursements
		SET created_at = $1
		WHERE id = $2
		`
	_, err = sqlExec.ExecContext(ctx, query, d.CreatedAt, id)
	require.NoError(t, err)

	// get disbursement
	disbursement, err := model.Get(ctx, model.dbConnectionPool, id)
	require.NoError(t, err)
	return disbursement
}

func UpdateDisbursementInstructionsFixture(t *testing.T, ctx context.Context, sqlExec db.SQLExecuter, disbursementID, fileName string, instructions []*DisbursementInstruction) {
	fileContent := CreateInstructionsFixture(t, instructions)

	const query = `
		UPDATE disbursements
		SET file_name = $1, file_content = $2
		WHERE id = $3
	`
	_, err := sqlExec.ExecContext(ctx, query, fileName, fileContent, disbursementID)
	require.NoError(t, err)
}

func CreateInstructionsFixture(t *testing.T, instructions []*DisbursementInstruction) []byte {
	// phone,id,amount,verification
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	// write header
	outerErr := writer.Write([]string{"phone", "id", "amount", "verification_value"})
	require.NoError(t, outerErr)

	// write instructions
	for _, instruction := range instructions {
		record := []string{instruction.Phone, instruction.ID, instruction.Amount, instruction.VerificationValue}
		err := writer.Write(record)
		require.NoError(t, err)
	}
	writer.Flush()
	return buf.Bytes()
}

func CreateDraftDisbursementFixture(t *testing.T, ctx context.Context, sqlExec db.SQLExecuter, model *DisbursementModel, insert Disbursement) *Disbursement {
	if insert.StatusHistory == nil {
		insert.StatusHistory = []DisbursementStatusHistoryEntry{{
			Timestamp: time.Now(),
			Status:    DraftDisbursementStatus,
			UserID:    "user1",
		}}
	}

	if insert.Status == "" {
		insert.Status = DraftDisbursementStatus
	}

	id, err := model.Insert(ctx, &insert)
	require.NoError(t, err)

	// get disbursement
	disbursement, err := model.Get(ctx, sqlExec, id)
	require.NoError(t, err)
	return disbursement
}

func DeleteAllDisbursementFixtures(t *testing.T, ctx context.Context, sqlExec db.SQLExecuter) {
	const query = "DELETE FROM disbursements"
	_, err := sqlExec.ExecContext(ctx, query)
	require.NoError(t, err)
}

func CreateMessageFixture(t *testing.T, ctx context.Context, sqlExec db.SQLExecuter, m *Message) *Message {
	if m.TextEncrypted == "" {
		m.TextEncrypted = "text encrypted"
	}

	if m.TitleEncrypted == "" {
		m.TitleEncrypted = "title encrypted"
	}

	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now().UTC()
	}

	const query = `
		INSERT INTO messages
			(
				type, asset_id, receiver_id, wallet_id, receiver_wallet_id,
				text_encrypted, title_encrypted, status, created_at, updated_at
			)
		VALUES
			($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING
			id, status_history
	`

	err := sqlExec.QueryRowxContext(ctx, query, m.Type, m.AssetID, m.ReceiverID, m.WalletID, m.ReceiverWalletID, m.TextEncrypted, m.TitleEncrypted, m.Status, m.CreatedAt, m.UpdatedAt).Scan(
		&m.ID,
		&m.StatusHistory,
	)
	require.NoError(t, err)

	return m
}

// EnableDisbursementApproval enables disbursement workflow approval for the given organization.
func EnableDisbursementApproval(t *testing.T, ctx context.Context, orgModel *OrganizationModel) {
	isApprovalRequired := true
	err := orgModel.Update(ctx, &OrganizationUpdate{IsApprovalRequired: &isApprovalRequired})
	require.NoError(t, err)
}

// DisableDisbursementApproval disables disbursement workflow approval for the given organization.
func DisableDisbursementApproval(t *testing.T, ctx context.Context, orgModel *OrganizationModel) {
	isApprovalRequired := false
	err := orgModel.Update(ctx, &OrganizationUpdate{IsApprovalRequired: &isApprovalRequired})
	require.NoError(t, err)
}

func DeleteAllMessagesFixtures(t *testing.T, ctx context.Context, sqlExec db.SQLExecuter) {
	const query = `
		DELETE FROM messages
	`
	_, err := sqlExec.ExecContext(ctx, query)
	require.NoError(t, err)
}

type ImageSize int

const (
	ImageSizeSmall ImageSize = iota
	ImageSizeMedium
	ImageSizeLarge
)

/*
CreateMockImage creates an RGBA image with the given proportion and size.
The size is defined by how many different colors are drawn in the image,
so the compression format (jpeg or png) will generate a larger file since
the image will have more complexity. Note: Depending on the compression format
the image size may vary.

Example creating a file:

	img := CreateMockImage(t, 3840, 2160, ImageSizeLarge)
	f, err := os.Create("image.png")
	require.NoError(t, err)
	err = jpeg.Encode(f, img, &jpeg.Options{Quality: jpeg.DefaultQuality}
	require.NoError(t, err)

Example in memory image:

	img := CreateMockImage(t, 1920, 1080, ImageSizeMedium)
	buf := new(bytes.Buffer)
	err = png.Encode(buf, img)
	require.NoError(t, err)
	fmt.Println(img.Bytes())
*/
func CreateMockImage(t *testing.T, width, height int, size ImageSize) image.Image {
	imgRect := image.Rect(0, 0, width, height)
	img := image.NewRGBA(imgRect)

	bigInt := big.NewInt(255)

	// sets a random color for every pixel. It increase the compression complexity.
	largeImageColor := func() color.Color {
		r, err := rand.Int(rand.Reader, bigInt)
		require.NoError(t, err)

		g, err := rand.Int(rand.Reader, bigInt)
		require.NoError(t, err)

		b, err := rand.Int(rand.Reader, bigInt)
		require.NoError(t, err)

		return color.RGBA{uint8(r.Int64()), uint8(g.Int64()), uint8(b.Int64()), 255}
	}

	// sets the same color for each line. It's less complex than the largeImageColor.
	mediumImageColor := func() color.Color {
		n, err := rand.Int(rand.Reader, bigInt)
		require.NoError(t, err)

		return color.RGBA{uint8(n.Int64()), uint8(n.Int64()), uint8(n.Int64()), 255}
	}

	// sets the same color for the entire image. No complexity.
	smallImageColor := func() color.Color {
		// returns the cyan color
		return color.RGBA{100, 200, 200, 0xff}
	}

	var c color.Color
	for x := 0; x < width; x++ {
		if size == ImageSizeMedium {
			c = mediumImageColor()
		}

		for y := 0; y < height; y++ {
			switch size {
			case ImageSizeSmall:
				c = smallImageColor()
			case ImageSizeLarge:
				c = largeImageColor()
			}

			img.Set(x, y, c)
		}
	}

	return img
}
