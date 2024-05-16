# Quick Start Guide - First Disbursement

## Table of Contents
- [Quick Start Guide - First Disbursement](#quick-start-guide---first-disbursement)
  - [Table of Contents](#table-of-contents)
  - [Introduction](#introduction)
  - [Prerequisites](#prerequisites)
    - [Docker](#docker)
    - [Stellar Accounts and .env File](#stellar-accounts-and-env-file)
    - [Stellar Accounts and .env File](#stellar-accounts-and-env-file-1)
  - [Setup](#setup)
    - [Build Docker Containers](#build-docker-containers)
    - [New Tenant Provisioning Process](#new-tenant-provisioning-process)
    - [Login Information](#login-information)
  - [Disbursement](#disbursement)
    - [Create First Disbursement](#create-first-disbursement)
    - [Deposit Money](#deposit-money)
  - [Troubleshooting](#troubleshooting)
      - [Sample Tenant Management Postman collection](#sample-tenant-management-postman-collection)
      - [Distribution account out of funds](#distribution-account-out-of-funds)
      - [Remote Debugging](#remote-debugging)

## Introduction

Follow these instructions to get started with the Stellar Disbursement Platform (SDP).

## Prerequisites

### Docker

Make sure you have Docker installed on your system. If not, you can download it from [here](https://www.docker.com/products/docker-desktop).

### Stellar Accounts and .env File
### Stellar Accounts and .env File

You need to create and configure two Stellar accounts to use the SDP. You can either create the accounts manually or use the provided script to automate the process.

**Option 1: Manually Create and Configure Accounts**

1. Create and fund a Distribution account that will be used for sending funds to receivers. Follow the instructions [here](https://developers.stellar.org/docs/stellar-disbursement-platform/getting-started#create-and-fund-a-distribution-account).
2. Create a SEP-10 account for authentication. It can be created the same way as the distribution account but it doesn't need to be funded.
3. Create a `.env` file in the `dev` directory by copying the [env.example](./.env.example) file:
    ```sh
    cp .env.example .env
    ```
4. Update the `.env` file with the public and private keys of the two accounts created in the previous steps.

**Option 2: Use create_env.sh script to create accounts and .env file**

You can use the make_env.sh script to automatically create a SEP-10 account funded with XLM using Friendbot, create a distribution account, and fund it with USDC by establishing a trustline and executing a path payment.  to run the make_env.sh script

1. Use [make_env.sh](./scripts/make_env.sh) script to create stellar accounts and .env file automatically:
    1. Navigate to the `dev` directory from the terminal:
    ```sh
    cd dev
    ```
    2. Run the `make_env.sh` in the `scripts` folder.
    ```sh
    scripts/make_env.sh
    ```
    You should see output as follows:
    ```
    ❯ scripts/make_env.sh
    ====> 👀 Checking if .env environment file exists in <REPO_ROOT>/stellar-disbursement-platform-backend/dev
    .env file does not exist. Creating
    Generating SEP-10 signing keys...
    Generating distribution keys with funding...
    .env file created successfully 
    ====> ✅ Finished .env setup
    ```

## Setup

### Build Docker Containers

A main.sh wrapper script has been included to help you bring up a local environment. The script stops and removes existing Docker containers, optionally deletes persistent volumes, and then uses Docker Compose to bring up new containers for the Stellar Disbursement Platform (SDP). This includes the SDP, Anchor Platform (for user registration), PostgreSQL database, Kafka for event handling, and a local demo wallet instance. It then initializes tenants if they don't exist and adds test users, setting up the local environment for the SEP-24 deposit flow.

1. Execute the following command to create all the necessary Docker containers needed to run SDP as well as provision sample tenants:
```sh
./main.sh
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
- `demo-wallet`: The demo wallet client that will be used as a receiver wallet, running on port `4000`.

The following are optional monitoring services that can be started through `docker-compose-monitoring.yml` and are primarily used for monitoring Kafka: 
- `db-conduktor`: Database instance for the Conduktor service. 
- `conduktor-monitoring`: Conduktor Monitoring service integrated into the Conduktor Platform. 
- `conduktor-platform`: Provides solutions for Kafka management, testing, monitoring, data quality, security, and data governance.


> [!TIP]  
> If you wish to start the sdp containers with monitoring services, you can use the docker-compose-monitoring.yml file 
> 
> `docker-compose -f docker-compose.yml -f docker-compose-monitoring.yml up`


### New Tenant Provisioning Process

When you ran `main.sh` file, you've already created new tenants: `tenants=("redcorp" "bluecorp")`. 
To add more tenants, simply append them separated by spaces to that variable like so: `tenants=("redcorp" "bluecorp")` and run `main.sh` again.

Be sure that the added tenant hosts are included in the host configuration file.
To check it, you can run the command `cat /etc/hosts`.
To include them, you can run command `sudo nano /etc/hosts` and insert the lines below:
```
127.0.0.1       bluecorp.sdp.local
127.0.0.1       redcorp.sdp.local
127.0.0.1       pinkcorp.sdp.local
```

### Login Information

Owner accounts and emails have been set up as follows:

```
🎉🎉🎉🎉 SUCCESS! 🎉🎉🎉🎉  
Login URLs for each tenant:
🔗Tenant `redcorp`: [http://redcorp.stellar.local:3000](http://redcorp.stellar.local:3000)
  username: `owner@redcorp.org`
  password: `Password123!`
🔗Tenant `bluecorp`: [http://bluecorp.stellar.local:3000](http://bluecorp.stellar.local:3000)
  username: `owner@bluecorp.org`
  password: `Password123!`
🔗Tenant `pinkcorp`: [http://pinkcorp.stellar.local:3000](http://pinkcorp.stellar.local:3000)
  username: `owner@pinkcorp.org`
  password: `Password123!`
```

## Disbursement

### Create First Disbursement

> [!NOTE]  
> In the following section, we will assume you're using the `bluecorp` tenant that was provisioned by default when you ran `main.sh`.

Navigate to the frontend service by opening a browser and going to [http://bluecorp.stellar.local:3000](http://bluecorp.stellar.local:3000).

- Click `New Disbursement+` on the Dashboard screen.
- Use `Demo Wallet` as your wallet and choose a verification method.
- Upload a disbursement file. A sample file is available [sample-disbursement.csv](./sample/sample-disbursement.csv). Make sure to update the invalid phone number before using it.
- Finally, confirm the disbursement.

### Deposit Money

To deposit money into your account:

- Access [demo-wallet](http://localhost:4000) in your browser.
- Click on `Generate Keypair for new account` to generate a new keypair. Make sure to save your public key & secret if you want to use this account later.
- Click `Create account` (in front of public key) to actually create the account on the Stellar testnet.
- Your newly created account will have 10,000 XLM.
- Add your home domain to the account by clicking on `Add Home Domain` and entering `http://bluecorp.stellar.local:8000`.
- In the `Select action` dropdown, select `SEP-24 Deposit`.
- In the new window, enter the phone number from the disbursement CSV.
- Enter the passcode. You can use `000000` passcode or find the actual passcode in the `sdp-api` container logs.
- Enter the birthday that matches the phone number in the CSV.
- Keep an eye on the dashboard until the payment status reaches `Success`. If everything was set up correctly, your money should be disbursed successfully.

## Troubleshooting

#### Sample Tenant Management Postman collection

A sample [Postman collection](./sample/SDP.postman_collection.json) is available in the `sample` directory. It contains endpoints for managing tenants, authentication, and other operations. You can import `SDP.postman_collection.json` into Postman to easily interact with the API.

#### Distribution account out of funds

Making payments requires transaction fees that are paid in XLM from the distribution account.  Payments will start failing if the distribution account does not have enough XLM to pay for these fees. To check this:
- Find the distribution account public key in `dev/docker-compose.yml` under the variable `DISTRIBUTION_PUBLIC_KEY`
- Access [https://horizon-testnet.stellar.org/accounts/:accountId](https://horizon-testnet.stellar.org/accounts/GARGKDIDH7WMKV5WWPK4BH4CKEQIZGWUCA4EUXCY5VICHTHLEBXVNVMW) in your browser and check the balance.  
- You could also check the balance using [demo wallet](https://demo-wallet.stellar.org/account?secretKey=YOUR_SECRET_KEY)
- If the balance is indeed low, here are some of the options to add additional XLM to the distribution account:

-- from the `dev` directory run the [create_and_fund.go](./scripts/create_and_fund.go) script and specify an existing account using the `--secret` option to specify the account secret key and the --fundxlm` option to add additional xlm via friendbot. Note: you will need to install golang.  example:
   ```sh
   ./go run scripts/create_and_fund.go --secret SECRET_KEY --fundxlm
   ```
-- Create a new funded account via Demo Wallet website and send funds to the Distribution account.
  - Access [https://demo-wallet.stellar.org/](https://demo-wallet.stellar.org/) in your browser.
  - Click on `Generate Keypair for new account` to create a new testnet account. Your account comes with 10,000 XLM.
  - Click on `Send` and enter the distribution account public key and the amount you want to send.
  - Using Freighter or Stellar Laboratory, swap the XLM for USDC if you wish to test with USDC.
  - Just use the newly created account (with 10,000 XLM) as the distribution account by updating the `DISTRIBUTION_PUBLIC_KEY` variable in `dev/docker-compose.yml` and restarting the `sdp-api` container.
  
#### Remote Debugging

A sample [launch.json](./sample//launch.json) is provided for remote debugging with vscode. 