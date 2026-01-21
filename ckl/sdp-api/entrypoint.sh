#!/bin/sh
./stellar-disbursement-platform db admin migrate up
./stellar-disbursement-platform db tss migrate up
./stellar-disbursement-platform db auth migrate up --all
./stellar-disbursement-platform db sdp migrate up --all
./stellar-disbursement-platform db setup-for-network --all
./stellar-disbursement-platform channel-accounts ensure ${NUM_CHANNEL_ACCOUNTS}
./stellar-disbursement-platform serve
