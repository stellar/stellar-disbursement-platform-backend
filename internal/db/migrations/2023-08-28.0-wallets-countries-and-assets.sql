-- +migrate Up

CREATE TABLE public.wallets_countries (
	wallet_id VARCHAR(36) REFERENCES public.wallets (id),
	country_code VARCHAR(3) REFERENCES public.countries (code),
	UNIQUE(wallet_id, country_code)
);

CREATE TABLE public.wallets_assets (
	wallet_id VARCHAR(36) REFERENCES public.wallets (id),
	asset_id VARCHAR(36) REFERENCES public.assets (id),
	UNIQUE(wallet_id, asset_id)
);

-- +migrate Down

DROP TABLE public.wallets_countries;
DROP TABLE public.wallets_assets;
