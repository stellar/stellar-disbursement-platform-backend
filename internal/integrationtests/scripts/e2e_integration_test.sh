#!/bin/bash
# This script is used to run e2e integration tests locally with all necessary steps.
set -eu

export DIVIDER="----------------------------------------"
# prepare

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

accountTypes=("DISTRIBUTION_ACCOUNT.STELLAR.ENV" "DISTRIBUTION_ACCOUNT.CIRCLE.DB_VAULT")
if [ -z "${REGISTRATION_CONTACT_TYPE+x}" ]; then
  export REGISTRATION_CONTACT_TYPE="PHONE_NUMBER"
fi
for accountType in "${accountTypes[@]}"; do
  export DISTRIBUTION_ACCOUNT_TYPE=$accountType
  if [ $accountType="DISTRIBUTION_ACCOUNT.STELLAR.ENV" ]
  then
    platform="Stellar"
  else
    platform="Circle"
  fi

  echo "====> 👀Starting e2e setup and integration test ($platform)"
  echo $DIVIDER
  echo "====> 👀Step 1: start preparation"
  docker container ps -aq -f name='e2e' --format '{{.ID}}' | xargs docker stop | xargs docker rm -v &&
  docker volume ls -f name='e2e' --format '{{.Name}}' | xargs docker volume rm
  echo "====> ✅Step 1: finish preparation"

  # Run docker compose
  echo $DIVIDER
  echo "====> 👀Step 2: build sdp-api, anchor-platform and tss"
  docker compose -f ../docker/docker-compose-e2e-tests.yml up --build -d
  wait_for_server "http://localhost:8000/health" 20
  echo "====> ✅Step 2: finishing build"

  # Create integration test data
  echo $DIVIDER
  echo "====> 👀Step 3: provision new tenant and populate new asset and test wallet on database"
  docker exec e2e-sdp-api bash -c "./stellar-disbursement-platform integration-tests create-data"
  echo "====> ✅Step 3: finish creating integration test data ($platform)"

  # Restart anchor platform container
  echo $DIVIDER
  echo "====> 👀Step 4: restart anchor platform container to get the new created asset"
  docker restart e2e-anchor-platform
  echo "waiting for anchor platform to initialize"
  wait_for_server "http://localhost:8080/health" 120
  wait_for_server "http://localhost:8085/health" 120
  echo "====> ✅Step 4: finish restarting anchor platform container"

  # Run integration tests
  echo $DIVIDER
  echo "====> 👀Step 5: run integration tests command"
  docker exec e2e-sdp-api bash -c "./stellar-disbursement-platform integration-tests start"
  echo "====> ✅Step 5: finish running integration test data ($platform)"

  # Cleanup container and volumes
  echo $DIVIDER
  echo "====> 👀Step 6: cleaning up e2e containers and volumes"
  docker container ps -aq -f name='e2e' --format '{{.ID}}' | xargs docker stop | xargs docker rm -v &&
  docker volume ls -f name='e2e' --format '{{.Name}}' | xargs docker volume rm
  echo "====> ✅Step 6: finish cleaning up containers and volumes"
done

echo $DIVIDER
echo "🎉🎉🎉🎉 SUCCESS! 🎉🎉🎉🎉"
