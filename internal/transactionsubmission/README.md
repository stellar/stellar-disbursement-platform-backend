# Transaction Submission Service

The Transaction Submission Service (TSS) is a component that is responsible for submitting payment transactions to the Stellar Network.

The SDP will directly 'queue' transactions (create transactions in the database) and the Transaction Submission Service will read these transactions and submit them to the Stellar Network.

The Transaction Submission Service requires channel accounts to be seeded in storage in advanced. To learn how to fulfill this prerequisite, please refer to the [Channel Accounts Management](#channel-accounts-management) section below.

## Transaction Submitter
### CLI Usage: `tss`
```sh
$ stellar-disbursement-platform tss --help
Run the Transaction Submission Service

Usage:
  stellar-disbursement-platform tss [flags]

Flags:
      --crash-tracker-type string    Crash tracker type. Options: "SENTRY", "DRY_RUN" (CRASH_TRACKER_TYPE) (default "DRY_RUN")
      --distribution-seed string     The private key of the Stellar account used to disburse funds (DISTRIBUTION_SEED)
  -h, --help                         help for tss
      --horizon-url string           Horizon URL (HORIZON_URL) (default "https://horizon-testnet.stellar.org/")
      --max-base-fee int             The max base fee for submitting a Stellar transaction (MAX_BASE_FEE) (default 100)
      --num-channel-accounts int     Number of channel accounts to utilize for transaction submission (NUM_CHANNEL_ACCOUNTS) (default 2)
      --queue-polling-interval int   Polling interval (seconds) to query the database for pending transactions to process (QUEUE_POLLING_INTERVAL) (default 6)
      --tss-metrics-port int         Port where the metrics server will be listening on. Default: 9002" (TSS_METRICS_PORT) (default 9002)
      --tss-metrics-type string      Metric monitor type. Options: "TSS_PROMETHEUS" (TSS_METRICS_TYPE) (default "TSS_PROMETHEUS")

Global Flags:
      --base-url string             The SDP UI base URL. (BASE_URL) (default "http://localhost:8000")
      --database-url string         Postgres DB URL (DATABASE_URL) (default "postgres://localhost:5432/sdp?sslmode=disable")
      --environment string          The environment where the application is running. Example: "development", "staging", "production". (ENVIRONMENT) (default "development")
      --log-level string            The log level used in this project. Options: "TRACE", "DEBUG", "INFO", "WARN", "ERROR", "FATAL", or "PANIC". (LOG_LEVEL) (default "TRACE")
      --network-passphrase string   The Stellar network passphrase (NETWORK_PASSPHRASE) (default "Test SDF Network ; September 2015")
      --sentry-dsn string           The DSN (client key) of the Sentry project. If not provided, Sentry will not be used. (SENTRY_DSN)
```

## Channel Accounts Management

Channel Accounts are used to increase throughput when submitting transaction to the Stellar Network, and are a prerequisite for using TSS. This CLI tools should enable all use cases for management of Channel Accounts (both onchain and in the database).

### CLI Usage: `channel-accounts`
```sh
$ stellar-disbursement-platform channel-accounts --help
Channel accounts related commands

Usage:
  stellar-disbursement-platform channel-accounts [command]

Available Commands:
  create      Create channel accounts
  delete      Delete a specified channel account from storage and on the network
  ensure      Ensure we are managing exactly the number of channel accounts equal to some specified count by dynamically increasing or decreasing the number of managed channel accounts in storage and onchain
  verify      Verify the existence of all channel accounts in the database on the Stellar newtwork
  view        View all channel accounts currently managed in the database
  
Flags:
  -h, --help                 help for channel-accounts
      --horizon-url string   Horizon URL" (HORIZON_URL) (default "https://horizon-testnet.stellar.org/")

Global Flags:
      --base-url string             The SDP UI base URL. (BASE_URL) (default "http://localhost:8000")
      --database-url string         Postgres DB URL (DATABASE_URL) (default "postgres://localhost:5432/sdp?sslmode=disable")
      --environment string          The environment where the application is running. Example: "development", "staging", "production". (ENVIRONMENT) (default "development")
      --log-level string            The log level used in this project. Options: "TRACE", "DEBUG", "INFO", "WARN", "ERROR", "FATAL", or "PANIC". (LOG_LEVEL) (default "TRACE")
      --network-passphrase string   The Stellar network passphrase (NETWORK_PASSPHRASE) (default "Test SDF Network ; September 2015")
      --sentry-dsn string           The DSN (client key) of the Sentry project. If not provided, Sentry will not be used. (SENTRY_DSN)

Use "stellar-disbursement-platform channel-accounts [command] --help" for more information about a command.
```

### CLI Usage: `channel-accounts create`
```sh
channel-accounts create --help
Usage:
  stellar-disbursement-platform channel-accounts create [flags]

Flags:
      --distribution-seed string          The private key of the Stellar account that will be used to sponsor the channel accounts (DISTRIBUTION_SEED)
  -h, --help                              help for create
      --max-base-fee int                  The max base fee for submitting a stellar transaction (MAX_BASE_FEE) (default 100)
      --num-channel-accounts-create int   The desired number of channel accounts to be created (NUM_CHANNEL_ACCOUNTS_CREATE) (default 1)
```

### CLI Usage: `channel-accounts ensure`
```sh
channel-accounts ensure --help
Usage:
  stellar-disbursement-platform channel-accounts ensure [flags]

Flags:
      --distribution-seed string          The private key of the Stellar account used to sponsor existing channel accounts (DISTRIBUTION_SEED)
  -h, --help                              help for ensure
      --max-base-fee int                  The max base fee for submitting a stellar transaction (MAX_BASE_FEE) (default 100)
      --num-channel-accounts-ensure int   The desired number of channel accounts to manage (NUM_CHANNEL_ACCOUNTS_ENSURE) (default 1)
```

### CLI Usage: `channel-accounts delete`
```sh
channel-accounts delete --help
Usage:
  stellar-disbursement-platform channel-accounts delete [flags]

Flags:
      --channel-account-id string   The ID of the channel account to delete (CHANNEL_ACCOUNT_ID)
      --delete-all-accounts         Delete all managed channel accoounts in the database and on the network (DELETE_ALL_ACCOUNTS)
      --distribution-seed string    The private key of the Stellar account used to sponsor the channel account specified (DISTRIBUTION_SEED)
  -h, --help                        help for delete
      --max-base-fee int            The max base fee for submitting a stellar transaction (MAX_BASE_FEE) (default 100)
```

### CLI Usage: `channel-accounts verify`
```sh
channel-accounts verify --help
Usage:
  stellar-disbursement-platform channel-accounts verify [flags]

Flags:
      --delete-invalid-accounts   Delete channel accounts from storage that are verified to be invalid on the network (DELETE_INVALID_ACCOUNTS)
  -h, --help                      help for verify
```

### CLI Usage: `channel-accounts verify`
```sh
channel-accounts verify --help
Usage:
  stellar-disbursement-platform channel-accounts verify [flags]

Flags:
      --delete-invalid-accounts   Delete channel accounts from storage that are verified to be invalid on the network (DELETE_INVALID_ACCOUNTS)
  -h, --help                      help for verify
```

### CLI Usage: `channel-accounts view`
```sh
channel-accounts view --help
Usage:
  stellar-disbursement-platform channel-accounts view [flags]

Flags:
  -h, --help   help for view
```

## Testing
### Mocks
TSS unit tests rely on mocks of its interfaces auto-generated by mockery. For installation instructions, see [here](https://vektra.github.io/mockery/installation/). 

Refer to the output to learn how to annotate interfaces and about the different flags that you can leverage to manipulate the output.
```
mockery --help
```

To generate the mocks
```
go generate ./...
```
