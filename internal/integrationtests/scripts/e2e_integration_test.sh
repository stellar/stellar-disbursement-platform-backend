#!/bin/bash
# internal/integrationtests/scripts/e2e_integration_test.sh
set -eu

export DIVIDER="----------------------------------------"

# Load environment variables from .env file
load_env_file() {
  if [ -f ".env" ]; then
    echo "📋 Loading environment variables from .env file..."
    set -a
    source .env
    set +a  
    echo "Environment variables loaded successfully"
  else
    echo "Warning: .env file not found in current directory"
  fi
}

echo "🚀 E2E Integration Test Script"
echo $DIVIDER
load_env_file

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

# Configuration arrays with Internal SEP
Config_Internal_Stellar_Env_Phone_XLM_Testnet=(
  "platform=InternalSEP_Stellar"
  "DISTRIBUTION_ACCOUNT_TYPE=DISTRIBUTION_ACCOUNT.STELLAR.ENV"
  "DISBURSEMENT_CSV_FILE_NAME=disbursement_instructions_phone.csv"
  "REGISTRATION_CONTACT_TYPE=PHONE_NUMBER"
  "DISBURSED_ASSET_CODE=XLM"
  "DISBURSED_ASSET_ISSUER="  # Empty for native XLM
  "NETWORK_PASSPHRASE=Test SDF Network ; September 2015"
  "HORIZON_URL=https://horizon-testnet.stellar.org"
  "USER_EMAIL=integration-test-user@stellar.local"
  "USER_PASSWORD=Password123!"
  "RECAPTCHA_SITE_KEY=6LeIxAcTAAAAAJcZVRqyHh71UMIEGNQ_MXjiZKhI"
  "BASE_URL=http://stellar.local:8000"
)

Config_Internal_Stellar_Env_Email_XLM_Testnet=(
  "platform=InternalSEP_Stellar"
  "DISTRIBUTION_ACCOUNT_TYPE=DISTRIBUTION_ACCOUNT.STELLAR.ENV"
  "DISBURSEMENT_CSV_FILE_NAME=disbursement_instructions_email.csv"
  "REGISTRATION_CONTACT_TYPE=EMAIL"
  "DISBURSED_ASSET_CODE=XLM"
  "DISBURSED_ASSET_ISSUER="  # Empty for native XLM
  "NETWORK_PASSPHRASE=Test SDF Network ; September 2015"
  "HORIZON_URL=https://horizon-testnet.stellar.org"
  "USER_EMAIL=integration-test-user@stellar.local"
  "USER_PASSWORD=Password123!"
  "RECAPTCHA_SITE_KEY=6LeIxAcTAAAAAJcZVRqyHh71UMIEGNQ_MXjiZKhI"
  "BASE_URL=http://stellar.local:8000"
)

Config_Internal_Stellar_Env_PhoneWithWallet_XLM_Testnet=(
  "platform=InternalSEP_Stellar"
  "DISTRIBUTION_ACCOUNT_TYPE=DISTRIBUTION_ACCOUNT.STELLAR.ENV"
  "DISBURSEMENT_CSV_FILE_NAME=disbursement_instructions_phone_with_wallet.csv"
  "REGISTRATION_CONTACT_TYPE=PHONE_NUMBER_AND_WALLET_ADDRESS"
  "DISBURSED_ASSET_CODE=XLM"
  "DISBURSED_ASSET_ISSUER="  # Empty for native XLM
  "NETWORK_PASSPHRASE=Test SDF Network ; September 2015"
  "HORIZON_URL=https://horizon-testnet.stellar.org"
  "USER_EMAIL=integration-test-user@stellar.local"
  "USER_PASSWORD=Password123!"
  "RECAPTCHA_SITE_KEY=6LeIxAcTAAAAAJcZVRqyHh71UMIEGNQ_MXjiZKhI"
  "BASE_URL=http://stellar.local:8000"
)

echo "🔧 Test Mode: Internal - Running Internal SEP tests"
options=(
  Config_Internal_Stellar_Env_Phone_XLM_Testnet[@]
  Config_Internal_Stellar_Env_Email_XLM_Testnet[@]
  Config_Internal_Stellar_Env_PhoneWithWallet_XLM_Testnet[@]
)

# Iterate over each configuration
for config_name in "${options[@]}"; do
  config=("${!config_name}")

  echo -e "\n====> 👀 Starting e2e setup and integration test for ${config_name}"
  export CIRCLE_API_TYPE=""

  # Parse and export key-value pairs
  for pair in "${config[@]}"; do
    IFS="=" read -r key value <<< "$pair"
    echo -e "\t- $key=$value"
    export "$key"="$value"
  done

  DOCKER_PROFILE=""
  echo "Using Internal SEP services"

  DESCRIPTION="$platform - $DISTRIBUTION_ACCOUNT_TYPE - $REGISTRATION_CONTACT_TYPE"

  echo $DIVIDER
  echo "====> 👀Step 1: start preparation"
  docker compose -f docker/docker-compose-e2e-tests.yml $DOCKER_PROFILE down -v
  docker container ps -aq -f name='e2e' --format '{{.ID}}' | xargs docker stop | xargs docker rm -v
  docker volume ls -f name='e2e' --format '{{.Name}}' | xargs docker volume rm
  echo "====> ✅Step 1: finish preparation"

  # Run docker compose
  echo $DIVIDER
  echo "====> 👀Step 2: build sdp-api, tss"
  docker compose -f docker/docker-compose-e2e-tests.yml $DOCKER_PROFILE up --build -d
  wait_for_server "http://localhost:8000/health" 20
  echo "====> ✅Step 2: finishing build"

  # Create integration test data
  echo $DIVIDER
  echo "====> 👀Step 3: provision new tenant and populate new asset and test wallet on database"
  docker exec e2e-sdp-api sh -c "./stellar-disbursement-platform integration-tests create-data"
  echo "====> ✅Step 3: finish creating integration test data ($DESCRIPTION)"

  # Run integration tests
  echo $DIVIDER
  echo "====> 👀Step 4: run integration tests command"
  docker exec e2e-sdp-api sh -c "./stellar-disbursement-platform integration-tests start"
  echo "====> ✅Step 4: finish running integration test data ($DESCRIPTION)"

  # Cleanup container and volumes
  echo $DIVIDER
  echo "====> 👀Step 5: cleaning up e2e containers and volumes"
  docker compose -f docker/docker-compose-e2e-tests.yml $DOCKER_PROFILE down -v
  docker container ps -aq -f name='e2e' --format '{{.ID}}' | xargs docker stop | xargs docker rm -v
  docker volume ls -f name='e2e' --format '{{.Name}}' | xargs docker volume rm
  echo "====> ✅Step 5: finish cleaning up containers and volumes"
done

echo $DIVIDER
echo "🎉🎉🎉🎉 SUCCESS! 🎉🎉🎉🎉"