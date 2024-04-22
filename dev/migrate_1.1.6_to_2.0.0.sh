#!/bin/bash
# This script is used to locally start the integration between SDP and AnchorPlatform for the SEP-24 deposit flow, needed for registering users.
set -eu

# Check if curl is installed
if ! command -v curl &> /dev/null
then
    echo "Error: curl is not installed. Please install curl to continue."
    exit 1
fi

singleTenantDBURL="postgres://localhost:5432/sdp-116?sslmode=disable"
multiTenantDBURL="postgres://localhost:5432/sdp-116-mtn?sslmode=disable"
multiTenantDBName="sdp-116-mtn"
psqlDumpOutput="sdp-mainBranch-v1_16.sql"

perform_step() {
    export DIVIDER="----------------------------------------"

    step=$1
    step_name="$2"
    command="$3"
    pause_message="${4:-}"
    
    echo
    echo $DIVIDER
    printf "STEP %s ⌛: %s\n" "$step" "$step_name"
    eval "$command"
    printf "STEP %s ✅: %s\n" "$step" "$step_name"
    echo $DIVIDER

    if [ -n "$pause_message" ]; then
        read -p "$pause_message"
    fi
}

stepCounter=1

perform_step $((stepCounter++)) "deleting pre-existing single-tenant dump file" "rm -f $psqlDumpOutput" "In the next step, we will delete and recreate the MTN database. Click enter to continue."

perform_step $((stepCounter++)) "deleting and recreating the multi-tenant database" "dropdb $multiTenantDBName && createdb $multiTenantDBName" "In the next step, we will be creating a new dump from the single-tenant DB. Click enter to continue."

perform_step $((stepCounter++)) "dumping single-tenant database" "pg_dump $singleTenantDBURL > $psqlDumpOutput" "In the next step, we will be restoring the single-tenant dump in the multi-tenant database. Click enter to continue."

perform_step $((stepCounter++)) "restoring single-tenant dump into multi-tenant database" "psql -d $multiTenantDBURL < $psqlDumpOutput" "In the next step, we will be running the TSS and Admin migrations. Click enter to continue."

perform_step $((stepCounter++)) "running tss and admin migrations" "go run main.go db admin migrate up && go run main.go db tss migrate up"


read -p "🚨 ATTENTION: for the next step, you'll need the admin server to be up and running under http://localhost:8003. Make sure to run 'go run main.go serve', and then hit Enter to continue"

tenant="bluecorp"
create_tenant() {
    ADMIN_ACCOUNT="SDP-admin"
    ADMIN_API_KEY="api_key_1234567890"
    basicAuthCredentials=$(echo -n "$ADMIN_ACCOUNT:$ADMIN_API_KEY" | base64)
    AuthHeader="Authorization: Basic $basicAuthCredentials"

    baseURL="http://$tenant.stellar.local:8000"
    sdpUIBaseURL="http://$tenant.stellar.local:3000"
    ownerEmail="owner@$tenant.org"

    curl -X POST http://localhost:8003/tenants \
        -H "Content-Type: application/json" \
        -H "$AuthHeader" \
        -d '{
                "name": "'"$tenant"'",
                "organization_name": "Blue Corp",
                "base_url": "'"$baseURL"'",
                "sdp_ui_base_url": "'"$sdpUIBaseURL"'",
                "owner_email": "'"$ownerEmail"'",
                "owner_first_name": "john",
                "owner_last_name": "doe"
        }'
    echo
}

perform_step $((stepCounter++)) "provisioning tenant $tenant" "create_tenant" "Your tenant was successfully created! Hit enter to copy the single-tenant data to the multi-tenant structure, and dump the single-tenant structure."

