#!/bin/bash

# Exit on error
set -e

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Get the directory where this script is located
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# Function to print status messages
print_status() {
    echo -e "${GREEN}====>${NC} $1"
}

# Function to print warning messages
print_warning() {
    echo -e "${YELLOW}====>${NC} $1"
}

# Function to print error messages
print_error() {
    echo -e "${RED}====>${NC} $1"
}

# Function to check if a command exists
check_command() {
    if ! command -v $1 &>/dev/null; then
        print_error "$1 is required but not installed. Please install it."
        exit 1
    fi
}

# Function to check if a port is in use
check_port() {
    if lsof -i :$1 &>/dev/null; then
        print_error "Port $1 is already in use. Please free it up."
        exit 1
    fi
}

# Check prerequisites
print_status "Checking prerequisites..."
check_command "postgres"
check_command "psql"
check_command "go"

# Check if PostgreSQL is running
print_status "Checking if PostgreSQL is running..."
if ! pg_isready -h localhost -p 5432 -U postgres &>/dev/null; then
    print_error "PostgreSQL is not running on port 5432"
    print_error "Please start PostgreSQL first"
    exit 1
fi

# Check for environment variables
print_status "Checking for environment variables..."
if [ ! -f "$PROJECT_ROOT/.env" ]; then
    print_error ".env file not found"
    print_error "Please run 'make env' first to generate the required environment variables"
    exit 1
fi

if [ ! -f "$SCRIPT_DIR/.env-dev" ]; then
    print_error ".env-dev file not found"
    print_error "Please create .env-dev file in the scripts directory"
    exit 1
fi

# Source both .env files
print_status "Sourcing environment files..."
source "$PROJECT_ROOT/.env"
source "$SCRIPT_DIR/.env-dev"

# Export all variables from .env-dev
print_status "Exporting environment variables..."
export DATABASE_URL
export DATA_SERVER
export DATA_DATABASE
export DATA_USERNAME
export DATA_PASSWORD
export HOST_URL
export SEP_SERVER_PORT
export PLATFORM_SERVER_PORT
export CALLBACK_API_BASE_URL
export CALLBACK_API_AUTH_TYPE
export PLATFORM_SERVER_AUTH_TYPE
export APP_LOGGING_LEVEL
export DATA_TYPE
export DATA_FLYWAY_ENABLED
export DATA_DDL_AUTO
export METRICS_ENABLED
export METRICS_EXTRAS_ENABLED
export SEP10_ENABLED
export SEP10_HOME_DOMAINS
export SEP10_HOME_DOMAIN
export SEP10_WEB_AUTH_DOMAIN
export SEP24_ENABLED
export SEP24_INTERACTIVE_URL_BASE_URL
export SEP24_INTERACTIVE_URL_JWT_EXPIRATION
export SEP24_MORE_INFO_URL_BASE_URL
export SEP1_ENABLED
export SEP1_TOML_TYPE
export SEP1_TOML_VALUE
export ANCHOR_PLATFORM_BASE_URL
export ANCHOR_PLATFORM_PLATFORM_URL
export ANCHOR_PLATFORM_BASE_PLATFORM_URL
export ANCHOR_PLATFORM_BASE_SEP_URL
export ANCHOR_PLATFORM_AUTH_SECRET
export ANCHOR_PLATFORM_AUTH_TYPE
export ANCHOR_PLATFORM_OUTGOING_JWT_SECRET
export BASE_URL
export ENVIRONMENT
export LOG_LEVEL
export PORT
export METRICS_PORT
export METRICS_TYPE
export EMAIL_SENDER_TYPE
export SMS_SENDER_TYPE
export NETWORK_PASSPHRASE
export RECAPTCHA_SITE_KEY
export DISABLE_MFA
export DISABLE_RECAPTCHA
export CORS_ALLOWED_ORIGINS
export ADMIN_PORT
export INSTANCE_NAME
export TENANT_XLM_BOOTSTRAP_AMOUNT
export SINGLE_TENANT_MODE
export SCHEDULER_RECEIVER_INVITATION_JOB_SECONDS
export SCHEDULER_PAYMENT_JOB_SECONDS
export EVENT_BROKER_TYPE
export ADMIN_ACCOUNT
export ADMIN_API_KEY
export AWS_ACCESS_KEY_ID
export AWS_REGION
export AWS_SECRET_ACCESS_KEY
export AWS_SES_SENDER_ID
export TWILIO_ACCOUNT_SID
export TWILIO_AUTH_TOKEN
export TWILIO_SERVICE_SID
export TWILIO_SENDGRID_API_KEY
export TWILIO_SENDGRID_SENDER_ADDRESS
export EC256_PRIVATE_KEY
export SEP24_JWT_SECRET
export RECAPTCHA_SITE_SECRET_KEY

