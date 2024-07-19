#!/bin/bash
# This script is used to run e2e integration tests locally with all necessary steps.
set -eu

export DIVIDER="----------------------------------------"

wait_for_server() {
  local endpoint=$1
  local max_wait_time=$2

  SECONDS=0
  while ! curl -s $endpoint > /dev/null; do
    echo "Waiting for server at $endpoint to be up... $SECONDS seconds elapsed"
    sleep 4
    if [ $SECONDS -ge $max_wait_time ]; then
      echo "Server at $endpoint is not up after $max_wait_time seconds."
      exit 1
    fi
  done
  echo "Server at $endpoint is up."
}

export DISTRIBUTION_ACCOUNT_TYPE="DISTRIBUTION_ACCOUNT.STELLAR.ENV"
export DATABASE_URL="postgres://postgres@db:5432/e2e-sdp?sslmode=disable"

echo "====> 👀Starting setup for DB migration test"
echo $DIVIDER
echo "====> 👀Step 1: start preparation"
docker container ps -aq -f name='e2e' --format '{{.ID}}' | xargs docker stop | xargs docker rm -v &&
docker volume ls -f name='e2e' --format '{{.Name}}' | xargs docker volume rm
echo "====> ✅Step 1: finish preparation"

# Run docker compose
echo $DIVIDER
echo "====> 👀Step 2: build sdp-api, anchor-platform and tss"
docker-compose -f ../docker/docker-compose-e2e-tests.yml up --build -d
wait_for_server "http://localhost:8000/health" 20
echo "====> ✅Step 2: finishing build"

echo $DIVIDER
echo "====> 👀Step 3: copy DB dump to container and restore it into the newly created database"
docker cp ../resources/single_tenant_dump.sql e2e-sdp-v2-database:/tmp/single_tenant_dump.sql
docker exec e2e-sdp-v2-database bash -c "psql -d $DATABASE_URL -f /tmp/single_tenant_dump.sql"
echo "====> ✅Step 3: finish copying and restoring DB dump"

echo $DIVIDER
echo "====> 👀Step 4: provision new tenant"
adminAccount="SDP-admin"
adminApiKey="api_key_1234567890"
encodedCredentials=$(echo -n "$adminAccount:$adminApiKey" | base64)
AuthHeader="Authorization: Basic $encodedCredentials"
tenant="migrated-tenant"
baseURL="http://$tenant.stellar.local:8000"
sdpUIBaseURL="http://$tenant.stellar.local:3000"
ownerEmail="init_owner@$tenant.local"
AdminTenantURL="http://localhost:8003/tenants"
response=$(curl -s -w "\n%{http_code}" -X POST $AdminTenantURL \
        -H "Content-Type: application/json" \
        -H "$AuthHeader" \
        -d '{
                "name": "'"$tenant"'",
                "organization_name": "'"$tenant"'",
                "base_url": "'"$baseURL"'",
                "sdp_ui_base_url": "'"$sdpUIBaseURL"'",
                "owner_email": "'"$ownerEmail"'",
                "owner_first_name": "jane",
                "owner_last_name": "doe",
                "distribution_account_type": "DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT"
        }')

http_code=$(echo "$response" | tail -n1)
response_body=$(echo "$response" | sed '$d')

if [[ "$http_code" -ge 200 && "$http_code" -lt 300 ]]; then
    echo "✅ Tenant $tenant created successfully."
    echo "🔗 You can now reset the password for the owner $ownerEmail on $sdpUIBaseURL/forgot-password"
    echo "Response body: $response_body"
else
    echo "❌ Failed to create tenant $tenant. HTTP status code: $http_code"
    echo "Server response: $response_body"
fi

echo $DIVIDER
echo "====> 👀Step 5: run migration"
docker exec e2e-sdp-v2-database bash -c "psql -d $DATABASE_URL -c \"SELECT admin.migrate_tenant_data_from_v1_to_v2('migrated-tenant');\""
echo "====> ✅Step 5: run migration"

echo $DIVIDER
echo "====> 👀Step 6: exclude deprecated tables"
docker exec e2e-sdp-v2-database bash -c "psql -d $DATABASE_URL -c \"
  BEGIN TRANSACTION;
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
\""
echo "====> ✅Step 6: exclude deprecated tables"

# Cleanup container and volumes
echo $DIVIDER
echo "====> 👀Step 7: cleaning up e2e containers and volumes"
docker container ps -aq -f name='e2e' --format '{{.ID}}' | xargs docker stop | xargs docker rm -v &&
docker volume ls -f name='e2e' --format '{{.Name}}' | xargs docker volume rm
echo "====> ✅Step 7: finish cleaning up containers and volumes"

echo $DIVIDER
echo "🎉🎉🎉🎉 SUCCESS! 🎉🎉🎉🎉"