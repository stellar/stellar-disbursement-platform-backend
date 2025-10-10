#!/bin/sh
# This script generates SSL certificates for local development using mkcert
# It creates a wildcard certificate for *.stellar.local and localhost

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
CERTS_DIR="$SCRIPT_DIR/../certs"

echo "====> Generating SSL certificates for local development"

# Check if mkcert is installed
if ! command -v mkcert > /dev/null 2>&1; then
    echo "ERROR: mkcert is not installed."
    echo "See: https://web.dev/articles/how-to-use-local-https"
    exit 1
fi

# Check if mkcert CA is installed
if ! mkcert -CAROOT > /dev/null 2>&1; then
    echo "ERROR: mkcert CA is not installed."
    echo "Run: mkcert -install"
    exit 1
fi

# Create certs directory if it doesn't exist
mkdir -p "$CERTS_DIR"

# Generate certificates using mkcert
echo "Generating certificates for *.stellar.local, localhost, 127.0.0.1..."
cd "$CERTS_DIR"
mkcert -key-file stellar.local-key.pem -cert-file stellar.local.pem \
    "*.stellar.local" localhost 127.0.0.1 ::1

echo "====> Certificates generated successfully"
echo "Location: $CERTS_DIR"
echo "  - Certificate: stellar.local.pem"
echo "  - Private Key: stellar.local-key.pem"
