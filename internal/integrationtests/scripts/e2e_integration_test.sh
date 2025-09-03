#!/bin/bash
# internal/integrationtests/scripts/e2e_integration_test.sh
set -eu

export DIVIDER="----------------------------------------"

# Check command line arguments
if [ $# -eq 0 ]; then
  echo "❌ Error: Please specify which tests to run"
  echo "Usage: $0 [internal|ap]"
  echo "  internal - Run Internal SEP tests only"
  echo "  ap       - Run Anchor Platform tests only"
  exit 1
fi

TEST_MODE=$1
if [ "$TEST_MODE" != "internal" ] && [ "$TEST_MODE" != "ap" ]; then
  echo "❌ Error: Invalid test mode '$TEST_MODE'"
  echo "Usage: $0 [internal|ap]"
  echo "  internal - Run Internal SEP tests only"
  echo "  ap       - Run Anchor Platform tests only"
  exit 1
fi

echo "🚀 E2E Integration Test Script"
echo "📋 Test Mode: $TEST_MODE"
echo "🔧 Configuration: $([ "$TEST_MODE" = "ap" ] && echo "Anchor Platform ONLY" || echo "Internal SEP ONLY")"
echo $DIVIDER

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

# Configuration arrays with Anchor Platform
Config_AnchorPlatform_Stellar_Env_Phone_USDC_Testnet=(
  "platform=AnchorPlatform_Stellar"
  "ENABLE_ANCHOR_PLATFORM=true"
  "DISTRIBUTION_ACCOUNT_TYPE=DISTRIBUTION_ACCOUNT.STELLAR.ENV"
  "DISBURSEMENT_CSV_FILE_NAME=disbursement_instructions_phone.csv"
  "REGISTRATION_CONTACT_TYPE=PHONE_NUMBER"
  "DISBURSED_ASSET_CODE=USDC"
  "DISBURSED_ASSET_ISSUER=GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5"
  "USER_EMAIL=integration-test-user@stellar.local"
  "USER_PASSWORD=Password123!"
  "RECAPTCHA_SITE_KEY=6LeIxAcTAAAAAJcZVRqyHh71UMIEGNQ_MXjiZKhI"
)

Config_AnchorPlatform_CircleTransfer_DBVault_Phone_USDC_Testnet=(
  "platform=AnchorPlatform_Circle"
  "ENABLE_ANCHOR_PLATFORM=true"
  "CIRCLE_API_TYPE=TRANSFERS"
  "DISTRIBUTION_ACCOUNT_TYPE=DISTRIBUTION_ACCOUNT.CIRCLE.DB_VAULT"
  "DISBURSEMENT_CSV_FILE_NAME=disbursement_instructions_phone.csv"
  "REGISTRATION_CONTACT_TYPE=PHONE_NUMBER"
  "DISBURSED_ASSET_CODE=USDC"
  "DISBURSED_ASSET_ISSUER=GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5"
  "USER_EMAIL=integration-test-user@stellar.local"
  "USER_PASSWORD=Password123!"
  "RECAPTCHA_SITE_KEY=6LeIxAcTAAAAAJcZVRqyHh71UMIEGNQ_MXjiZKhI"
)

# Configuration arrays with Internal SEP
Config_Internal_Stellar_Env_Phone_XLM_Testnet=(
  "platform=InternalSEP_Stellar"
  "ENABLE_ANCHOR_PLATFORM=false"
  "DISTRIBUTION_ACCOUNT_TYPE=DISTRIBUTION_ACCOUNT.STELLAR.ENV"
  "DISBURSEMENT_CSV_FILE_NAME=disbursement_instructions_phone.csv"
  "REGISTRATION_CONTACT_TYPE=PHONE_NUMBER"
  "DISBURSED_ASSET_CODE=XLM"
  "DISBURSED_ASSET_ISSUER="  # Empty for native XLM
  "NETWORK_PASSPHRASE=Test SDF Network ; September 2015"
  "HORIZON_URL=https://horizon-testnet.stellar.org"
  "SEP10_SIGNING_PUBLIC_KEY=GASJFPGBOIC2KEIZB57OGHKWVOOJJ2IXELSP6HZTIRRCQN2O3CGHZYON"
  "SEP10_SIGNING_PRIVATE_KEY=SC2WWCBFSGB5O66TQ2XMXJUY7KONHCJXGMIPKJBPIVE5IJPEFRKIRZVA"
  "DISTRIBUTION_PUBLIC_KEY=GDTHBM4X7MOTYXJBUT43MFRDREJ4XFIFYP5XTWKAZCJJGXHKQRGRSN3X"
  "DISTRIBUTION_SEED=SCOISU5ODOKTMC2EUKH7BUGUKYVSRW4O5Q5LDP7GZRW2RODP2OPHLWKU"
  "USER_EMAIL=integration-test-user@stellar.local"
  "USER_PASSWORD=Password123!"
  "RECAPTCHA_SITE_KEY=6LeIxAcTAAAAAJcZVRqyHh71UMIEGNQ_MXjiZKhI"
  "CLIENT_DOMAIN_SIGNING_KEY=SBFJYZCBUEEU3C47INA23C72J3D2KJH3G26ILLGQESVX5H5ABREWRGF6"
  "BASE_URL=http://stellar.local:8000"
)

Config_Internal_Stellar_Env_Email_XLM_Testnet=(
  "platform=InternalSEP_Stellar"
  "ENABLE_ANCHOR_PLATFORM=false"
  "DISTRIBUTION_ACCOUNT_TYPE=DISTRIBUTION_ACCOUNT.STELLAR.ENV"
  "DISBURSEMENT_CSV_FILE_NAME=disbursement_instructions_email.csv"
  "REGISTRATION_CONTACT_TYPE=EMAIL"
  "DISBURSED_ASSET_CODE=XLM"
  "DISBURSED_ASSET_ISSUER="  # Empty for native XLM
  "NETWORK_PASSPHRASE=Test SDF Network ; September 2015"
  "HORIZON_URL=https://horizon-testnet.stellar.org"
  "SEP10_SIGNING_PUBLIC_KEY=GASJFPGBOIC2KEIZB57OGHKWVOOJJ2IXELSP6HZTIRRCQN2O3CGHZYON"
  "SEP10_SIGNING_PRIVATE_KEY=SC2WWCBFSGB5O66TQ2XMXJUY7KONHCJXGMIPKJBPIVE5IJPEFRKIRZVA"
  "DISTRIBUTION_PUBLIC_KEY=GDTHBM4X7MOTYXJBUT43MFRDREJ4XFIFYP5XTWKAZCJJGXHKQRGRSN3X"
  "DISTRIBUTION_SEED=SCOISU5ODOKTMC2EUKH7BUGUKYVSRW4O5Q5LDP7GZRW2RODP2OPHLWKU"
  "USER_EMAIL=integration-test-user@stellar.local"
  "USER_PASSWORD=Password123!"
  "RECAPTCHA_SITE_KEY=6LeIxAcTAAAAAJcZVRqyHh71UMIEGNQ_MXjiZKhI"
  "CLIENT_DOMAIN_SIGNING_KEY=SBFJYZCBUEEU3C47INA23C72J3D2KJH3G26ILLGQESVX5H5ABREWRGF6"
  "BASE_URL=http://stellar.local:8000"
)

Config_Internal_Stellar_Env_PhoneWithWallet_XLM_Testnet=(
  "platform=InternalSEP_Stellar"
  "ENABLE_ANCHOR_PLATFORM=false"
  "DISTRIBUTION_ACCOUNT_TYPE=DISTRIBUTION_ACCOUNT.STELLAR.ENV"
  "DISBURSEMENT_CSV_FILE_NAME=disbursement_instructions_phone_with_wallet.csv"
  "REGISTRATION_CONTACT_TYPE=PHONE_NUMBER_AND_WALLET_ADDRESS"
  "DISBURSED_ASSET_CODE=XLM"
  "DISBURSED_ASSET_ISSUER="  # Empty for native XLM
  "NETWORK_PASSPHRASE=Test SDF Network ; September 2015"
  "HORIZON_URL=https://horizon-testnet.stellar.org"
  "SEP10_SIGNING_PUBLIC_KEY=GASJFPGBOIC2KEIZB57OGHKWVOOJJ2IXELSP6HZTIRRCQN2O3CGHZYON"
  "SEP10_SIGNING_PRIVATE_KEY=SC2WWCBFSGB5O66TQ2XMXJUY7KONHCJXGMIPKJBPIVE5IJPEFRKIRZVA"
  "DISTRIBUTION_PUBLIC_KEY=GDTHBM4X7MOTYXJBUT43MFRDREJ4XFIFYP5XTWKAZCJJGXHKQRGRSN3X"
  "DISTRIBUTION_SEED=SCOISU5ODOKTMC2EUKH7BUGUKYVSRW4O5Q5LDP7GZRW2RODP2OPHLWKU"
  "USER_EMAIL=integration-test-user@stellar.local"
  "USER_PASSWORD=Password123!"
  "RECAPTCHA_SITE_KEY=6LeIxAcTAAAAAJcZVRqyHh71UMIEGNQ_MXjiZKhI"
  "CLIENT_DOMAIN_SIGNING_KEY=SBFJYZCBUEEU3C47INA23C72J3D2KJH3G26ILLGQESVX5H5ABREWRGF6"
  "BASE_URL=http://stellar.local:8000"
)

# Dynamic configuration selection based on command line argument - mutually exclusive
if [ "$TEST_MODE" = "ap" ]; then
  # ONLY Anchor Platform configurations
  echo "🔧 Test Mode: AP - Running ONLY Anchor Platform tests"
  options=(
    Config_AnchorPlatform_Stellar_Env_Phone_USDC_Testnet[@]
    Config_AnchorPlatform_CircleTransfer_DBVault_Phone_USDC_Testnet[@]
  )
else
  # ONLY Internal SEP configurations
  echo "🔧 Test Mode: Internal - Running ONLY Internal SEP tests"
  options=(
    Config_Internal_Stellar_Env_Phone_XLM_Testnet[@]
    Config_Internal_Stellar_Env_Email_XLM_Testnet[@]
    Config_Internal_Stellar_Env_PhoneWithWallet_XLM_Testnet[@]
  )
fi

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

  # Determine which Docker profile to use
  if [ "$TEST_MODE" = "ap" ]; then
    DOCKER_PROFILE="--profile anchor-platform"
    echo "Using Anchor Platform integration"
  else
    DOCKER_PROFILE=""
    echo "Using Internal SEP services"
  fi

  DESCRIPTION="$platform - $DISTRIBUTION_ACCOUNT_TYPE - $REGISTRATION_CONTACT_TYPE"

  echo $DIVIDER
  echo "====> 👀Step 1: start preparation"
  docker compose -f ../docker/docker-compose-e2e-tests.yml $DOCKER_PROFILE down -v
  docker container ps -aq -f name='e2e' --format '{{.ID}}' | xargs docker stop | xargs docker rm -v
  docker volume ls -f name='e2e' --format '{{.Name}}' | xargs docker volume rm
  echo "====> ✅Step 1: finish preparation"

  # Run docker compose
  echo $DIVIDER
  echo "====> 👀Step 2: build sdp-api, tss, and optionally anchor-platform ($DESCRIPTION)"
  docker compose -f ../docker/docker-compose-e2e-tests.yml $DOCKER_PROFILE up --build -d
  wait_for_server "http://localhost:8000/health" 20
  echo "====> ✅Step 2: finishing build"

  # Create integration test data
  echo $DIVIDER
  echo "====> 👀Step 3: provision new tenant and populate new asset and test wallet on database"
  docker exec e2e-sdp-api sh -c "./stellar-disbursement-platform integration-tests create-data"
  echo "====> ✅Step 3: finish creating integration test data ($DESCRIPTION)"

  # Restart anchor platform container if it's being used
  if [ "$TEST_MODE" = "ap" ]; then
    echo $DIVIDER
    echo "====> 👀Step 4: restart anchor platform container so the new created asset shows up in the toml file"
    docker restart e2e-anchor-platform
    echo "waiting for anchor platform to initialize"
    wait_for_server "http://localhost:8080/health" 120
    wait_for_server "http://localhost:8085/health" 120
    echo "====> ✅Step 4: finish restarting anchor platform container"
  else
    echo $DIVIDER
    echo "====> 👀Step 4: skipping anchor platform restart (using internal SEP services)"
  fi

  # Run integration tests
  echo $DIVIDER
  echo "====> 👀Step 5: run integration tests command"
  docker exec e2e-sdp-api sh -c "./stellar-disbursement-platform integration-tests start"
  echo "====> ✅Step 5: finish running integration test data ($DESCRIPTION)"

  # Cleanup container and volumes
  echo $DIVIDER
  echo "====> 👀Step 6: cleaning up e2e containers and volumes"
  docker compose -f ../docker/docker-compose-e2e-tests.yml $DOCKER_PROFILE down -v
  docker container ps -aq -f name='e2e' --format '{{.ID}}' | xargs docker stop | xargs docker rm -v
  docker volume ls -f name='e2e' --format '{{.Name}}' | xargs docker volume rm
  echo "====> ✅Step 6: finish cleaning up containers and volumes"
done

echo $DIVIDER
echo "🎉🎉🎉🎉 SUCCESS! 🎉🎉🎉🎉"