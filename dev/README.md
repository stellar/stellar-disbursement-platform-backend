# Quick Start Guide - First Disbursement

## Table of Contents
  - [Introduction](#introduction)
  - [Prerequisites](#prerequisites)
  - [Setup](#setup)
    - [Build Docker Containers](#build-docker-containers)
    - [New Tenant Provisioning Process](#new-tenant-provisioning-process)
    - [Setup Owner User Password for each tenant](#setup-owner-user-password-for-each-tenant)
  - [Disbursement](#disbursement)
    - [Create First Disbursement](#create-first-disbursement)
    - [Deposit Money](#deposit-money)
  - [Troubleshooting](#troubleshooting)

## Introduction

Follow these instructions to get started with the Stellar Disbursement Platform (SDP).

## Prerequisites

### Docker

Make sure you have Docker installed on your system. If not, you can download it from [here](https://www.docker.com/products/docker-desktop).

### Stellar accounts 
We will need to create and configure two Stellar accounts to be able to use the SDP. 
* A Distribution account that will be used for sending funds to receivers. [Create and Fund a Distribution Account](https://developers.stellar.org/docs/stellar-disbursement-platform/getting-started#create-and-fund-a-distribution-account)
* A SEP-10 account that will be used for authentication. It can be created the same way as the distribution account but it doesn't need to be funded.

The public and private key of these two accounts will be used to configure the SDP in the next step.

## Setup

### Build Docker Containers

1. Navigate to the `dev` directory from the terminal:
```sh
cd dev
```

2. Create a `.env` file in the `dev` directory by copying the `.env.example` file:
```sh
cp .env.example .env
```

3. Update the `.env` file with the public and private keys of the two accounts created in the previous step.

4. Execute the following command to create all the necessary Docker containers needed to run SDP:
```sh
./main.sh
```

5. In order to start the sdp containers with monitoring services, run the following command:
```sh
docker-compose -f docker-compose.yml -f docker-compose-monitoring.yml up
```

This will spin up the following services:

- `sdp_v2_database`: The main SDP and TSS database.
- `anchor-platform-postgres-db`: Database used by the anchor platform.
- `anchor-platform`: A local instance of the anchor platform.
- `sdp-api`: SDP service running on port `8000`.
- `sdp-tss`: Transaction Submission service.
- `sdp-frontend`: SDP frontend service running on port `3000`.
- `kafka`: Kafka service running on ports `9092`, `9094`(external).
- `kafka-init`:  Initial workflow to exec into the Kafka container and create topics.

The following are optional monitoring services that can be started through `docker-compose-monitoring.yml` and are primarily used for monitoring Kafka: 
- `db-conduktor`: Database instance for the Conduktor service. 
- `conduktor-monitoring`: Conduktor Monitoring service integrated into the Conduktor Platform. 
- `conduktor-platform`: Provides solutions for Kafka management, testing, monitoring, data quality, security, and data governance.

### New Tenant Provisioning Process

When you ran `main.sh` file, you've already created new tenants: `tenants=("redcorp" "bluecorp")`. 
To add more tenants, simply append them separated by spaces to that variable like so: `tenants=("redcorp" "bluecorp" "greencorp")` and run `main.sh` again.

Be sure that the added tenant hosts are included in the host configuration file.
To check it, you can run the command `cat /etc/hosts`.
To include them, you can run command `sudo nano /etc/hosts` and insert the lines below:
```
127.0.0.1       bluecorp.sdp.local
127.0.0.1       redcorp.sdp.local
```

### Setup Owner User Password for each tenant

Go through the forgot password flow to be able to login as an owner user.

Go to Forgot Password page on `http://${tenant}.stellar.local:3000/forgot-password` and enter the tenant and owner email `owner@${tenant}.org`.

A token will be generated, and it's possible to check it on `sdp-api` logs. This token will be needed to Reset Password on `http://${tenant}.stellar.local:3000/reset-password`.

## Disbursement

### Create First Disbursement

> [!NOTE]  
> In the following section, we will assume you're using the `bluecorp` tenant that was provisioned by default when you ran `main.sh`.

Navigate to the frontend service by opening a browser and going to [http://bluecorp.stellar.local:3000](http://bluecorp.stellar.local:3000).

- Click `New Disbursement+` on the Dashboard screen.
- Use `Demo Wallet` as your wallet.
- Upload a disbursement file. A sample file is available `./dev/sample/sample-disbursement.csv`. Make sure to update the invalid phone number before using it.
- Finally, confirm the disbursement.

### Deposit Money

To deposit money into your account:

- Access [demo-wallet](http://localhost:4000) in your browser.
- Click on `Generate Keypair for new account` to create a new testnet receiver account. Make sure to save your public key & secret.
- Add an Asset with the following information:
  - Asset Code: `USDC`
  - Anchor Home Domain: `http://bluecorp.stellar.local:8000`
  - Issuer Public Key: `GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5`
- Click `Create Account` (in front of public key) and add Trustline for USDC.
- For USDC, select `SEP-24 Deposit`.
- In the new window, enter the phone number from the disbursement CSV.
- Enter the passcode. You can use `000000` passcode or find the actual passcode in the `sdp-api` container logs.
- Enter the birthday that matches the phone number in the CSV.
- Keep an eye on the dashboard until the payment status reaches `Success`. If everything was set up correctly, your money should be disbursed successfully.

## Troubleshooting

### Distribution account out of funds

Payments will start failing if the distribution account runs out of funds. To fix this, you can either write a script that funds the distribution account or use the tools
available to add more funds to the distribution account by following these steps:

- Find the distribution account public key in `dev/docker-compose.yml` under the variable `DISTRIBUTION_PUBLIC_KEY`
- Access [https://horizon-testnet.stellar.org/accounts/:accountId](https://horizon-testnet.stellar.org/accounts/GARGKDIDH7WMKV5WWPK4BH4CKEQIZGWUCA4EUXCY5VICHTHLEBXVNVMW) in your browser and check the balance.
- If the balance is indeed low, you can add more funds by creating a new account and sending funds to the distribution account.
  - Access [https://demo-wallet.stellar.org/](https://demo-wallet.stellar.org/) in your browser.
  - Click on `Generate Keypair for new account` to create a new testnet account. Your account comes with 10,000 XLM.
  - Click on `Send` and enter the distribution account public key and the amount you want to send.
  - Using Freighter or Stellar Laboratory, swap the XLM for USDC.

You can also just use the newly created account as the distribution account by updating the `DISTRIBUTION_PUBLIC_KEY` variable in `dev/docker-compose.yml` and restarting the `sdp-api` container.
