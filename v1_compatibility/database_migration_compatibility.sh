#!/bin/bash
# This script is used to locally run the integration tests for compatibility between SDP-v1 and SDP-v2
set -eu

export DIVIDER="----------------------------------------"

# prepare
rm -rf stellar-relief-backoffice-backend
docker ps -aq | xargs docker stop | xargs docker rm

# Clone SDP v1
echo $DIVIDER
echo "====> 👀Step 1: start cloning SDP v1 (stellar/stellar-relief-backoffice-backend)"
git clone -b main git@github.com:stellar/stellar-relief-backoffice-backend.git
echo "====> ✅Step 1: finish cloning SDP v1 (stellar/stellar-relief-backoffice-backend)"

# Run docker compose
echo $DIVIDER
echo "====> 👀Step 2: start calling docker compose up"
docker compose down && docker-compose up --abort-on-container-exit
echo "====> ✅Step 2: finish calling docker-compose up"

echo $DIVIDER
echo "🎉🎉🎉🎉 SUCCESS! 🎉🎉🎉🎉"