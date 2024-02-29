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

adminAccount="SDP-admin"
adminApiKey="api_key_1234567890"
encodedCredentials=$(echo -n "$adminAccount:$adminApiKey" | base64)
AuthHeader="Authorization: Basic $encodedCredentials"

existingTenants=$(curl -s -H "$AuthHeader" $AdminTenantURL)
echo "Response from tenant check: $existingTenants"

if [ "$existingTenants" == "[]" ]; then
    echo "No existing tenants found. Initializing new tenants..."

    # Initialize tenants
    tenants=("redcorp" "bluecorp")

    for tenant in "${tenants[@]}"
    do
        echo "🐈Provisioning tenant: $tenant"
        baseURL="http://$tenant.stellar.local:8000"
        sdpUIBaseURL="http://$tenant.stellar.local:3000"
        ownerEmail="john.doe@$tenant.org"

        curl -X POST $AdminTenantURL \
        -H "Content-Type: application/json" \
        -H "$AuthHeader" \
        -d '{
                "name": "'"$tenant"'",
                "organization_name": "'"$tenant"'",
                "email_sender_type": "DRY_RUN",
                "sms_sender_type": "DRY_RUN",
                "enable_mfa": false,
                "enable_recaptcha": false,
                "base_url": "'"$baseURL"'",
                "sdp_ui_base_url": "'"$sdpUIBaseURL"'",
                "cors_allowed_origins": ["*"],
                "owner_email": "'"$ownerEmail"'",
                "owner_first_name": "john",
                "owner_last_name": "doe"
        }'

        echo "✅Tenant $tenant created successfully."
        echo "🔗You can now reset the password for the owner $ownerEmail on $sdpUIBaseURL/forgot-password"
    done
else
    echo "🛑Existing tenants found. Skipping initialization."
fi

echo "====> ✅Step 3: finished initialization of tenants"
echo $DIVIDER
echo "🎉🎉🎉🎉 SUCCESS! 🎉🎉🎉🎉"
