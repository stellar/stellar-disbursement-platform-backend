-- +migrate Up

CREATE TABLE wallets_assets (
	wallet_id VARCHAR(36) REFERENCES wallets (id),
	asset_id VARCHAR(36) REFERENCES assets (id),
	UNIQUE(wallet_id, asset_id)
);

-- +migrate Down

DROP TABLE wallets_assets;
