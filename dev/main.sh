#!/bin/bash
# This script is used to locally start the integration between SDP and AnchorPlatform for the SEP-24 deposit flow, needed for registering users.
set -eu

export DIVIDER="----------------------------------------"

# prepare
echo "====> 👀Step 1: start preparation"
docker ps -aq | xargs docker stop | xargs docker rm
echo "====> ✅Step 1: finish preparation"

# Run docker compose
echo $DIVIDER
echo "====> 👀Step 2: start calling docker compose up"
docker-compose -f docker-compose-sdp-anchor.yml down && docker-compose -f docker-compose-sdp-anchor.yml up --abort-on-container-exit
echo "====> ✅Step 2: finish calling docker-compose up"

echo $DIVIDER
echo "🎉🎉🎉🎉 SUCCESS! 🎉🎉🎉🎉"
