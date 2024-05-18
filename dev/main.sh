#!/bin/bash
# This script is used to locally start the integration between SDP and AnchorPlatform for the SEP-24 deposit flow, needed for registering users.
set -eu

# Check if curl is installed
if ! command -v curl &> /dev/null
then
    echo "Error: curl is not installed. Please install curl to continue."
    exit 1
fi

export DIVIDER="----------------------------------------"

# prepare
echo "====> 👀Step 1: start preparation"
docker ps -aq | xargs docker stop | xargs docker rm
echo "====> ✅Step 1: finish preparation"

# Run docker compose
echo $DIVIDER
echo "====> 👀Step 2: start calling docker compose up"
docker-compose down && docker-compose -p sdp-multi-tenant up -d --build
echo "====> ✅Step 2: finish calling docker-compose up"

# Initialize tenants
echo $DIVIDER
echo "====> 👀Step 3: initialize tenants... (😴 10s sleep)"

# Wait for docker containers to start
sleep 10
AdminTenantURL="http://localhost:8003/tenants"

# Initialize tenants
tenants=("redcorp" "bluecorp" "pinkcorp")

# Create missing tenants
adminAccount="SDP-admin"
adminApiKey="api_key_1234567890"
encodedCredentials=$(echo -n "$adminAccount:$adminApiKey" | base64)
AuthHeader="Authorization: Basic $encodedCredentials"

existingTenants=$(curl -s -H "$AuthHeader" $AdminTenantURL)
echo "Response from GET /tenants: $existingTenants"

existingTenantNames=[]
if names=$(echo $existingTenants | jq -r '.[].name'); then
    if [ -n "$names" ]; then  # Only assign if names is non-empty
        existingTenantNames=($names)
    fi
fi
echo "existingTenantNames: $existingTenantNames"

for tenant in "${tenants[@]}"; do
    # Check if the tenant already exists
    if printf '%s\n' "${existingTenantNames[@]}" | grep -q "^$tenant$"; then
        echo "🔵Tenant $tenant already exists. Skipping."
    else
        echo "🐈Provisioning missing tenant: $tenant"
        baseURL="http://$tenant.stellar.local:8000"
        sdpUIBaseURL="http://$tenant.stellar.local:3000"
        ownerEmail="owner@$tenant.org"

        response=$(curl -s -w "\n%{http_code}" -X POST $AdminTenantURL \
                -H "Content-Type: application/json" \
                -H "$AuthHeader" \
                -d '{
                        "name": "'"$tenant"'",
                        "organization_name": "'"$tenant"'",
                        "base_url": "'"$baseURL"'",
                        "sdp_ui_base_url": "'"$sdpUIBaseURL"'",
                        "owner_email": "'"$ownerEmail"'",
                        "owner_first_name": "jane",
                        "owner_last_name": "doe"
                }')

        http_code=$(echo "$response" | tail -n1)
        response_body=$(echo "$response" | sed '$d')

        if [[ "$http_code" -ge 200 && "$http_code" -lt 300 ]]; then
            echo "✅Tenant $tenant created successfully."
            echo "🔗You can now reset the password for the owner $ownerEmail on $sdpUIBaseURL/forgot-password"
            echo "Response body: $response_body"
        else
            echo "❌Failed to create tenant $tenant. HTTP status code: $http_code"
            echo "Server response: $response_body"
        fi
    fi
done

echo "====> ✅Step 3: finished initialization of tenants"
echo $DIVIDER
echo "🎉🎉🎉🎉 SUCCESS! 🎉🎉🎉🎉"
