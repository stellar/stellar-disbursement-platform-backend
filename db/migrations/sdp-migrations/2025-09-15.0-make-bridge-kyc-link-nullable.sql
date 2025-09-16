-- Support direct bridge onboarding with optional KYC link

-- +migrate Up
-- Make kyc_link_id nullable to support direct onboarding
ALTER TABLE bridge_integration ALTER COLUMN kyc_link_id DROP NOT NULL;

-- Update constraints to allow OPTED_IN status with NULL kyc_link_id for direct onboarding
ALTER TABLE bridge_integration DROP CONSTRAINT bridge_integration_opted_in_check;
ALTER TABLE bridge_integration ADD CONSTRAINT bridge_integration_opted_in_check
    CHECK (
        status != 'OPTED_IN' OR
        (
            virtual_account_id IS NULL AND 
            opted_in_by IS NOT NULL AND 
            opted_in_at IS NOT NULL AND
            (kyc_link_id IS NOT NULL OR customer_id IS NOT NULL) -- Either KYC link OR customer_id required
        )
    );

-- Update READY_FOR_DEPOSIT constraint to allow NULL kyc_link_id for direct onboarding
ALTER TABLE bridge_integration DROP CONSTRAINT bridge_integration_ready_for_deposit_check;
ALTER TABLE bridge_integration ADD CONSTRAINT bridge_integration_ready_for_deposit_check
    CHECK (
        status != 'READY_FOR_DEPOSIT' OR
        (
            virtual_account_id IS NOT NULL AND 
            virtual_account_created_by IS NOT NULL AND 
            virtual_account_created_at IS NOT NULL AND
            customer_id IS NOT NULL
        )
    );

-- +migrate Down
ALTER TABLE bridge_integration DROP CONSTRAINT bridge_integration_opted_in_check;
ALTER TABLE bridge_integration ADD CONSTRAINT bridge_integration_opted_in_check
    CHECK (
        status != 'OPTED_IN' OR
        (kyc_link_id IS NOT NULL AND virtual_account_id IS NULL AND opted_in_by IS NOT NULL AND opted_in_at IS NOT NULL)
    );

ALTER TABLE bridge_integration DROP CONSTRAINT bridge_integration_ready_for_deposit_check;
ALTER TABLE bridge_integration ADD CONSTRAINT bridge_integration_ready_for_deposit_check 
    CHECK (
        status != 'READY_FOR_DEPOSIT' OR
        (kyc_link_id IS NOT NULL AND virtual_account_id IS NOT NULL AND virtual_account_created_by IS NOT NULL AND virtual_account_created_at IS NOT NULL)
    );