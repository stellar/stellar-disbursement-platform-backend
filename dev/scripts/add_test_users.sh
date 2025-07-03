#!/bin/sh
echo "running $0 from directory $(pwd)"

# Credentials for Basic Auth
USERNAME="SDP-admin"
PASSWORD="api_key_1234567890"

# Retrieve tenant details using wget with Basic Auth from an API endpoint
AUTH_HEADER="Authorization: Basic $(echo -n "$USERNAME:$PASSWORD" | base64)"
TENANTS_JSON=$(wget --quiet --header="$AUTH_HEADER" --output-document=- http://localhost:8003/tenants/)

if [ -z "$TENANTS_JSON" ]; then
  echo "No tenant details could be retrieved. Exiting."
  exit 1
fi

# Function to add a user for  tenant
add_user_for_tenant() {
  echo
  local tenant_id=$1
  local tenant_name=$2
  echo "Adding owner user to tenant: $tenant_name (ID: $tenant_id)"
  echo "Password123!" |  ./stellar-disbursement-platform auth add-user "owner@${tenant_name}.local" "john" "doe" --password  --owner --roles "owner" --tenant-id "${tenant_id}"

} 

# Loop through each tenant and add an owner user
echo "$TENANTS_JSON" | jq -c '.[]' | while read -r tenant; do
  tenant_id=$(echo "$tenant" | jq -r '.id')
  tenant_name=$(echo "$tenant" | jq -r '.name' | tr -d '[:space:]') # Remove spaces from tenant name for usernames and emails

  add_user_for_tenant "$tenant_id" "$tenant_name"
done

echo "Owner users added successfully to all tenants."


