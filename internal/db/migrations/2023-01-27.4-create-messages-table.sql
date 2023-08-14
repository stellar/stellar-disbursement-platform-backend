-- This creates the messages table and updates the other tables that depend on it.

-- +migrate Up

CREATE TYPE message_type AS ENUM(
    'TWILIO_SMS',
    'AWS_SMS',
    'AWS_EMAIL'
);

-- Table: messages
CREATE TABLE public.messages (
    id VARCHAR(36) PRIMARY KEY DEFAULT uuid_generate_v4(),
    type message_type NOT NULL,
    asset_id VARCHAR(36) NOT NULL REFERENCES public.assets (id),
    wallet_id VARCHAR(36) NOT NULL REFERENCES public.wallets (id),
    receiver_id VARCHAR(36) NOT NULL REFERENCES public.receivers (id),
    text_encrypted VARCHAR(1024) NOT NULL,
    title_encrypted VARCHAR(128),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- +migrate Down

-- Table: messages
DROP TABLE public.messages CASCADE;
DROP TYPE message_type;
