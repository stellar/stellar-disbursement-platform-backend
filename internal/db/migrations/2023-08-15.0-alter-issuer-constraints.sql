-- +migrate Up

ALTER TABLE public.assets
    DROP CONSTRAINT asset_issuer_length_check RESTRICT,
    ADD CONSTRAINT asset_issuer_length_check CHECK ((code = 'XLM' AND char_length(issuer) = 0) OR char_length(issuer) = 56);

ALTER TABLE public.submitter_transactions
    ADD CONSTRAINT asset_issuer_length_check CHECK ((asset_code = 'XLM' AND char_length(asset_issuer) = 0) OR char_length(asset_issuer) = 56);

-- +migrate Down

ALTER TABLE public.assets
    DROP CONSTRAINT asset_issuer_length_check RESTRICT,
    ADD CONSTRAINT asset_issuer_length_check CHECK (char_length(issuer) = 56);

ALTER TABLE public.submitter_transactions
    DROP CONSTRAINT asset_issuer_length_check RESTRICT;
