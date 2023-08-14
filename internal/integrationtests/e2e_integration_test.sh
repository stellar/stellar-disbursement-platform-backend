#!/bin/bash
# This script is used to run e2e integration tests locally with all necessary steps.
set -eu

export DIVIDER="----------------------------------------"

# prepare
echo "====> 👀Step 1: start preparation"
docker container  ps -aq -f name='e2e' --format '{{.ID}}' | xargs docker stop | xargs docker rm -v &&
docker volume ls -f name='e2e' --format '{{.Name}}' | xargs docker volume rm
echo "====> ✅Step 1: finish preparation"

# Run docker compose
echo $DIVIDER
echo "====> 👀Step 2: build sdp-api, anchor-platform and tss"
docker-compose -f docker-compose-e2e-tests.yml up --build -d
sleep 10
echo "====> ✅Step 2: finishing build"

# Create new auth user
echo $DIVIDER
echo "====> 👀Step 3: create a new auth user on SDP API"
docker exec e2e-sdp-api bash -c "echo '$USER_PASSWORD' | ./stellar-disbursement-platform auth add-user '$USER_EMAIL' joe yabuki --password --owner --roles owner"
echo "====> ✅Step 3: finish creating new auth user"

# Create integration test data
echo $DIVIDER
echo "====> 👀Step 4: create new asset and test wallet on database"
docker exec e2e-sdp-api bash -c "./stellar-disbursement-platform integration-tests create-data"
echo "====> ✅Step 4: finish creating integration test data"

# Restart anchor platform container
echo $DIVIDER
echo "====> 👀Step 5: restart anchor platform container to get the new created asset"
docker restart e2e-anchor-platform
echo "waiting for anchor platform to initialize"
sleep 120
echo "====> ✅Step 5: finish restarting anchor platform container"

# Run integration tests
echo $DIVIDER
echo "====> 👀Step 6: run integration tests command"
docker exec e2e-sdp-api bash -c "./stellar-disbursement-platform integration-tests start"
echo "====> ✅Step 6: finish running integration test data"

# Cleanup container and volumes
echo $DIVIDER
echo "====> 👀Step 7: cleaning up e2e containers and volumes"
docker container  ps -aq -f name='e2e' --format '{{.ID}}' | xargs docker stop | xargs docker rm -v &&
docker volume ls -f name='e2e' --format '{{.Name}}' | xargs docker volume rm
echo "====> ✅Step 7: finish cleaning up containers and volumes"

echo $DIVIDER
echo "🎉🎉🎉🎉 SUCCESS! 🎉🎉🎉🎉"
