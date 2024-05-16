# Check if .env already exists
echo "====> 👀 Checking if .env environment file exists in $(pwd)"
if [ ! -f ".env" ]; then
    GO_EXECUTABLE="go run ./scripts/create_and_fund.go"
    echo ".env file does not exist. Creating in $(pwd)."

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

    echo ".env file created successfully in $(pwd)."
else
    echo ".env file already exists in $(pwd). Skipping creation."
fi
echo "====> ✅ Finished .env setup"

