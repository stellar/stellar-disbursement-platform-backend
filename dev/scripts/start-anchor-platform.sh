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

# Default Anchor Platform directory (can be overridden with ANCHOR_PLATFORM_DIR env var)
DEFAULT_ANCHOR_PLATFORM_DIR="$PROJECT_ROOT/../anchor-platform"
ANCHOR_PLATFORM_DIR="/Users/reecemarkowsky/dev/research/docker-less/anchor-platform"

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

# Check Anchor Platform directory first
print_status "Checking Anchor Platform directory..."
if [ ! -d "$ANCHOR_PLATFORM_DIR" ]; then
    print_error "Anchor Platform directory not found at: $ANCHOR_PLATFORM_DIR"
    print_error "Please specify the correct directory using:"
    print_error "export ANCHOR_PLATFORM_DIR=/path/to/anchor-platform"
    print_error "Or clone the repository:"
    print_error "git clone https://github.com/stellar/anchor-platform.git $DEFAULT_ANCHOR_PLATFORM_DIR"
    exit 1
fi

# Check if Anchor Platform JAR exists
if [ ! -f "$ANCHOR_PLATFORM_DIR/service-runner/build/libs/anchor-platform-runner-3.1.0.jar" ]; then
    print_error "Anchor Platform JAR not found in $ANCHOR_PLATFORM_DIR/service-runner/build/libs/"
    print_error "Please build the Anchor Platform first:"
    print_error "cd $ANCHOR_PLATFORM_DIR && gradle clean bootJar --stacktrace -x test"
    exit 1
fi

# Check prerequisites
print_status "Checking prerequisites..."
check_command "postgres"
check_command "psql"
check_command "java"

# Check Java version
JAVA_VERSION=$(java -version 2>&1 | awk -F '"' '/version/ {print $2}' | awk -F. '{print $1}')
if [ "$JAVA_VERSION" -lt 17 ]; then
    print_error "Java 17 or higher is required. Found Java $JAVA_VERSION"
    print_error "Please install Java 17 using: brew install openjdk@17"
    print_error "Then add to your PATH: export PATH=\"/opt/homebrew/opt/openjdk@17/bin:\$PATH\""
    exit 1
fi

# Check ports
print_status "Checking ports..."
check_port 8080 # Anchor Platform SEP
check_port 8085 # Anchor Platform Platform

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
source "$PROJECT_ROOT/.env"
source "$SCRIPT_DIR/.env-dev"

# Verify required environment variables
if [ -z "$DISTRIBUTION_PUBLIC_KEY" ]; then
    print_error "DISTRIBUTION_PUBLIC_KEY not found in .env file"
    print_error "Please make sure to populate .env (see .env.example or run scripts/make_env.sh)"
    exit 1
fi

# Create database if it doesn't exist
print_status "Creating database if it doesn't exist..."
psql -h localhost -U postgres -tc "SELECT 1 FROM pg_database WHERE datname = '$DATA_DATABASE'" | grep -q 1 || psql -h localhost -U postgres -c "CREATE DATABASE $DATA_DATABASE"

# Set up environment variables
print_status "Setting up environment variables..."
export ASSETS_TYPE="file"
export ASSETS_FILE="$SCRIPT_DIR/assets.json"

# Replace DISTRIBUTION_PUBLIC_KEY placeholder in assets.json
print_status "Updating assets.json with distribution public key..."
sed -i '' "s/DISTRIBUTION_PUBLIC_KEY/$DISTRIBUTION_PUBLIC_KEY/g" "$ASSETS_FILE"

# Secrets
export SECRET_DATA_USERNAME="$DATA_USERNAME"
export SECRET_DATA_PASSWORD="$DATA_PASSWORD"
export SECRET_PLATFORM_API_AUTH_SECRET="$ANCHOR_PLATFORM_AUTH_SECRET"
export SECRET_SEP10_JWT_SECRET="$(openssl rand -hex 32)"
export SECRET_SEP10_SIGNING_SEED="$SEP10_SIGNING_PRIVATE_KEY"
export SECRET_SEP24_INTERACTIVE_URL_JWT_SECRET="$(openssl rand -hex 32)"
export SECRET_SEP24_MORE_INFO_URL_JWT_SECRET="$(openssl rand -hex 32)"

# Debug print ASSETS_VALUE
print_status "ASSETS_VALUE: $ASSETS_FILE"
env

# Start Anchor Platform
print_status "Starting Anchor Platform..."
print_status "ASSETS_VALUE: $ASSETS_FILE"
cd "$ANCHOR_PLATFORM_DIR" && java -jar service-runner/build/libs/anchor-platform-runner-3.1.0.jar --sep-server --platform-server &
ANCHOR_PLATFORM_PID=$!

# Wait for Anchor Platform to be ready
print_status "Waiting for Anchor Platform to be ready..."
until curl -s http://localhost:$SEP_SERVER_PORT/.well-known/stellar.toml >/dev/null; do
    sleep 1
done

print_status "Anchor Platform started successfully!"
echo "Anchor Platform SEP: http://localhost:$SEP_SERVER_PORT"
echo "Anchor Platform Platform: http://localhost:$PLATFORM_SERVER_PORT"

# Handle cleanup on script exit
trap "kill $ANCHOR_PLATFORM_PID" EXIT

# Keep the script running
wait
