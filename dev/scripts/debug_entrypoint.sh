#!/bin/bash
set -e

# Running migrations
echo "Running DB migrations and setup..."
./stellar-disbursement-platform db admin migrate up
./stellar-disbursement-platform db tss migrate up
./stellar-disbursement-platform db auth migrate up --all
./stellar-disbursement-platform db sdp migrate up --all
./stellar-disbursement-platform db setup-for-network --all

# Starting the main process using original entrypoint
echo "starting dlv stellar-disbursement-platform"
/go/bin/dlv exec ./stellar-disbursement-platform serve --headless --listen=:2345 --api-version=2 --log