# Debug output
print_status "Verifying environment variables..."
echo "EC256_PRIVATE_KEY: $EC256_PRIVATE_KEY"
echo "INSTANCE_NAME: $INSTANCE_NAME"

# Verify required environment variables
if [ -z "$DISTRIBUTION_PUBLIC_KEY" ]; then
    print_error "DISTRIBUTION_PUBLIC_KEY not found in .env file"
    print_error "Please make sure to populate .env (see .env.example or run scripts/make_env.sh)"
    exit 1
fi

# Create databases if they don't exist
print_status "Creating databases if they don't exist..."
psql -h localhost -U postgres -tc "SELECT 1 FROM pg_database WHERE datname = 'sdp'" | grep -q 1 || psql -h localhost -U postgres -c "CREATE DATABASE sdp"
psql -h localhost -U postgres -tc "SELECT 1 FROM pg_database WHERE datname = 'sdp_auth'" | grep -q 1 || psql -h localhost -U postgres -c "CREATE DATABASE sdp_auth"
psql -h localhost -U postgres -tc "SELECT 1 FROM pg_database WHERE datname = 'sdp_tss'" | grep -q 1 || psql -h localhost -U postgres -c "CREATE DATABASE sdp_tss"
psql -h localhost -U postgres -tc "SELECT 1 FROM pg_database WHERE datname = 'sdp_admin'" | grep -q 1 || psql -h localhost -U postgres -c "CREATE DATABASE sdp_admin"

# Set up environment variables
print_status "Setting up environment variables..."
export ANCHOR_PLATFORM_DISTRIBUTION_ACCOUNT="$DISTRIBUTION_PUBLIC_KEY"
export ANCHOR_PLATFORM_SEP10_SIGNING_KEY="$SEP10_SIGNING_PUBLIC_KEY"
export ANCHOR_PLATFORM_SEP10_SIGNING_SEED="$SEP10_SIGNING_PRIVATE_KEY"
export ANCHOR_PLATFORM_SEP24_INTERACTIVE_URL_JWT_SECRET="$(openssl rand -hex 32)"
export ANCHOR_PLATFORM_SEP24_MORE_INFO_URL_JWT_SECRET="$(openssl rand -hex 32)"

# Print all environment variables
print_status "Environment variables:"
env | sort

# SDP Configuration
export INSTANCE_NAME="SDP Local Development"
export BASE_URL="http://localhost:8000"
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

# Multi-tenant configuration
export ADMIN_PORT="8003"
export TENANT_XLM_BOOTSTRAP_AMOUNT="5"
export SINGLE_TENANT_MODE="false"

# Scheduler options
export SCHEDULER_RECEIVER_INVITATION_JOB_SECONDS="10"
export SCHEDULER_PAYMENT_JOB_SECONDS="10"
export EVENT_BROKER_TYPE="SCHEDULER"

# Multi-tenant secrets
export ADMIN_ACCOUNT="SDP-admin"
export ADMIN_API_KEY="api_key_1234567890"
export DISTRIBUTION_ACCOUNT_ENCRYPTION_PASSPHRASE="${DISTRIBUTION_ACCOUNT_ENCRYPTION_PASSPHRASE}"
export CHANNEL_ACCOUNT_ENCRYPTION_PASSPHRASE="${CHANNEL_ACCOUNT_ENCRYPTION_PASSPHRASE}"

# Database configuration
export DATABASE_URL="postgres://postgres:postgres@localhost:5432/sdp?sslmode=disable"
export AUTH_DATABASE_URL="postgres://postgres:postgres@localhost:5432/sdp_auth?sslmode=disable"
export TSS_DATABASE_URL="postgres://postgres:postgres@localhost:5432/sdp_tss?sslmode=disable"
export ADMIN_DATABASE_URL="postgres://postgres:postgres@localhost:5432/sdp_admin?sslmode=disable"

# Run database migrations
print_status "Running database migrations..."
cd "$PROJECT_ROOT" && ../build/stellar-disbursement-platform db admin migrate up
cd "$PROJECT_ROOT" && ../build/stellar-disbursement-platform db tss migrate up
cd "$PROJECT_ROOT" && ../build/stellar-disbursement-platform db auth migrate up --all
cd "$PROJECT_ROOT" && ../build/stellar-disbursement-platform db sdp migrate up --all
cd "$PROJECT_ROOT" && ../build/stellar-disbursement-platform db setup-for-network --all

# Start SDP API
print_status "Starting SDP API..."
cd "$PROJECT_ROOT" && ../build/stellar-disbursement-platform serve &
SDP_API_PID=$!

# Wait for SDP API to be ready
print_status "Waiting for SDP API to be ready..."
until curl -s http://localhost:8000/health >/dev/null; do
    sleep 1
done

print_status "SDP API started successfully!"
echo "SDP API: http://localhost:8000"

# Handle cleanup on script exit
trap "kill $SDP_API_PID" EXIT

# Keep the script running
wait
