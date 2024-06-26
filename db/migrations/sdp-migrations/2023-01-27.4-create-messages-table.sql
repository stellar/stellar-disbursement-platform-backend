-- This creates the messages table and updates the other tables that depend on it.

-- +migrate Up

CREATE TYPE message_type AS ENUM(
    'TWILIO_SMS',
    'AWS_SMS',
    'AWS_EMAIL'
);

-- Table: messages
CREATE TABLE messages (
    id VARCHAR(36) PRIMARY KEY DEFAULT public.uuid_generate_v4(),
    type message_type NOT NULL,
    asset_id VARCHAR(36) NOT NULL REFERENCES assets (id),
    wallet_id VARCHAR(36) NOT NULL REFERENCES wallets (id),
    receiver_id VARCHAR(36) NOT NULL REFERENCES receivers (id),
    text_encrypted VARCHAR(1024) NOT NULL,
    title_encrypted VARCHAR(128),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- +migrate StatementBegin
-- Insert messages if there is pre-existing data, so to prevent SMSs from being re-triggered.
DO $$
BEGIN
    IF (SELECT COUNT(*) FROM receivers WHERE status <> ALL(ARRAY['READY', 'PAID', 'PARTIALLY_CASHED_OUT', 'FULLY_CASHED_OUT'])) > 0 THEN 
        INSERT INTO messages (
            type,
            asset_id,
            wallet_id,
            receiver_id,
            text_encrypted,
            title_encrypted,
            created_at
        )
        SELECT 
            'AWS_SMS',
            (SELECT id FROM assets WHERE code = 'USDC' LIMIT 1),
            (SELECT id FROM wallets WHERE name ILIKE '%Vibrant%' LIMIT 1),
            r.id,
            'text omitted during initial migration',
            'title omitted during initial migration',
            NOW()
        FROM receivers r
        WHERE r.status <> ALL(ARRAY['READY', 'PAID', 'PARTIALLY_CASHED_OUT', 'FULLY_CASHED_OUT']);
    END IF; 
END $$;
-- +migrate StatementEnd

-- +migrate Down

-- Table: messages
DROP TABLE messages CASCADE;
DROP TYPE message_type;
