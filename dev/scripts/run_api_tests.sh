#!/bin/bash

# Exit immediately if a command exits with a non-zero status.
set -e

# Define directories and log files
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
LOG_DIR="${PROJECT_ROOT}/logs"
BACKEND_LOG="${LOG_DIR}/backend.log"

# Ensure log directory exists
mkdir -p "${LOG_DIR}"

# Define API endpoints and credentials
BACKEND_HEALTH_URL="http://localhost:8000/health"
ADMIN_API_URL="http://localhost:8003"
TENANT_API_URL="${ADMIN_API_URL}/tenants"
FORGOT_PASSWORD_API_URL="http://localhost:8000/forgot-password"
RESET_PASSWORD_API_URL="http://localhost:8000/reset-password"
LOGIN_API_URL="http://localhost:8000/login"

ADMIN_USER="SDP-admin"
ADMIN_PASS="api_key_1234567890"
ADMIN_AUTH_HEADER="Authorization: Basic $(echo -n "${ADMIN_USER}:${ADMIN_PASS}" | base64)"

# Define Test Tenant Details
TENANT_NAME="test-tenant"
TENANT_ORG_NAME="Test Org"
TENANT_EMAIL="test@example.com"
TENANT_BASE_URL="http://localhost:8000"
TENANT_UI_BASE_URL="http://localhost:3000"
NEW_PASSWORD="NewSecurePassword123!"

# Log file for test results
TEST_LOG="${LOG_DIR}/api_tests.log"

# Redirect all output to log file
exec > >(tee -a "${TEST_LOG}") 2>&1

echo "====> Starting Stellar Disbursement Platform API Tests $(date)"

# --- Helper Functions ---

check_health() {
    echo "--- Checking Service Health ---"
    echo "Checking Backend Health (${BACKEND_HEALTH_URL})..."
    if curl -s --fail "${BACKEND_HEALTH_URL}"; then
        echo "Backend Health: OK"
    else
        echo "ERROR: Backend Health Check FAILED." >&2
        exit 1
    fi
}

add_tenant() {
    echo "--- Adding Test Tenant (${TENANT_NAME}) ---"
    local tenant_payload
    tenant_payload=$(cat <<EOF
{
  "name": "${TENANT_NAME}",
  "organization_name": "${TENANT_ORG_NAME}",
  "owner_first_name": "Test",
  "owner_last_name": "User",
  "owner_email": "${TENANT_EMAIL}",
  "distribution_account_type": "DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT",
  "base_url": "${TENANT_BASE_URL}",
  "sdp_ui_base_url": "${TENANT_UI_BASE_URL}"
}
EOF
    )

    echo "Attempting to add tenant..."
    http_code=$(curl -s -w "%{http_code}" -X POST "${TENANT_API_URL}" \
        -H "Content-Type: application/json" \
        -H "${ADMIN_AUTH_HEADER}" \
        -d "${tenant_payload}" -o /dev/null)

    if [ "${http_code}" -eq 201 ]; then
        echo "Tenant '${TENANT_NAME}' created successfully (HTTP ${http_code})."
    elif [ "${http_code}" -eq 409 ]; then
        echo "Tenant '${TENANT_NAME}' already exists (HTTP ${http_code}). Proceeding..."
    else
        echo "ERROR: Failed to add tenant '${TENANT_NAME}'. HTTP Status Code: ${http_code}" >&2
        # Optionally show response body if needed for debugging
        # curl -v -X POST "${TENANT_API_URL}" -H "Content-Type: application/json" -H "${ADMIN_AUTH_HEADER}" -d "${tenant_payload}"
        exit 1
    fi
}

trigger_password_reset() {
    echo "--- Triggering Password Reset for ${TENANT_EMAIL} ---"
    # This assumes a simple POST request. Adjust payload/method as needed.
    local reset_payload
    reset_payload=$(cat <<EOF
{
  "email": "${TENANT_EMAIL}",
  "organization_name": "${TENANT_ORG_NAME}" 
}
EOF
    )
    
    echo "Sending forgot password request..."
    # Note: This might fail due to reCAPTCHA if not disabled via env var DISABLE_RECAPTCHA=true
    http_code=$(curl -s -w "%{http_code}" -X POST "${FORGOT_PASSWORD_API_URL}" \
        -H "Content-Type: application/json" \
        -d "${reset_payload}" -o /dev/null)

    if [ "${http_code}" -eq 200 ] || [ "${http_code}" -eq 204 ]; then
        echo "Forgot password request sent successfully (HTTP ${http_code}). Check backend logs for token."
    else
        echo "WARNING: Forgot password request failed or returned unexpected status: ${http_code}. reCAPTCHA might be enabled or endpoint/payload incorrect." >&2
        # Continue anyway, maybe token was generated previously
    fi
}

extract_reset_token() {
    echo "--- Extracting Reset Token from Logs (${BACKEND_LOG}) ---"
    echo "Waiting a few seconds for log propagation..."
    sleep 5

    # Adjust grep pattern based on the actual log message format for the reset token
    # This is a placeholder pattern
    RESET_TOKEN=$(grep -oP "Password reset token for ${TENANT_EMAIL}: \K[a-zA-Z0-9_-]+" "${BACKEND_LOG}" | tail -n 1)

    if [ -z "${RESET_TOKEN}" ]; then
        echo "ERROR: Could not find password reset token in ${BACKEND_LOG}." >&2
        echo "Please ensure EMAIL_SENDER_TYPE=DRY_RUN is set and check the log format." >&2
        exit 1
    fi
    echo "Reset token found: ${RESET_TOKEN}" # Avoid printing the actual token in production logs if sensitive
}

reset_password() {
    echo "--- Resetting Password using Token ---"
    local reset_payload
    reset_payload=$(cat <<EOF
{
  "token": "${RESET_TOKEN}",
  "password": "${NEW_PASSWORD}"
}
EOF
    )

    echo "Submitting password reset request..."
    http_code=$(curl -s -w "%{http_code}" -X POST "${RESET_PASSWORD_API_URL}" \
        -H "Content-Type: application/json" \
        -d "${reset_payload}" -o /dev/null)

    if [ "${http_code}" -eq 200 ] || [ "${http_code}" -eq 204 ]; then
        echo "Password reset successful (HTTP ${http_code})."
    else
        echo "ERROR: Password reset failed. HTTP Status Code: ${http_code}" >&2
        exit 1
    fi
}

test_login() {
    echo "--- Testing Login with New Password ---"
    local login_payload
    login_payload=$(cat <<EOF
{
  "email": "${TENANT_EMAIL}",
  "password": "${NEW_PASSWORD}"
}
EOF
    )

    echo "Attempting login..."
    # This assumes basic auth or a session cookie is returned. Adjust as needed.
    http_code=$(curl -s -w "%{http_code}" -X POST "${LOGIN_API_URL}" \
        -H "Content-Type: application/json" \
        -d "${login_payload}" -o /dev/null)
        # Add -c cookie.txt -b cookie.txt for session handling if needed

    if [ "${http_code}" -eq 200 ]; then
        echo "Login successful (HTTP ${http_code})."
    else
        echo "ERROR: Login failed. HTTP Status Code: ${http_code}" >&2
        exit 1
    fi
}

# --- Main Execution ---

check_health
add_tenant
trigger_password_reset
extract_reset_token
reset_password
test_login

echo "====> Stellar Disbursement Platform API Tests Completed Successfully $(date)"

exit 0