sql_script=$(cat <<EOF
BEGIN TRANSACTION;

-- 1. delete multi-tenant data that was auto-created
DELETE FROM sdp_${tenant}.auth_users;
DELETE FROM sdp_${tenant}.wallets_assets;
DELETE FROM sdp_${tenant}.assets;
DELETE FROM sdp_${tenant}.wallets;


-- 2. Copy the data
---- 2.1 These tables can be copied without changing any types in the source table columns:
INSERT INTO sdp_${tenant}.wallets SELECT * FROM public.wallets;
INSERT INTO sdp_${tenant}.assets SELECT * FROM public.assets;
INSERT INTO sdp_${tenant}.wallets_assets SELECT * FROM public.wallets_assets;
INSERT INTO sdp_${tenant}.auth_users SELECT * FROM public.auth_users;
INSERT INTO sdp_${tenant}.receivers SELECT * FROM public.receivers;
INSERT INTO sdp_${tenant}.auth_user_mfa_codes SELECT * FROM public.auth_user_mfa_codes;
INSERT INTO sdp_${tenant}.auth_user_password_reset SELECT * FROM public.auth_user_password_reset;

---- 2.2. These tables need to have the type of some columns changed, we do that with the ALTER TABLE directives:
---- NOTE: we are not reverting the types back to the original ones, as the source tables will be dropped after the migration.
ALTER TABLE public.receiver_wallets
	ALTER COLUMN status DROP DEFAULT,
	ALTER COLUMN status TYPE sdp_${tenant}.receiver_wallet_status
		USING status::text::sdp_${tenant}.receiver_wallet_status;
INSERT INTO sdp_${tenant}.receiver_wallets SELECT * FROM public.receiver_wallets;

ALTER TABLE public.receiver_verifications
	ALTER COLUMN verification_field TYPE sdp_${tenant}.verification_type
		USING verification_field::text::sdp_${tenant}.verification_type;
INSERT INTO sdp_${tenant}.receiver_verifications SELECT * FROM public.receiver_verifications;

ALTER TABLE public.disbursements
	ALTER COLUMN status DROP DEFAULT,
	ALTER COLUMN status TYPE sdp_${tenant}.disbursement_status
		USING status::text::sdp_${tenant}.disbursement_status;
ALTER TABLE public.disbursements
	ALTER COLUMN verification_field DROP DEFAULT,
	ALTER COLUMN verification_field TYPE sdp_${tenant}.verification_type
		USING verification_field::text::sdp_${tenant}.verification_type;
INSERT INTO sdp_${tenant}.disbursements SELECT * FROM public.disbursements;

ALTER TABLE public.payments
	ALTER COLUMN status DROP DEFAULT,
	ALTER COLUMN status TYPE sdp_${tenant}.payment_status
		USING status::text::sdp_${tenant}.payment_status;
INSERT INTO sdp_${tenant}.payments SELECT * FROM public.payments;

ALTER TABLE public.messages
	ALTER COLUMN status DROP DEFAULT,
	ALTER COLUMN status TYPE sdp_${tenant}.message_status
		USING status::text::sdp_${tenant}.message_status;
ALTER TABLE public.messages
	ALTER COLUMN type TYPE sdp_${tenant}.message_type
		USING type::text::sdp_${tenant}.message_type;
INSERT INTO sdp_${tenant}.messages SELECT * FROM public.messages;


-- 2.3 Copy the data from the submitter_transactions table
------ 2.3.1: add new columns to the transaction_submitter table and populate them
ALTER TABLE public.submitter_transactions
    ADD COLUMN tenant_id VARCHAR(36),
    ADD COLUMN distribution_account VARCHAR(56);
WITH SelectedTenant AS (
    SELECT id AS tenant_id, distribution_account
    FROM admin.tenants
    LIMIT 1
)
UPDATE public.submitter_transactions SET tenant_id = (SELECT tenant_id FROM SelectedTenant), distribution_account = (SELECT distribution_account FROM SelectedTenant);

------ 2.3.2: copy values to the new table
INSERT INTO tss.submitter_transactions 
SELECT
    id, external_id,
    status::text::tss.transaction_status AS status,
    status_history, status_message, asset_code, asset_issuer, amount, destination, created_at, updated_at, locked_at, started_at, sent_at, completed_at, synced_at, locked_until_ledger_number, stellar_transaction_hash, attempts_count, xdr_sent, xdr_received, tenant_id, distribution_account
FROM public.submitter_transactions;


-- 3. delete the source tables
DROP TABLE public.messages CASCADE;
DROP TABLE public.payments CASCADE;
DROP TABLE public.disbursements CASCADE;
DROP TABLE public.receiver_verifications CASCADE;
DROP TABLE public.receiver_wallets CASCADE;
DROP TABLE public.auth_user_password_reset CASCADE;
DROP TABLE public.auth_user_mfa_codes CASCADE;
DROP TABLE public.receivers CASCADE;
DROP TABLE public.auth_users CASCADE;
DROP TABLE public.wallets_assets CASCADE;
DROP TABLE public.assets CASCADE;
DROP TABLE public.wallets CASCADE;
DROP TABLE public.organizations CASCADE;
DROP TABLE public.gorp_migrations CASCADE;
DROP TABLE public.auth_migrations CASCADE;
DROP TABLE public.countries CASCADE;
DROP TABLE public.submitter_transactions CASCADE;
DROP TABLE public.channel_accounts CASCADE;


COMMIT;
EOF
)

perform_step $((stepCounter++)) "copying data from the single-tenant to the multi-tenant structure" "echo '$sql_script' | psql -d '$multiTenantDBURL'"

echo "🎉🎉🎉🎉 Successfully migrated the data!"
