#!/bin/bash

# Exit on error
set -e

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Function to print status messages
print_status() {
    echo -e "${GREEN}====>${NC} $1"
}

# Check prerequisites
print_status "Checking prerequisites..."
for cmd in postgres psql go; do
    if ! command -v $cmd &>/dev/null; then
        echo "$cmd is required but not installed. Please install it."
    exit 1
fi
done

# Set up environment variables
print_status "Setting up environment variables..."
if [ ! -f ".env" ]; then
    print_status "Creating .env file..."
    ./scripts/make_env.sh
fi

# Source the .env file
set -a
source ".env"
set +a

# Check if PostgreSQL is running
print_status "Checking PostgreSQL..."
if ! pg_isready -h localhost -p 5432 -U postgres &>/dev/null; then
    print_status "Starting PostgreSQL..."
    brew services start postgresql@14
    sleep 5
fi

# Create databases
print_status "Creating databases..."
psql -h localhost -U postgres -tc "SELECT 1 FROM pg_database WHERE datname = 'sdp'" | grep -q 1 || psql -h localhost -U postgres -c "CREATE DATABASE sdp"
psql -h localhost -U postgres -tc "SELECT 1 FROM pg_database WHERE datname = 'sdp_auth'" | grep -q 1 || psql -h localhost -U postgres -c "CREATE DATABASE sdp_auth"
psql -h localhost -U postgres -tc "SELECT 1 FROM pg_database WHERE datname = 'sdp_tss'" | grep -q 1 || psql -h localhost -U postgres -c "CREATE DATABASE sdp_tss"
psql -h localhost -U postgres -tc "SELECT 1 FROM pg_database WHERE datname = 'sdp_admin'" | grep -q 1 || psql -h localhost -U postgres -c "CREATE DATABASE sdp_admin"

# Set SDP environment variables from docker-compose
export BASE_URL="http://localhost:8000"
export DATABASE_URL="postgres://postgres@localhost:5432/sdp?sslmode=disable"
export AUTH_DATABASE_URL="postgres://postgres@localhost:5432/sdp_auth?sslmode=disable"
export TSS_DATABASE_URL="postgres://postgres@localhost:5432/sdp_tss?sslmode=disable"
export ADMIN_DATABASE_URL="postgres://postgres@localhost:5432/sdp_admin?sslmode=disable"
export ENVIRONMENT="localhost"
export LOG_LEVEL="INFO"
export PORT="8000"
export METRICS_PORT="8002"
export METRICS_TYPE="PROMETHEUS"
export EMAIL_SENDER_TYPE="DRY_RUN"
export SMS_SENDER_TYPE="DRY_RUN"
export NETWORK_PASSPHRASE="Test SDF Network ; September 2015"
export RECAPTCHA_SITE_KEY="6LeIxAcTAAAAAJcZVRqyHh71UMIEGNQ_MXjiZKhI"
export DISABLE_MFA="true"
export DISABLE_RECAPTCHA="true"
export CORS_ALLOWED_ORIGINS="*"
export ADMIN_PORT="8003"
export INSTANCE_NAME="SDP Local Development"
export TENANT_XLM_BOOTSTRAP_AMOUNT="5"
export SINGLE_TENANT_MODE="false"
export SCHEDULER_RECEIVER_INVITATION_JOB_SECONDS="10"
export SCHEDULER_PAYMENT_JOB_SECONDS="10"
export EVENT_BROKER_TYPE="SCHEDULER"
export ADMIN_ACCOUNT="SDP-admin"
export ADMIN_API_KEY="api_key_1234567890"
export EC256_PRIVATE_KEY="-----BEGIN PRIVATE KEY-----\nMIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQgdo6o+tdFkF94B7z8\nnoybH6/zO3PryLLjLbj54/zOi4WhRANCAAQncc2mE8AQoe+1GOyXkqPBz21MypLa\nmZg3JusuzFnpy5C+DbKIShdmLE/ZwnvtywcKVcLpxvXBCn8E0YO8Yqg+\n-----END PRIVATE KEY-----"
export SEP24_JWT_SECRET="jwt_secret_1234567890"
export RECAPTCHA_SITE_SECRET_KEY="6LeIxAcTAAAAAGG-vFI1TnRWxMZNFuojJ4WifJWe"
export ANCHOR_PLATFORM_OUTGOING_JWT_SECRET="mySdpToAnchorPlatformSecret"
export ANCHOR_PLATFORM_BASE_SEP_URL="http://localhost:8080"
export ANCHOR_PLATFORM_BASE_PLATFORM_URL="http://localhost:8085"

# Run migrations
print_status "Running database migrations..."
cd .. && go run ./main.go db admin migrate up
go run ./main.go db tss migrate up
go run ./main.go db auth migrate up --all
go run ./main.go db sdp migrate up --all
go run ./main.go db setup-for-network --all

# Start SDP
print_status "Starting SDP API..."
go run ./main.go serve
