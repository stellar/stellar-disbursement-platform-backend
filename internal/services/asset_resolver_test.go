package services

import (
	"context"
	"reflect"
	"testing"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
)

func TestNewAssetResolver(t *testing.T) {
	type args struct {
		assetModel *data.AssetModel
	}
	tests := []struct {
		name string
		args args
		want *AssetResolver
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewAssetResolver(tt.args.assetModel); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewAssetResolver() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAssetResolver_ResolveAssetReferences(t *testing.T) {
	type fields struct {
		assetModel *data.AssetModel
	}
	type args struct {
		ctx        context.Context
		references []validators.AssetReference
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    []string
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ar := &AssetResolver{
				assetModel: tt.fields.assetModel,
			}
			got, err := ar.ResolveAssetReferences(tt.args.ctx, tt.args.references)
			if (err != nil) != tt.wantErr {
				t.Errorf("AssetResolver.ResolveAssetReferences() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("AssetResolver.ResolveAssetReferences() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAssetResolver_resolveAssetReference(t *testing.T) {
	type fields struct {
		assetModel *data.AssetModel
	}
	type args struct {
		ctx context.Context
		ref validators.AssetReference
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    string
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ar := &AssetResolver{
				assetModel: tt.fields.assetModel,
			}
			got, err := ar.resolveAssetReference(tt.args.ctx, tt.args.ref)
			if (err != nil) != tt.wantErr {
				t.Errorf("AssetResolver.resolveAssetReference() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("AssetResolver.resolveAssetReference() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAssetResolver_ValidateAssetIDs(t *testing.T) {
	type fields struct {
		assetModel *data.AssetModel
	}
	type args struct {
		ctx      context.Context
		assetIDs []string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ar := &AssetResolver{
				assetModel: tt.fields.assetModel,
			}
			if err := ar.ValidateAssetIDs(tt.args.ctx, tt.args.assetIDs); (err != nil) != tt.wantErr {
				t.Errorf("AssetResolver.ValidateAssetIDs() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
