# Transaction Submission Payments Load Testing Script 

Load Test Flow:
1) Create N number of Transactions in the database for TSS to process
2) Poll database until all Transactions have completed
3) Query Horizon for each transaction and use those details to calculate and print metrics for the load test

### CLI Flags: 
```sh
    --databaseUrl            Postgres DB URL
    --horizonUrl             Horizon URL (default "https://horizon-testnet.stellar.org")
    --assetCode              Asset code (default "USDC")
    --assetIssuer            Asset issuer (default "GDQOE23CFSUMSVQK4Y5JHPPYK73VYCNHZHA7ENKCV37P6SUEO6XQBKPP")
    --paymentDestination     Destination address to send the payments to
    --paymentCount           Number of payment Transactions to create
```

### CLI Usage Example: 
```sh
% go run internal/transactionsubmission/scripts/tss_payments_loadtest.go \
--databaseUrl "postgres://postgres:password@localhost:5432/tss-testing?sslmode=disable" \
--horizonUrl "https://horizon.stellar.org" \
--assetCode "USDS" -assetIssuer "GCDUFCM7HA2AXFPWCXI55MXMCPORHOE42YIIBKN72SAMZ6WBO3G2E5TF" \
--paymentDestination "GAR5YLLLSTPOJGK2T5P5WMSVGEFWQLDMPMZXICURGBUYJOVXARI2ZTXI" \
--paymentCount 3


All 3 transactions have completed!
Test size: 3 payment(s)
==========================================================
TSS first created payment time:      2023-06-21 11:06:55
Stellar first observed payment time: 2023-06-21 11:07:02
TSS last created payment time:       2023-06-21 11:06:55
Stellar final payment observed time: 2023-06-21 11:07:26
=========================================================
Total test latency (first created, last observed): 30.29
==========================================================
min e2e payment latency:      6.29
average e2e payment latency:  16.28
max e2e payment latency:      30.28
==========================================================
calculated average TPS:       0.10
unique ledgers:               3
==========================================================
```
