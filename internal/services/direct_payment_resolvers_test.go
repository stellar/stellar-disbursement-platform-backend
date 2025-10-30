package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/testutils"
)

func TestAssetResolver_Validate(t *testing.T) {
	tests := []struct {
		name          string
		ref           AssetReference
		expectedError error
	}{
		{
			name: "no reference provided",
			ref:  AssetReference{},
			expectedError: ValidationError{
				EntityType: EntityTypeAsset,
				Field:      FieldReference,
				Message:    "must be specified by id or type",
			},
		},
		{
			name: "both id and type provided",
			ref: AssetReference{
				ID:   testutils.StringPtr("asset-id"),
				Type: testutils.StringPtr(AssetTypeNative),
			},
			expectedError: ValidationError{
				EntityType: EntityTypeAsset,
				Field:      FieldReference,
				Message:    "must be specified by either id or type, not both",
			},
		},
		{
			name: "invalid asset type",
			ref: AssetReference{
				Type: testutils.StringPtr("chaos"),
			},
			expectedError: ValidationError{
				EntityType: EntityTypeAsset,
				Field:      FieldType,
				Message:    "invalid type 'chaos', must be one of: native, classic, contract, fiat",
			},
		},
		{
			name: "classic asset missing code",
			ref: AssetReference{
				Type:   testutils.StringPtr(AssetTypeClassic),
				Issuer: testutils.StringPtr("GISSUER..."),
			},
			expectedError: ValidationError{
				EntityType: EntityTypeAsset,
				Field:      FieldCode,
				Message:    "required for classic asset",
			},
		},
		{
			name: "classic asset missing issuer",
			ref: AssetReference{
				Type: testutils.StringPtr(AssetTypeClassic),
				Code: testutils.StringPtr("THRONE"),
			},
			expectedError: ValidationError{
				EntityType: EntityTypeAsset,
				Field:      FieldIssuer,
				Message:    "required for classic asset",
			},
		},
		{
			name: "contract asset missing contract_id",
			ref: AssetReference{
				Type: testutils.StringPtr(AssetTypeContract),
			},
			expectedError: ValidationError{
				EntityType: EntityTypeAsset,
				Field:      FieldContractID,
				Message:    "required for contract asset",
			},
		},
		{
			name: "fiat asset missing code",
			ref: AssetReference{
				Type: testutils.StringPtr(AssetTypeFiat),
			},
			expectedError: ValidationError{
				EntityType: EntityTypeAsset,
				Field:      FieldCode,
				Message:    "required for fiat asset",
			},
		},
		{
			name: "valid id reference",
			ref: AssetReference{
				ID: testutils.StringPtr("asset-throne-gelt"),
			},
		},
		{
			name: "valid native reference",
			ref: AssetReference{
				Type: testutils.StringPtr(AssetTypeNative),
			},
		},
		{
			name: "valid classic reference",
			ref: AssetReference{
				Type:   testutils.StringPtr(AssetTypeClassic),
				Code:   testutils.StringPtr("THRONE"),
				Issuer: testutils.StringPtr("GISSUER1234567890ABCDEFGHIJKLMNOPQRSTUVWXYZ123456"),
			},
		},
		{
			name: "invalid contract reference",
			ref: AssetReference{
				Type:       testutils.StringPtr(AssetTypeContract),
				ContractID: testutils.StringPtr("CONTRACT123"),
			},
			expectedError: ValidationError{
				EntityType: EntityTypeAsset,
				Field:      FieldContractID,
				Message:    "invalid contract format provided",
			},
		},
		{
			name: "valid fiat reference",
			ref: AssetReference{
				Type: testutils.StringPtr(AssetTypeFiat),
				Code: testutils.StringPtr("USD"),
			},
		},
		{
			name: "empty string id",
			ref: AssetReference{
				ID: testutils.StringPtr(""),
			},
			expectedError: ValidationError{
				EntityType: EntityTypeAsset,
				Field:      FieldReference,
				Message:    "must be specified by id or type",
			},
		},
		{
			name: "whitespace only id",
			ref: AssetReference{
				ID: testutils.StringPtr("   "),
			},
			expectedError: ValidationError{
				EntityType: EntityTypeAsset,
				Field:      FieldReference,
				Message:    "must be specified by id or type",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resolver := &AssetResolver{}
			err := resolver.Validate(tc.ref)

			if tc.expectedError != nil {
				require.Error(t, err)
				require.Equal(t, tc.expectedError, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestAssetResolver_Resolve(t *testing.T) {
	dbConnectionPool := testutils.GetDBConnectionPool(t)

	ctx := context.Background()
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	xlm := data.CreateAssetFixture(t, ctx, dbConnectionPool, "XLM", "")
	trc := data.CreateAssetFixture(t, ctx, dbConnectionPool, "TRC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")

	resolver := NewAssetResolver(models)

	tests := []struct {
		name          string
		ref           AssetReference
		wantAsset     *data.Asset
		expectedError error
	}{
		{
			name: "resolve by ID",
			ref: AssetReference{
				ID: &trc.ID,
			},
			wantAsset: trc,
		},
		{
			name: "resolve native asset",
			ref: AssetReference{
				Type: testutils.StringPtr(AssetTypeNative),
			},
			wantAsset: xlm,
		},
		{
			name: "resolve classic asset",
			ref: AssetReference{
				Type:   testutils.StringPtr(AssetTypeClassic),
				Code:   &trc.Code,
				Issuer: &trc.Issuer,
			},
			wantAsset: trc,
		},
		{
			name: "non-existent ID",
			ref: AssetReference{
				ID: testutils.StringPtr("non-existent"),
			},
			expectedError: NotFoundError{
				EntityType: EntityTypeAsset,
				Reference:  "non-existent",
			},
		},
		{
			name: "non-existent classic asset",
			ref: AssetReference{
				Type:   testutils.StringPtr(AssetTypeClassic),
				Code:   testutils.StringPtr("CHAOS"),
				Issuer: testutils.StringPtr("GCHAOS1234567890ABCDEFGHIJKLMNOPQRSTUVWXYZ123456"),
			},
			expectedError: NotFoundError{
				EntityType: EntityTypeAsset,
				Reference:  "CHAOS:GCHAOS1234567890ABCDEFGHIJKLMNOPQRSTUVWXYZ123456",
			},
		},
		{
			name: "contract asset not supported",
			ref: AssetReference{
				Type:       testutils.StringPtr(AssetTypeContract),
				ContractID: testutils.StringPtr("CONTRACT123"),
			},
			expectedError: ValidationError{
				EntityType: EntityTypeAsset,
				Field:      FieldContractID,
				Message:    "invalid contract format provided",
			},
		},
		{
			name: "fiat asset not supported",
			ref: AssetReference{
				Type: testutils.StringPtr(AssetTypeFiat),
				Code: testutils.StringPtr("USD"),
			},
			expectedError: UnsupportedError{
				EntityType: EntityTypeAsset,
				Feature:    "fiat assets",
			},
		},
		{
			name: "invalid reference",
			ref:  AssetReference{},
			expectedError: ValidationError{
				EntityType: EntityTypeAsset,
				Field:      FieldReference,
				Message:    "must be specified by id or type",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			asset, err := resolver.Resolve(ctx, dbConnectionPool, tc.ref)

			if tc.expectedError != nil {
				require.Error(t, err)
				require.Equal(t, tc.expectedError, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.wantAsset.ID, asset.ID)
				assert.Equal(t, tc.wantAsset.Code, asset.Code)
				assert.Equal(t, tc.wantAsset.Issuer, asset.Issuer)
			}
		})
	}
}

func TestReceiverResolver_Validate(t *testing.T) {
	tests := []struct {
		name          string
		ref           ReceiverReference
		expectedError error
	}{
		{
			name: "no reference provided",
			ref:  ReceiverReference{},
			expectedError: ValidationError{
				EntityType: EntityTypeReceiver,
				Field:      FieldReference,
				Message:    "must be specified by id, email, phone_number, or wallet_address",
			},
		},
		{
			name: "multiple references provided",
			ref: ReceiverReference{
				ID:    testutils.StringPtr("id"),
				Email: testutils.StringPtr("magnus@tzeentch.com"),
			},
			expectedError: ValidationError{
				EntityType: EntityTypeReceiver,
				Field:      FieldReference,
				Message:    "must be specified by only one identifier",
			},
		},
		{
			name: "invalid email",
			ref: ReceiverReference{
				Email: testutils.StringPtr("not-an-email"),
			},
			expectedError: ValidationError{
				EntityType: EntityTypeReceiver,
				Field:      FieldEmail,
				Message:    "invalid format: not-an-email",
			},
		},
		{
			name: "invalid phone number",
			ref: ReceiverReference{
				PhoneNumber: testutils.StringPtr("123"),
			},
			expectedError: ValidationError{
				EntityType: EntityTypeReceiver,
				Field:      FieldPhoneNumber,
				Message:    "invalid format: 123",
			},
		},
		{
			name: "invalid wallet address",
			ref: ReceiverReference{
				WalletAddress: testutils.StringPtr("not-stellar-address"),
			},
			expectedError: ValidationError{
				EntityType: EntityTypeReceiver,
				Field:      FieldWalletAddress,
				Message:    "invalid stellar address format: not-stellar-address",
			},
		},
		{
			name: "valid ID reference",
			ref: ReceiverReference{
				ID: testutils.StringPtr("receiver-magnus"),
			},
		},
		{
			name: "valid email reference",
			ref: ReceiverReference{
				Email: testutils.StringPtr("magnus@prospero.imperium"),
			},
		},
		{
			name: "valid phone reference",
			ref: ReceiverReference{
				PhoneNumber: testutils.StringPtr("+41555511111"),
			},
		},
		{
			name: "valid wallet address reference",
			ref: ReceiverReference{
				WalletAddress: testutils.StringPtr("GBXGQJWVLWOYHFLVTKWV5FGHA3LNYY2JQKM7OAJAUEQFU6LPCSEFVXON"),
			},
		},
		{
			name: "valid contract wallet address reference",
			ref: ReceiverReference{
				WalletAddress: testutils.StringPtr("CAMAMZUOULVWFAB3KRROW5ELPUFHSEKPUALORCFBLFX7XBWWUCUJLR53"),
			},
		},
		{
			name: "empty string references",
			ref: ReceiverReference{
				ID: testutils.StringPtr(""),
			},
			expectedError: ValidationError{
				EntityType: EntityTypeReceiver,
				Field:      FieldReference,
				Message:    "must be specified by id, email, phone_number, or wallet_address",
			},
		},
		{
			name: "whitespace only references",
			ref: ReceiverReference{
				Email: testutils.StringPtr("   "),
			},
			expectedError: ValidationError{
				EntityType: EntityTypeReceiver,
				Field:      FieldReference,
				Message:    "must be specified by id, email, phone_number, or wallet_address",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resolver := &ReceiverResolver{}
			err := resolver.Validate(tc.ref)

			if tc.expectedError != nil {
				require.Error(t, err)
				require.Equal(t, tc.expectedError, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestReceiverResolver_Resolve(t *testing.T) {
	dbConnectionPool := testutils.GetDBConnectionPool(t)

	ctx := context.Background()
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
		Email:       "vulkan@nocturne.imperium",
		PhoneNumber: "+41555511111",
	})

	wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "Nocturne Wallet", "https://nocturne.com", "nocturne.com", "nocturne://")
	receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.RegisteredReceiversWalletStatus)

	// Update receiver wallet with stellar address
	stellarAddress := "GBXGQJWVLWOYHFLVTKWV5FGHA3LNYY2JQKM7OAJAUEQFU6LPCSEFVXON"
	err = models.ReceiverWallet.Update(ctx, receiverWallet.ID, data.ReceiverWalletUpdate{
		StellarAddress: stellarAddress,
	}, dbConnectionPool)
	require.NoError(t, err)

	resolver := NewReceiverResolver(models)

	tests := []struct {
		name          string
		ref           ReceiverReference
		wantReceiver  *data.Receiver
		expectedError error
	}{
		{
			name: "resolve by ID",
			ref: ReceiverReference{
				ID: &receiver.ID,
			},
			wantReceiver: receiver,
		},
		{
			name: "resolve by email",
			ref: ReceiverReference{
				Email: &receiver.Email,
			},
			wantReceiver: receiver,
		},
		{
			name: "resolve by phone",
			ref: ReceiverReference{
				PhoneNumber: &receiver.PhoneNumber,
			},
			wantReceiver: receiver,
		},
		{
			name: "resolve by wallet address",
			ref: ReceiverReference{
				WalletAddress: &stellarAddress,
			},
			wantReceiver: receiver,
		},
		{
			name: "non-existent ID",
			ref: ReceiverReference{
				ID: testutils.StringPtr("non-existent"),
			},
			expectedError: NotFoundError{
				EntityType: EntityTypeReceiver,
				Reference:  "non-existent",
			},
		},
		{
			name: "non-existent email",
			ref: ReceiverReference{
				Email: testutils.StringPtr("chaos@warp.imperium"),
			},
			expectedError: NotFoundError{
				EntityType: EntityTypeReceiver,
				Message:    "no receiver found with contact info: chaos@warp.imperium",
			},
		},
		{
			name: "non-existent wallet address",
			ref: ReceiverReference{
				WalletAddress: testutils.StringPtr("GD6VWBXI6NY3AOOR55RLVQ4MNIDSXE5JSAVXUTF35FRRI72LYPI3WL6Z"),
			},
			expectedError: NotFoundError{
				EntityType: EntityTypeReceiver,
				Message:    "no receiver found with wallet address: GD6VWBXI6NY3AOOR55RLVQ4MNIDSXE5JSAVXUTF35FRRI72LYPI3WL6Z",
			},
		},
		{
			name: "invalid reference",
			ref:  ReceiverReference{},
			expectedError: ValidationError{
				EntityType: EntityTypeReceiver,
				Field:      FieldReference,
				Message:    "must be specified by id, email, phone_number, or wallet_address",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resolvedReceiver, err := resolver.Resolve(ctx, dbConnectionPool, tc.ref)

			if tc.expectedError != nil {
				require.Error(t, err)
				require.Equal(t, tc.expectedError, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.wantReceiver.ID, resolvedReceiver.ID)
				assert.Equal(t, tc.wantReceiver.Email, resolvedReceiver.Email)
				assert.Equal(t, tc.wantReceiver.PhoneNumber, resolvedReceiver.PhoneNumber)
			}
		})
	}
}

func TestReceiverResolver_GetContactInfo(t *testing.T) {
	tests := []struct {
		name string
		ref  ReceiverReference
		want string
	}{
		{
			name: "returns email",
			ref: ReceiverReference{
				Email: testutils.StringPtr("konrad@nostramo.imperium"),
			},
			want: "konrad@nostramo.imperium",
		},
		{
			name: "returns phone when no email",
			ref: ReceiverReference{
				PhoneNumber: testutils.StringPtr("+1234567890"),
			},
			want: "+1234567890",
		},
		{
			name: "returns email when both present",
			ref: ReceiverReference{
				Email:       testutils.StringPtr("both@test.com"),
				PhoneNumber: testutils.StringPtr("+1234567890"),
			},
			want: "both@test.com",
		},
		{
			name: "returns empty when neither present",
			ref:  ReceiverReference{},
			want: "",
		},
		{
			name: "returns empty for empty strings",
			ref: ReceiverReference{
				Email:       testutils.StringPtr(""),
				PhoneNumber: testutils.StringPtr(""),
			},
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.ref.GetContactInfo()
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestWalletResolver_Validate(t *testing.T) {
	tests := []struct {
		name          string
		ref           WalletReference
		expectedError error
	}{
		{
			name: "no reference provided",
			ref:  WalletReference{},
			expectedError: ValidationError{
				EntityType: EntityTypeWallet,
				Field:      FieldReference,
				Message:    "must be specified by id or address",
			},
		},
		{
			name: "both id and address provided",
			ref: WalletReference{
				ID:      testutils.StringPtr("wallet-id"),
				Address: testutils.StringPtr("GBXGQJWVLWOYHFLVTKWV5FGHA3LNYY2JQKM7OAJAUEQFU6LPCSEFVXON"),
			},
			expectedError: ValidationError{
				EntityType: EntityTypeWallet,
				Field:      FieldReference,
				Message:    "must be specified by either id or address, not both",
			},
		},
		{
			name: "invalid stellar address",
			ref: WalletReference{
				Address: testutils.StringPtr("not-a-stellar-address"),
			},
			expectedError: ValidationError{
				EntityType: EntityTypeWallet,
				Field:      FieldAddress,
				Message:    "invalid stellar address format: not-a-stellar-address",
			},
		},
		{
			name: "valid ID reference",
			ref: WalletReference{
				ID: testutils.StringPtr("wallet-ultramar"),
			},
		},
		{
			name: "valid address reference",
			ref: WalletReference{
				Address: testutils.StringPtr("GBXGQJWVLWOYHFLVTKWV5FGHA3LNYY2JQKM7OAJAUEQFU6LPCSEFVXON"),
			},
		},
		{
			name: "valid contract address reference",
			ref: WalletReference{
				Address: testutils.StringPtr("CAMAMZUOULVWFAB3KRROW5ELPUFHSEKPUALORCFBLFX7XBWWUCUJLR53"),
			},
		},
		{
			name: "empty string references",
			ref: WalletReference{
				ID: testutils.StringPtr(""),
			},
			expectedError: ValidationError{
				EntityType: EntityTypeWallet,
				Field:      FieldReference,
				Message:    "must be specified by id or address",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resolver := &WalletResolver{}
			err := resolver.Validate(tc.ref)

			if tc.expectedError != nil {
				require.Error(t, err)
				require.Equal(t, tc.expectedError, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestWalletResolver_Resolve(t *testing.T) {
	dbConnectionPool := testutils.GetDBConnectionPool(t)

	ctx := context.Background()
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	// Create test wallets
	managedWallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "Imperium Treasury", "https://terra.gov", "terra.gov", "imperium://")

	// Create a user-managed wallet
	userManagedWallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "User Managed Wallet", "stellar.org", "stellar.org", "stellar://")
	data.MakeWalletUserManaged(t, ctx, dbConnectionPool, userManagedWallet.ID)
	receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
		Email:       "vulkan@nocturne.imperium",
		PhoneNumber: "+41555511111",
	})

	receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, userManagedWallet.ID, data.RegisteredReceiversWalletStatus)

	resolver := NewWalletResolver(models)

	tests := []struct {
		name          string
		ref           WalletReference
		wantWallet    *data.Wallet
		expectedError error
	}{
		{
			name: "resolve by ID",
			ref: WalletReference{
				ID: &managedWallet.ID,
			},
			wantWallet: managedWallet,
		},
		{
			name: "resolve by address returns user-managed wallet",
			ref: WalletReference{
				Address: &receiverWallet.StellarAddress,
			},
			wantWallet: userManagedWallet,
		},
		{
			name: "non-existent ID",
			ref: WalletReference{
				ID: testutils.StringPtr("non-existent"),
			},
			expectedError: NotFoundError{
				EntityType: EntityTypeWallet,
				Reference:  "non-existent",
			},
		},
		{
			name: "invalid reference",
			ref:  WalletReference{},
			expectedError: ValidationError{
				EntityType: EntityTypeWallet,
				Field:      FieldReference,
				Message:    "must be specified by id or address",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			wallet, err := resolver.Resolve(ctx, dbConnectionPool, tc.ref)

			if tc.expectedError != nil {
				require.Error(t, err)
				require.Equal(t, tc.expectedError, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.wantWallet.ID, wallet.ID)
				assert.Equal(t, tc.wantWallet.Name, wallet.Name)
			}
		})
	}
}

func TestResolverFactory(t *testing.T) {
	models := &data.Models{}
	factory := NewResolverFactory(models)

	assert.NotNil(t, factory.Asset())
	assert.NotNil(t, factory.Receiver())
	assert.NotNil(t, factory.Wallet())

	assert.Equal(t, factory.assetResolver, factory.Asset())
	assert.Equal(t, factory.receiverResolver, factory.Receiver())
	assert.Equal(t, factory.walletResolver, factory.Wallet())
}
