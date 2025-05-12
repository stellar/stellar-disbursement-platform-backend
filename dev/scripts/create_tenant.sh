#!/bin/bash
set -e

# Check prerequisites
for cmd in curl jq; do
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

# Define tenant details
TENANT_NAME="localhost"
TENANT_ORG_NAME="Local Organization"
TENANT_EMAIL="admin@localhost.local"
TENANT_BASE_URL="http://localhost:8000"
TENANT_UI_BASE_URL="http://localhost:3000"

# Check if tenant already exists
TENANT_EXISTS=$(curl -s -u 'SDP-admin:api_key_1234567890' http://localhost:8003/tenants | jq -r '.[] | select(.name == "'$TENANT_NAME'") | .id')

if [ -z "$TENANT_EXISTS" ]; then
    # Create tenant
    echo "Creating tenant: $TENANT_NAME"
    curl -X POST http://localhost:8003/tenants \
        -H "Content-Type: application/json" \
        -H "Authorization: Basic $(echo -n 'SDP-admin:api_key_1234567890' | base64)" \
        -d "{
      \"name\": \"$TENANT_NAME\",
      \"organization_name\": \"$TENANT_ORG_NAME\",
      \"owner_first_name\": \"Admin\",
      \"owner_last_name\": \"User\",
      \"owner_email\": \"$TENANT_EMAIL\",
      \"distribution_account_type\": \"DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT\",
      \"base_url\": \"$TENANT_BASE_URL\",
      \"sdp_ui_base_url\": \"$TENANT_UI_BASE_URL\"
    }"
    echo "Tenant created successfully."
else
    echo "Tenant $TENANT_NAME already exists."
fi

# Add a user with password
echo "Adding user for tenant: $TENANT_NAME"
echo "Password123!" | go run ../main.go auth add-user "owner@${TENANT_NAME}.local" "Admin" "User" --password --owner --roles "owner" --tenant-id "$TENANT_EXISTS"

echo "User added successfully."
