#!/bin/bash
set -e

# Check prerequisites
for cmd in postgres psql go; do
    if ! command -v $cmd &>/dev/null; then
        echo "$cmd is required but not installed. Please install it."
        exit 1
    fi
done

# Set up environment variables
if [ ! -f ".env" ]; then
    ./scripts/make_env.sh
fi

set -a
source ".env"
set +a

export DATABASE_URL="postgres://postgres@localhost:5432/sdp_tss?sslmode=disable"
export NETWORK_PASSPHRASE="Test SDF Network ; September 2015"
export HORIZON_URL="https://horizon-testnet.stellar.org"
export NUM_CHANNEL_ACCOUNTS="3"
export MAX_BASE_FEE="1000000"
export TSS_METRICS_PORT="9002"
export TSS_METRICS_TYPE="TSS_PROMETHEUS"
export EVENT_BROKER_TYPE="SCHEDULER"

# Run TSS migrations
cd .. && go run ./main.go db tss migrate up

# Start TSS
go run ./main.go tss
