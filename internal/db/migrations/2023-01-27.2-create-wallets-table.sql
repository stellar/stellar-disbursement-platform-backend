-- This creates the wallets table and updates the other tables that depend on it.

-- +migrate Up

CREATE TABLE public.wallets (
    id VARCHAR(36) PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(30) NOT NULL,
    homepage VARCHAR(255) NOT NULL,
    deep_link_schema VARCHAR(30) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE,
    UNIQUE (name),
    UNIQUE (homepage),
    UNIQUE (deep_link_schema)
);
-- TODO: keep in mind that the deep link `vibrantapp://` is not confirmed yet and is subject to change.
INSERT INTO public.wallets (name, homepage, deep_link_schema) VALUES ('Vibrant Assist', 'https://vibrantapp.com', 'https://vibrantapp.com/sdp-dev');

ALTER TABLE public.disbursements
    ADD COLUMN wallet_id VARCHAR(36),
    ADD CONSTRAINT fk_disbursement_wallet_id FOREIGN KEY (wallet_id) REFERENCES public.wallets (id);
UPDATE public.disbursements SET wallet_id = (SELECT id FROM public.wallets WHERE name = 'Vibrant Assist');
ALTER TABLE public.disbursements ALTER COLUMN wallet_id SET NOT NULL;

CREATE TRIGGER refresh_wallet_updated_at BEFORE UPDATE ON public.wallets FOR EACH ROW EXECUTE PROCEDURE update_at_refresh();


-- +migrate Down
DROP TRIGGER refresh_wallet_updated_at ON public.wallets;

ALTER TABLE public.disbursements DROP COLUMN wallet_id;

DROP TABLE public.wallets CASCADE;
