# Quick Start Guide - First Disbursement

## Table of Contents
  - [Introduction](#introduction)
  - [Prerequisites](#prerequisites)
  - [Setup](#setup)
    - [Build Docker Containers](#build-docker-containers)
    - [Create an Owner SDP User](#create-an-owner-sdp-user)
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
docker-compose up
```

This will spin up the following services:

- `sdp_v2_database`: The main SDP and TSS database.
- `anchor-platform-postgres-db`: Database used by the anchor platform.
- `anchor-platform`: A local instance of the anchor platform.
- `sdp-api`: SDP service running on port `8000`.
- `sdp-tss`: Transaction Submission service.
- `sdp-frontend`: SDP frontend service running on port `3000`.

### Create an Owner SDP User

Open a terminal for the `sdp-api` container and run the following command to create an owner user:

```sh
docker exec -it sdp-api bash # Or use Docker Desktop to open terminal
./stellar-disbursement-platform auth add-user owner@stellar.org joe yabuki --password --owner --roles owner
```

You will be prompted to enter a password for the user. Be sure to remember it as it will be required for future authentications.

## Disbursement

### Create First Disbursement

Navigate to the frontend service by opening a browser and going to [localhost:3000](http://localhost:3000).

- Click `New Disbursement+` on the Dashboard screen.
- Use `Demo Wallet` as your wallet.
- Upload a disbursement file. A sample file is available `./dev/sample/sample-disbursement.csv`. Make sure to update the invalid phone number before using it.
- Finally, confirm the disbursement.

### Deposit Money

To deposit money into your account:

- Access [https://demo-wallet.stellar.org/](https://demo-wallet.stellar.org/) in your browser.
- Click on `Generate Keypair for new account` to create a new testnet receiver account. Make sure to save your public key & secret.
- Add an Asset with the following information:
  - Asset Code: `USDC`
  - Anchor Home Domain: `localhost:8080`
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
