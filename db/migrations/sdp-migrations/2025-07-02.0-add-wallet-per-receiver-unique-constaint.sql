-- +migrate Up
DO $$
DECLARE
    duplicate_count INTEGER;
BEGIN
    SELECT COUNT(*) INTO duplicate_count
    FROM (
        SELECT stellar_address
        FROM receiver_wallets 
        WHERE stellar_address IS NOT NULL 
        AND trim(stellar_address) != ''
        GROUP BY stellar_address
        HAVING COUNT(DISTINCT receiver_id) > 1
    ) duplicates;
    
    IF duplicate_count > 0 THEN
        RAISE EXCEPTION 'Migration failed: Found % stellar addresses shared across multiple receivers. Please resolve data conflicts before proceeding.', duplicate_count;
    END IF;
END $$;

CREATE INDEX idx_receiver_wallets_stellar_address 
ON receiver_wallets (stellar_address) 
WHERE stellar_address IS NOT NULL;

-- +migrate StatementBegin
CREATE OR REPLACE FUNCTION validate_stellar_address_per_receiver() 
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.stellar_address IS NULL OR trim(NEW.stellar_address) = '' THEN
        RETURN NEW;
    END IF;
    IF EXISTS (
        SELECT 1 FROM receiver_wallets 
        WHERE stellar_address = NEW.stellar_address 
        AND receiver_id != NEW.receiver_id
    ) THEN
        RAISE EXCEPTION 'Stellar address % already belongs to another receiver', NEW.stellar_address;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +migrate StatementEnd

CREATE TRIGGER trigger_validate_stellar_address_per_receiver
    BEFORE INSERT OR UPDATE ON receiver_wallets
    FOR EACH ROW
    EXECUTE FUNCTION validate_stellar_address_per_receiver();

-- +migrate Down
DROP TRIGGER trigger_validate_stellar_address_per_receiver ON receiver_wallets;

DROP FUNCTION IF EXISTS validate_stellar_address_per_receiver;
DROP INDEX IF EXISTS idx_receiver_wallets_stellar_address;