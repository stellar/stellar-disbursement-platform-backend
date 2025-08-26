#!/bin/bash
# This script is used to locally start the SDP environment for wallet registration and payment processing.
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

if [ ! -f ./.env ]; then
    echo ".env file is required but not found in the current directory."
    echo "You can create one using scripts/make_env.sh"
    echo "Refer to the README.md for more details."
    exit 1
fi

declare -a required_tools=( docker curl jq )
declare failed=0

for tool in ${required_tools[@]}; do
    if ! command -v $tool &> /dev/null
    then
        echo "Error: $tool is not installed. Please install $tool to continue."
        failed=1
    fi
done

if [[ $failed != 0 ]]; then
    exit 1
fi

# prepare
echo $DIVIDER
echo "====> 👀 start calling docker compose -p sdp-multi-tenant down"
#docker compose -p sdp-multi-tenant down
docker compose -p sdp-multi-tenant down
echo "====> ✅ finish calling docker compose down"

# Run docker compose
echo $DIVIDER

# Check if "--delete_pv" is passed as a parameter
if [[ " $@ " =~ " --delete_pv " ]]; then
    echo "====> 👀 deleting persistent volumes sdp-multi-tenant_postgres-db"
    
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
    delete_volume "sdp-multi-tenant_postgres-db"
fi

echo $DIVIDER
echo "====> 👀calling docker compose up"
export GIT_COMMIT="debug"
docker compose -p sdp-multi-tenant up -d --build

# Run docker compose
echo $DIVIDER
echo "====> ✅finish calling docker compose up"


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
        ownerEmail="init_owner@$tenant.local"

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
                        "owner_last_name": "doe",
                        "distribution_account_type": "DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT"
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
# Initialize test_users
echo "====> Step 4: initialize test users..."
docker compose -p sdp-multi-tenant exec sdp-api ./dev/scripts/add_test_users.sh
echo $DIVIDER

echo "🎉🎉🎉🎉 SUCCESS! 🎉🎉🎉🎉"
echo "Login URLs for each tenant:"
for tenant in "${tenants[@]}"; do
    url="http://$tenant.stellar.local:3000"
    echo -e "🔗Tenant $tenant: \033]8;;$url\033\\$url\033]8;;\033\\"
    echo "username: init_owner@$tenant.local  password: Password123!"

    if ! grep -q $tenant /etc/hosts; then
        echo >&2 "WARN $tenant.stellar.local missing from /etc/hosts"
    fi
done

