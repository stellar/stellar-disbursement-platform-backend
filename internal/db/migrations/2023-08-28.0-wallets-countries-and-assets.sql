-- +migrate Up

CREATE TABLE public.wallets_assets (
	wallet_id VARCHAR(36) REFERENCES public.wallets (id),
	asset_id VARCHAR(36) REFERENCES public.assets (id),
	UNIQUE(wallet_id, asset_id)
);

-- +migrate Down

DROP TABLE public.wallets_assets;
