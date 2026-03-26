# Transaction Submission Service (TSS) Load Testing Script 

Supports both payment and wallet creation transaction load testing.

Load Test Flow:
1) Create N number of Transactions in the database for TSS to process
2) Poll database until all Transactions have completed
3) Query Horizon for each transaction and use those details to calculate and print metrics for the load test

### CLI Flags: 
```sh
    --type                   Type of load test: 'payment' or 'wallet' (required)
    --count                  Number of transactions to create (required)  
    --databaseUrl            Postgres DB URL (required)
    --tenantId               Tenant ID for multi-tenant testing (required)
    --horizonUrl             Horizon URL (default "https://horizon-testnet.stellar.org")
    
    # Payment-specific flags
    --assetCode              Asset code (default "USDC")
    --assetIssuer            Asset issuer (default "GDQOE23CFSUMSVQK4Y5JHPPYK73VYCNHZHA7ENKCV37P6SUEO6XQBKPP")
    --paymentDestination     Destination address to send the payments to (required for payment tests)
    
    # Wallet creation-specific flags
    --wasmHash               WASM hash for contract deployment (default "a5016f845e76fe452de6d3638ac47523b845a813db56de3d713eb7a49276e254")
```

### CLI Usage Examples: 

#### Payment Load Test:
```sh
go run internal/transactionsubmission/scripts/tss_loadtest.go \
  --type payment \
  --count 3 \
  --databaseUrl "postgres://postgres@localhost:5432/sdp_mtn?sslmode=disable&search_path=tss" \
  --tenantId "4c8be26b-bdaa-42cb-bdd1-6988ea74810c" \
  --paymentDestination "GAIEWCHNAB2VTVQZEQ7STONZVTLR5BCDZQTUPFWGTNHLLKR66IB77FXR"
```

#### Wallet Creation Load Test:
```sh
go run internal/transactionsubmission/scripts/tss_loadtest.go \
  --type wallet \
  --count 5 \
  --databaseUrl "postgres://postgres@localhost:5432/sdp_mtn?sslmode=disable&search_path=tss" \
  --tenantId "4c8be26b-bdaa-42cb-bdd1-6988ea74810c"
```

### Example Output:
```
All 3 payment transactions have completed!
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
