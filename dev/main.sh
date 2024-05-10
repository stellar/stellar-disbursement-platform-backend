#!/bin/bash
# This script is used to locally start the integration between SDP and AnchorPlatform for the SEP-24 deposit flow, needed for registering users.
set -eu

export DIVIDER="----------------------------------------"
# Check if curl is installed
if ! command -v curl &> /dev/null
then
    echo "Error: curl is not installed. Please install curl to continue."
    exit 1
fi



# prepare

echo $DIVIDER
echo "====> 👀 Step 2: start calling docker-compose up"
docker-compose -p sdp-multi-tenant down
echo "====> ✅ Step 2: finish calling docker-compose up"

# Check if "--delete_pv" is passed as a parameter
if [[ " $@ " =~ " --delete_pv " ]]; then
    echo "You have opted to delete persistent volumes sdp-multi-tenant_kafka-data sdp-multi-tenant_postgres-ap-db sdp-multi-tenant_postgres-db. Are you sure? (yes/no)"
    read -r confirmation
    if [ "$confirmation" == "yes" ]; then
        echo "Deleting persistent volumes..."
        docker volume rm sdp-multi-tenant_kafka-data sdp-multi-tenant_postgres-ap-db sdp-multi-tenant_postgres-db
    else
        echo "Persistent volumes will not be deleted."
    fi
fi

# Check if .env already exists
if [ ! -f ".env" ]; then
    GO_EXECUTABLE="go run ./scripts/create_and_fund.go"
    echo ".env file does not exist. creating."

    # Function to run Go script and extract keys
    function generate_keys() {
        # Run the Go script with the necessary arguments
        if [ "$1" == "nop" ]; then
            output=$($GO_EXECUTABLE -fundxlm=true)
        else
            output=$($GO_EXECUTABLE -fundxlm=true -fundusdc=true -xlm_amount="20")
        fi
        echo "$output"
    }

    # Generate keys for SEP-10 without funding
    echo "Generating SEP-10 signing keys..."
    sep10_output=$(generate_keys "nop")
    sep10_public=$(echo "$sep10_output" | grep 'Public Key:' | awk '{print $3}')
    sep10_private=$(echo "$sep10_output" | grep 'Secret Key:' | awk '{print $3}')

    # Generate keys for distribution with funding
    echo "Generating distribution keys with funding..."
    distribution_output=$(generate_keys "with_funding")
    distribution_public=$(echo "$distribution_output" | grep 'Public Key:' | awk '{print $3}')
    distribution_private=$(echo "$distribution_output" | grep 'Secret Key:' | awk '{print $3}')

    # Create .env file with the extracted values
    cat << EOF > .env
    # Generate a new keypair for SEP-10 signing
    SEP10_SIGNING_PUBLIC_KEY=$sep10_public
    SEP10_SIGNING_PRIVATE_KEY=$sep10_private

    # Generate a new keypair for the distribution account
    DISTRIBUTION_PUBLIC_KEY=$distribution_public
    DISTRIBUTION_SEED=$distribution_private

    # CHANNEL_ACCOUNT_ENCRYPTION_PASSPHRASE
    CHANNEL_ACCOUNT_ENCRYPTION_PASSPHRASE=$distribution_private

    # Distribution signer
    DISTRIBUTION_SIGNER_TYPE=DISTRIBUTION_ACCOUNT_ENV
    DISTRIBUTION_ACCOUNT_ENCRYPTION_PASSPHRASE=$distribution_private
EOF

    echo ".env file created successfully."
fi


docker-compose -p sdp-multi-tenant up -d --build

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
                "password": "Password123!"
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
