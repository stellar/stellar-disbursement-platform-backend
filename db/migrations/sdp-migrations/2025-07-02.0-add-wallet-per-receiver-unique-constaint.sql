-- +migrate Up
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