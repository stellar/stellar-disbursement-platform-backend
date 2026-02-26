#!/bin/bash
# Run embedded wallet integration test only
set -eu

cd "$(dirname "$0")"

echo "🚀 Running Embedded Wallet Integration Test"
echo "----------------------------------------"

# Load base env
source .env

# Override with embedded wallet specific settings
export DISTRIBUTION_ACCOUNT_TYPE="DISTRIBUTION_ACCOUNT.STELLAR.ENV"
export DISBURSEMENT_CSV_FILE_NAME="disbursement_instructions_embedded_wallet.csv"
export REGISTRATION_CONTACT_TYPE="PHONE_NUMBER"
export DISBURSED_ASSET_CODE="XLM"
export DISBURSED_ASSET_ISSUER=""
export USER_EMAIL="integration-test-user@stellar.local"
export USER_PASSWORD="Password123!"
export ENABLE_EMBEDDED_WALLETS="true"
export TEST_TYPE="embedded-wallet"
export RPC_URL="${RPC_URL:-https://soroban-testnet.stellar.org}"
export EMBEDDED_WALLETS_WASM_HASH="${EMBEDDED_WALLETS_WASM_HASH:-9b784817dff1620a3e2b223fe1eb8dac56e18980dea9726f692847ccbbd3a853}"

# Cleanup
echo "====> Step 1: Cleanup"
docker compose -f docker/docker-compose-e2e-tests.yml down -v 2>/dev/null || true
docker container ps -aq -f name='e2e' --format '{{.ID}}' | xargs -r docker stop 2>/dev/null || true
docker container ps -aq -f name='e2e' --format '{{.ID}}' | xargs -r docker rm -v 2>/dev/null || true
docker volume ls -f name='e2e' --format '{{.Name}}' | xargs -r docker volume rm 2>/dev/null || true

# Start containers
echo "====> Step 2: Build and start containers"
docker compose -f docker/docker-compose-e2e-tests.yml up --build -d

# Wait for server
echo "====> Step 3: Wait for server"
wait_for_server() {
  local endpoint=$1
  local max_wait_time=$2
  SECONDS=0
  while ! curl -s $endpoint > /dev/null; do
    echo "Waiting for server at $endpoint... $SECONDS seconds"
    sleep 4
    if [ $SECONDS -ge $max_wait_time ]; then
      echo "Server not up after $max_wait_time seconds"
      exit 1
    fi
  done
  echo "Server at $endpoint is up"
}
wait_for_server "http://localhost:8000/health" 120

# Create test data
echo "====> Step 4: Create test data"
docker exec e2e-sdp-api sh -c "./stellar-disbursement-platform integration-tests create-data"

# Run embedded wallet tests
echo "====> Step 5: Run embedded wallet integration tests"
docker exec e2e-sdp-api sh -c "./stellar-disbursement-platform integration-tests start-embedded-wallet"

echo "====> Step 6: Cleanup"
docker compose -f docker/docker-compose-e2e-tests.yml logs
docker compose -f docker/docker-compose-e2e-tests.yml down -v

echo "🎉 Embedded wallet test complete!"
