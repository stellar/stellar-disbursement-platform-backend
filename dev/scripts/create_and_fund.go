package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/keypair"
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
	flag.StringVar(&xlmAmount, "xlm_amount", "10", "The amount of XLM to fund the account with (default is 10).")

	//nolint:errcheck // Not handling error on flag.Usage
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of %s:\n", os.Args[0])
		fmt.Fprintln(flag.CommandLine.Output(), "  This program creates and manages funding for Stellar accounts.")
		fmt.Fprintln(flag.CommandLine.Output(), "  It can generate a new keypair or use an existing secret key to manage account operations.")
		flag.PrintDefaults()
		fmt.Fprintln(flag.CommandLine.Output(), "\nExamples:")
		fmt.Fprintln(flag.CommandLine.Output(), "\nCreate new stellar account with any funding:")
		fmt.Fprintln(flag.CommandLine.Output(), "  go run scripts/create_and_fund.go -secret=SXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX -fundusdc=true")
		fmt.Fprintln(flag.CommandLine.Output(), "\nFund USDC into an existing account:")
		fmt.Fprintln(flag.CommandLine.Output(), "  go run scripts/create_and_fund.go -secret=SXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX -fundxlm=true")
	}

	flag.Parse()

	if secretKey == "" {
		pair, err = keypair.Random()
		if err != nil {
			log.Fatalf("Failed to generate a new keypair: %v", err)
		}
	} else {
		pair, err = keypair.ParseFull(secretKey)
		if err != nil {
			log.Fatalf("Failed to parse secret key: %v", err)
		}
	}

	if pair != nil {
		fmt.Printf("Public Key: %s\n", pair.Address())
		fmt.Printf("Secret Key: %s\n", pair.Seed())
		fmt.Printf("Demo Wallet URL: https://demo-wallet.stellar.org/account?secretKey=%s\n", pair.Seed())
	} else {
		log.Fatal("Key pair was not initialized.")
	}

	client := horizonclient.DefaultTestNetClient

	if fundXLM {
		_, err := client.Fund(pair.Address())
		if err != nil {
			printHorizonError(err)
			log.Fatalf("Failed to fund with Friendbot: %v", err)
		}
		fmt.Println("Funded with XLM using Friendbot.")
	}

	if fundUSDC {
		err := establishTrustlineAndBuyUSDC(client, pair, xlmAmount)
		if err != nil {
			printHorizonError(err)
			log.Fatal(err)
		}
	}

	fmt.Printf("Account URL: https://demo-wallet.stellar.org/account?secretKey=%s\n", pair.Seed())
}

func printHorizonError(err error) {
	var hErr *horizonclient.Error
	if errors.As(err, &hErr) {
		fmt.Printf("Horizon Error: %s\n", hErr.Problem.Title)
		fmt.Printf("Detail: %s\n", hErr.Problem.Detail)
		fmt.Printf("Status: %d\n", hErr.Problem.Status)
		fmt.Printf("Type: %s\n", hErr.Problem.Type)
	} else {
		fmt.Printf("Error: %v\n", err)
	}
}

func establishTrustlineAndBuyUSDC(client *horizonclient.Client, pair *keypair.Full, xlmAmount string) error {
	sourceAccount, err := client.AccountDetail(horizonclient.AccountRequest{AccountID: pair.Address()})
	if err != nil {
		return fmt.Errorf("failed to get account details: %w", err)
	}

	usdcAsset := txnbuild.CreditAsset{Code: "USDC", Issuer: "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5"}
	changeTrustAsset, err := usdcAsset.ToChangeTrustAsset()
	if err != nil {
		log.Fatalf("Failed to convert to ChangeTrustAsset: %v", err)
	}

	trustLine := txnbuild.ChangeTrust{
		Line: changeTrustAsset,
	}

	pathPaymentOp := txnbuild.PathPaymentStrictSend{
		SendAsset:   txnbuild.NativeAsset{},
		SendAmount:  xlmAmount,
		Destination: pair.Address(),
		DestAsset:   usdcAsset,
		DestMin:     "0.1",
	}

	tx, err := txnbuild.NewTransaction(
		txnbuild.TransactionParams{
			SourceAccount:        &sourceAccount,
			IncrementSequenceNum: true,
			Operations:           []txnbuild.Operation{&trustLine, &pathPaymentOp},
			BaseFee:              txnbuild.MinBaseFee,
			Preconditions:        txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(300)},
		},
	)
	if err != nil {
		return fmt.Errorf("failed to build the transaction: %w", err)
	}

	tx, err = tx.Sign(network.TestNetworkPassphrase, pair)
	if err != nil {
		return fmt.Errorf("failed to sign the transaction: %w", err)
	}

	resp, err := client.SubmitTransaction(tx)
	if err != nil {
		return fmt.Errorf("failed to submit the transaction: %w", err)
	}

	fmt.Printf("Transaction successful: %s\n", resp.Hash)
	return nil
}
