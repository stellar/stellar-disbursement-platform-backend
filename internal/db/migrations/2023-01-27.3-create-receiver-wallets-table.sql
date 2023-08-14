-- This creates the receiver_wallets table and updates the other tables that depend on it.

-- +migrate Up

-- Table: receiver_wallets
CREATE TABLE public.receiver_wallets (
    id VARCHAR(36) PRIMARY KEY DEFAULT uuid_generate_v4(),
    receiver_id VARCHAR(36) NOT NULL REFERENCES public.receivers (id),
    wallet_id VARCHAR(36) REFERENCES public.wallets (id),
    stellar_address VARCHAR(56),
    stellar_memo VARCHAR(56),
    stellar_memo_type VARCHAR(56),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    UNIQUE (receiver_id, wallet_id)
);
INSERT
    INTO receiver_wallets (receiver_id, stellar_address) 
    (SELECT id, public_key FROM receivers WHERE public_key IS NOT NULL);
UPDATE public.receiver_wallets SET wallet_id = (SELECT id FROM public.wallets WHERE name = 'Vibrant Assist');
ALTER TABLE public.receiver_wallets ALTER COLUMN wallet_id SET NOT NULL;

-- Table: receivers
ALTER TABLE public.receivers DROP COLUMN public_key;

CREATE TRIGGER refresh_receiver_wallet_updated_at BEFORE UPDATE ON public.receiver_wallets FOR EACH ROW EXECUTE PROCEDURE update_at_refresh();


-- +migrate Down
DROP TRIGGER refresh_receiver_wallet_updated_at ON public.receiver_wallets;

-- Table: receivers
ALTER TABLE public.receivers ADD COLUMN public_key VARCHAR(128);
UPDATE public.receivers SET public_key = (SELECT stellar_address FROM public.receiver_wallets WHERE receiver_id = public.receivers.id);

-- Table: receiver_wallets
DROP TABLE public.receiver_wallets CASCADE;
