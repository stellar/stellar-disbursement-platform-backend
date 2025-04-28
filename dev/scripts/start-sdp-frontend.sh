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
SDP_FRONTEND_DIR="/Users/reecemarkowsky/dev/research/docker-less/stellar-disbursement-platform-frontend"

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

# Check prerequisites
print_status "Checking prerequisites..."
check_command "node"
check_command "yarn"

# Check if SDP frontend directory exists
if [ ! -d "$SDP_FRONTEND_DIR" ]; then
    print_error "SDP frontend directory not found at $SDP_FRONTEND_DIR"
    print_error "Please make sure the stellar-disbursement-platform-frontend repository is cloned at the same level as anchor-platform"
    exit 1
fi

# Create .env file for frontend
print_status "Creating frontend environment configuration..."
cat > "$SDP_FRONTEND_DIR/.env" << EOF
REACT_APP_DISABLE_WINDOW_ENV=false
REACT_APP_DISABLE_TENANT_PREFIL_FROM_DOMAIN=false
REACT_APP_API_URL=http://localhost:8000
REACT_APP_STELLAR_EXPERT_URL=https://stellar.expert/explorer/testnet
REACT_APP_HORIZON_URL=https://horizon-testnet.stellar.org
REACT_APP_RECAPTCHA_SITE_KEY=6Lego1wmAAAAAJNwh6RoOrsHuWnsciCTIL3NN-bn
EOF

# Create env-config.js for window environment variables
print_status "Creating window environment configuration..."
mkdir -p "$SDP_FRONTEND_DIR/public/settings"
cat > "$SDP_FRONTEND_DIR/public/settings/env-config.js" << EOF
window._env_ = {
  API_URL: "http://localhost:8000",
  STELLAR_EXPERT_URL: "https://stellar.expert/explorer/testnet",
  HORIZON_URL: "https://horizon-testnet.stellar.org",
  RECAPTCHA_SITE_KEY: "6Lego1wmAAAAAJNwh6RoOrsHuWnsciCTIL3NN-bn",
  SINGLE_TENANT_MODE: false,
};
EOF

# Install dependencies
print_status "Installing frontend dependencies..."
cd "$SDP_FRONTEND_DIR" && yarn install

# Start the frontend development server
print_status "Starting SDP frontend development server..."
cd "$SDP_FRONTEND_DIR" && yarn start 
