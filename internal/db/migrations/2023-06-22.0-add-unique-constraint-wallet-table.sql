-- +migrate Up

CREATE UNIQUE INDEX unique_wallets_index ON public.wallets(name, homepage, deep_link_schema);

-- +migrate Down

DROP INDEX IF EXISTS unique_wallets_index;
