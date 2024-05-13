package main

import (
    "flag"
    "fmt"
    "log"
    "os"

    "github.com/stellar/go/keypair"
    "github.com/stellar/go/clients/horizonclient"
    "github.com/stellar/go/network"
    "github.com/stellar/go/txnbuild"  
)

func main() {
    var secretKey string
    var fundXLM bool
    var fundUSDC bool
    var xlmAmount string
    var pair *keypair.Full
    var err error

    flag.StringVar(&secretKey, "secret", "", "The secret key of an existing Stellar account")
    flag.BoolVar(&fundXLM, "fundxlm", false, "Set to true to fund the account with XLM using Friendbot")
    flag.BoolVar(&fundUSDC, "fundusdc", false, "Set to true to fund the account with USDC and establish a trustline")
    flag.StringVar(&xlmAmount, "xlm_amount", "10", "The amount of USDC to fund the account with (default is 10).")

    
    flag.Usage = func() {
        fmt.Fprintf(flag.CommandLine.Output(), "Usage of %s:\n", os.Args[0])
        fmt.Fprintf(flag.CommandLine.Output(), "  This program creates and manages funding for Stellar accounts.\n")
        fmt.Fprintf(flag.CommandLine.Output(), "  It can generate a new keypair or use an existing secret key to manage account operations.\n\n")
        flag.PrintDefaults()
        fmt.Fprintf(flag.CommandLine.Output(), "\nExamples:\n")
        fmt.Fprintf(flag.CommandLine.Output(), "  %s -secret=SXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX -fundxlm=true\n", os.Args[0])
        fmt.Fprintf(flag.CommandLine.Output(), "  %s -secret=SXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX -fundusdc=true\n", os.Args[0])
    }

    flag.Parse()
    
    if secretKey == "" {
            // Do not redeclare pair, just assign
            pair, err = keypair.Random()
            if err != nil {
                log.Fatalf("Failed to generate a new keypair: %v", err)
            }
        } else {
            // Do not redeclare pair, just assign
            pair, err = keypair.ParseFull(secretKey)
            if err != nil {
                log.Fatalf("Failed to parse secret key: %v", err)
            }
        }
    
        // Ensure that pair is not nil before using it
        if pair != nil {
            fmt.Printf("Public Key: %s\n", pair.Address())
            fmt.Printf("Secret Key: %s\n", pair.Seed())
        } else {
            log.Fatal("Key pair was not initialized.")
        }

    client := horizonclient.DefaultTestNetClient

    // Fund with XLM using Friendbot if --fundxlm is specified
    if fundXLM {
        _, err := client.Fund(pair.Address())
        if err != nil {
            log.Fatalf("Failed to fund with Friendbot: %v", err)
        }
        fmt.Println("Funded with XLM using Friendbot.")
    }

    if fundUSDC {
        err := establishTrustlineAndBuyUSDC(client, pair, xlmAmount)
        if err != nil {
            log.Fatal(err)
        }
    }
}

func establishTrustlineAndBuyUSDC(client *horizonclient.Client, pair *keypair.Full, xlmAmount string) error {
    sourceAccount, err := client.AccountDetail(horizonclient.AccountRequest{AccountID: pair.Address()})
    if err != nil {
        return fmt.Errorf("failed to get account details: %v", err)
    }

    usdcAsset := txnbuild.CreditAsset{Code: "USDC", Issuer: "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5"}
    changeTrustAsset, err := usdcAsset.ToChangeTrustAsset()
        if err != nil {
            log.Fatalf("Failed to convert to ChangeTrustAsset: %v", err)
        }
    
    trustLine := txnbuild.ChangeTrust{
        Line: changeTrustAsset,
    }

      // Create a path payment strict send operation
    pathPaymentOp := txnbuild.PathPaymentStrictSend{
        SendAsset:   txnbuild.NativeAsset{},
        SendAmount:  xlmAmount,
        Destination: pair.Address(),
        DestAsset:   usdcAsset,
        DestMin:     "0.1", // Very low minimum to ensure the trade goes through
    }

    // Build the transaction with both operations
    tx, err := txnbuild.NewTransaction(
        txnbuild.TransactionParams{
            SourceAccount:        &sourceAccount,
            IncrementSequenceNum: true,
            Operations:           []txnbuild.Operation{&trustLine, &pathPaymentOp},
            BaseFee:              txnbuild.MinBaseFee,
            Preconditions:           txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(300)},
        },
    )
    if err != nil {
        return fmt.Errorf("failed to build the transaction: %v", err)
    }

    // Sign the transaction with the network passphrase and the secret key
    tx, err = tx.Sign(network.TestNetworkPassphrase, pair)
    if err != nil {
        return fmt.Errorf("failed to sign the transaction: %v", err)
    }

    // Submit the transaction to the Stellar network
    resp, err := client.SubmitTransaction(tx)
    if err != nil {
        return fmt.Errorf("failed to submit the transaction: %v", err)
    }

    fmt.Printf("Transaction successful: %s\n", resp.Hash)
    return nil
}
