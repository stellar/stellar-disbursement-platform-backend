#!/bin/bash
# This script is used to locally start the integration between SDP and AnchorPlatform for the SEP-24 deposit flow, needed for registering users.
set -eu

export DIVIDER="----------------------------------------"
# Function to display help
display_help() {
    echo "Usage: $0 [options]"
    echo
    echo "Options:"
    echo "  --help            Show this help message and exit."
    echo "  --delete_pv       Delete persistent volumes for SDP databases."
}

# Check if --help is passed as an argument
if [[ " $@ " =~ " --help " ]]; then
    display_help
    exit 0
fi

# Check if curl is installed
if ! command -v curl &> /dev/null
then
    echo "Error: curl is not installed. Please install curl to continue."
    exit 1
fi

# prepare
echo $DIVIDER
echo "====> 👀 start calling docker-compose -p sdp-multi-tenant down"
docker ps -aq | xargs docker stop | xargs docker rm
#docker-compose -p sdp-multi-tenant down
docker-compose down
echo "====> ✅ finish calling docker-compose down"

# Run docker compose
echo $DIVIDER

# Check if "--delete_pv" is passed as a parameter
if [[ " $@ " =~ " --delete_pv " ]]; then
    echo "====> 👀 deleting persistent volumes sdp-multi-tenant_kafka-data sdp-multi-tenant_postgres-ap-db sdp-multi-tenant_postgres-db"
    
    # Function to delete volume if it exists
    delete_volume() {
        local volume_name=$1
        if docker volume inspect "$volume_name" &> /dev/null; then
            docker volume rm "$volume_name"
            echo "====> ✅ volume $volume_name deleted"
        else
            echo "====> ⚠️ volume $volume_name does not exist"
        fi
    }

    # Delete volumes
    delete_volume "sdp-multi-tenant_kafka-data"
    delete_volume "sdp-multi-tenant_postgres-ap-db"
    delete_volume "sdp-multi-tenant_postgres-db"
fi

echo $DIVIDER
echo "====> 👀calling docker compose up"
export GIT_COMMIT="debug"
docker-compose -p sdp-multi-tenant up -d --build

# Run docker compose
echo $DIVIDER
echo "====> ✅finish calling docker-compose up"


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
echo "Response from tenant check: $existingTenants"

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
        ownerEmail="init_owner@$tenant.org"

        curl -X POST $AdminTenantURL \
        -H "Content-Type: application/json" \
        -H "$AuthHeader" \
        -d '{
                "name": "'"$tenant"'",
                "organization_name": "'"$tenant"'",
                "email_sender_type": "DRY_RUN",
                "sms_sender_type": "DRY_RUN",
                "base_url": "'"$baseURL"'",
                "sdp_ui_base_url": "'"$sdpUIBaseURL"'",
                "owner_email": "'"$ownerEmail"'",
                "owner_first_name": "jane",
                "owner_last_name": "doe",
                "is_default": true
        }'
        echo "✅Tenant $tenant created successfully."
        echo "🔗Note: You can reset the password for the owner $ownerEmail on $sdpUIBaseURL/forgot-password"
    fi
done

echo "====> ✅Step 3: finished initialization of tenants"
echo $DIVIDER
# Initialize test_users
echo "====> Step 4: initialize test users..."
docker-compose -p sdp-multi-tenant exec sdp-api ./dev/scripts/add_test_users.sh
echo $DIVIDER

echo "🎉🎉🎉🎉 SUCCESS! 🎉🎉🎉🎉"
echo "Login URLs for each tenant:"
for tenant in "${tenants[@]}"; do
    echo "🔗Tenant $tenant: http://$tenant.stellar.local:3000"
    echo "username: owner@$tenant.org  password: Password123!"
done
