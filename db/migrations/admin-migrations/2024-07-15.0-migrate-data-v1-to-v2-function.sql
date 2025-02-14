-- +migrate Up

-- +migrate StatementBegin
-- This function migrates data from the public schema (V1 version) to the tenant's specific schema  (V2 version).
-- It copies relevant data from the public schema to the tenant's schema, and then imports TSS data.
CREATE OR REPLACE FUNCTION migrate_tenant_data_from_v1_to_v2(tenant_name TEXT) RETURNS void AS $$
DECLARE
    schema_name TEXT := 'sdp_' || tenant_name;
BEGIN

    -- Step 1: Delete existing data in the tenant's schema
    --      auth_users: delete the owner created during tenant provisioning
    --      wallets, assets and wallets_assets: delete data created by the `setup-for-network` cmd if any.
    EXECUTE format('
        DELETE FROM %I.auth_user_mfa_codes;
        DELETE FROM %I.auth_user_password_reset;
        DELETE FROM %I.auth_users;
        DELETE FROM %I.wallets_assets;
        DELETE FROM %I.assets;
        DELETE FROM %I.wallets;
    ', schema_name, schema_name, schema_name, schema_name, schema_name, schema_name);

    -- Step 2: Insert new data into tenant's schema.
    EXECUTE format('
        -- These tables can be copied without changing any types in the source table columns:
        INSERT INTO %I.wallets SELECT * FROM public.wallets;
        INSERT INTO %I.assets SELECT * FROM public.assets;
        INSERT INTO %I.wallets_assets SELECT * FROM public.wallets_assets;
        INSERT INTO %I.auth_users SELECT * FROM public.auth_users;
        INSERT INTO %I.receivers SELECT * FROM public.receivers;
        INSERT INTO %I.auth_user_mfa_codes SELECT * FROM public.auth_user_mfa_codes;
        INSERT INTO %I.auth_user_password_reset SELECT * FROM public.auth_user_password_reset;

        -- These tables need to have the type of some columns changed, we do that with the `ALTER TABLE` directives:
        -- NOTE: we''re not reverting the types back to the original ones, as the source tables should be dropped after the migration.
        ALTER TABLE public.receiver_wallets
            ALTER COLUMN status DROP DEFAULT,
            ALTER COLUMN status TYPE %I.receiver_wallet_status
                USING status::text::%I.receiver_wallet_status,
            ALTER COLUMN stellar_memo_type TYPE %I.memo_type
                USING stellar_memo_type::text::%I.memo_type;
        INSERT INTO %I.receiver_wallets SELECT * FROM public.receiver_wallets;

        ALTER TABLE public.receiver_verifications
            ALTER COLUMN verification_field TYPE %I.verification_type
                USING verification_field::text::%I.verification_type;
        INSERT INTO %I.receiver_verifications SELECT * FROM public.receiver_verifications;

        ALTER TABLE public.disbursements
            ALTER COLUMN status DROP DEFAULT,
            ALTER COLUMN status TYPE %I.disbursement_status
                USING status::text::%I.disbursement_status,
            ALTER COLUMN verification_field DROP DEFAULT,
            ALTER COLUMN verification_field TYPE %I.verification_type
                USING verification_field::text::%I.verification_type,
            ADD COLUMN IF NOT EXISTS registration_contact_type %I.registration_contact_types NOT NULL DEFAULT ''PHONE_NUMBER'',
            DROP COLUMN IF EXISTS country_code CASCADE;
        INSERT INTO %I.disbursements SELECT * FROM public.disbursements;

        ALTER TABLE public.payments
            ALTER COLUMN status DROP DEFAULT,
            ALTER COLUMN status TYPE %I.payment_status
                USING status::text::%I.payment_status;
        INSERT INTO %I.payments SELECT * FROM public.payments;

        ALTER TABLE public.messages
            ALTER COLUMN status DROP DEFAULT,
            ALTER COLUMN status TYPE %I.message_status
                USING status::text::%I.message_status,
            ALTER COLUMN type TYPE %I.message_type
                USING type::text::%I.message_type;
        INSERT INTO %I.messages SELECT * FROM public.messages;
    ', schema_name, schema_name, schema_name, schema_name, schema_name, schema_name, schema_name,
                   schema_name, schema_name, schema_name, schema_name, schema_name, schema_name,
                   schema_name, schema_name, schema_name, schema_name, schema_name, schema_name,
                   schema_name, schema_name, schema_name, schema_name, schema_name, schema_name,
                   schema_name, schema_name, schema_name, schema_name);


    -- Step 3: Import TSS data

    -- Add new columns to the transaction_submitter table and populate them
    ALTER TABLE public.submitter_transactions
        ADD COLUMN IF NOT EXISTS tenant_id VARCHAR(36),
        ADD COLUMN IF NOT EXISTS distribution_account_address VARCHAR(56);

    WITH SelectedTenant AS (
        SELECT id AS tenant_id, distribution_account_address
        FROM admin.tenants
        LIMIT 1
    )
    UPDATE public.submitter_transactions SET tenant_id = (SELECT tenant_id FROM SelectedTenant), distribution_account_address = (SELECT distribution_account_address FROM SelectedTenant);

    -- Copy values to the new table
    INSERT INTO tss.submitter_transactions
    SELECT
        id, external_id,
        status::text::tss.transaction_status AS status,
        status_history, status_message, asset_code, asset_issuer, amount, destination, created_at, updated_at, locked_at, started_at, sent_at, completed_at, synced_at, locked_until_ledger_number, stellar_transaction_hash, attempts_count, xdr_sent, xdr_received, tenant_id, distribution_account_address
    FROM public.submitter_transactions;

END;
$$ LANGUAGE plpgsql;
-- +migrate StatementEnd

COMMENT ON FUNCTION migrate_tenant_data_from_v1_to_v2(TEXT) IS 'Migrate data from v1 to v2 for a given tenant';

-- +migrate Down
DROP FUNCTION migrate_tenant_data_from_v1_to_v2(TEXT);