package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/network"
	"github.com/stellar/go/txnbuild"
)

const (
	tomlPath = "/.well-known/stellar.toml"
)

type StellarTOML struct {
	WEB_AUTH_ENDPOINT   string `toml:"WEB_AUTH_ENDPOINT"`
	TRANSFER_SERVER_SEP string `toml:"TRANSFER_SERVER_SEP0024"`
}

func main() {
	if len(os.Args) != 3 {
		fmt.Println("Usage: go run script.go [domain] [secretKey]")
		os.Exit(1)
	}

	domain := os.Args[1]
	secretKey := os.Args[2]

	// Fetch and parse the stellar.toml file
	config, err := fetchStellarTOML(domain)

	if err != nil {
		fmt.Println("Failed to fetch or parse stellar.toml:", err)
		return
	}
	fmt.Println("Fetched stellar.toml successfully:", config)
	print(config.WEB_AUTH_ENDPOINT)

	kp, err := keypair.ParseFull(secretKey)
	if err != nil {
		fmt.Println("Failed to parse secret key:", err)
		return
	}
	fmt.Println("Successfully parsed secret key, public key is:", kp.Address())
	fmt.Println("AUTH SERVER:", config.WEB_AUTH_ENDPOINT)

	token, err := performSEP10Auth(config.WEB_AUTH_ENDPOINT, kp)
	if err != nil {
		fmt.Println("SEP-10 Authentication failed:", err)
		return
	}
	fmt.Println("SEP-10 Authentication successful, token obtained:", token)

	err = performSEP24Deposit(config.TRANSFER_SERVER_SEP, token, kp.Address())
	if err != nil {
		fmt.Println("SEP-24 Deposit failed:", err)
		return
	}
	fmt.Println("SEP-24 Deposit initiated successfully")
}

func fetchStellarTOML(domain string) (StellarTOML, error) {
	var config StellarTOML
	tomlURL := "http://" + domain + tomlPath
	print(tomlURL)

	// Create a client to skip certificate verification (for testing)
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	resp, err := client.Get(tomlURL)
	if err != nil {
		return config, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		return config, fmt.Errorf("failed to fetch stellar.toml: %s, response: %s", tomlURL, string(bodyBytes))
	}

	_, err = toml.DecodeReader(resp.Body, &config)
	return config, err
}

func performSEP10Auth(authURL string, kp *keypair.Full) (string, error) {
	fmt.Println("Fetching challenge transaction from:", authURL)

	// request a transaction to sign
	resp, err := http.Get(authURL + "?account=" + kp.Address())
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch challenge: status code %d, response: %s", resp.StatusCode, string(bodyBytes))
	}

	var challenge struct {
		Transaction string `json:"transaction"`
		Network     string `json:"network"`
	}

	if err := json.Unmarshal(bodyBytes, &challenge); err != nil {
		return "", err
	}

	// Parse the transaction from XDR
	genericTx, err := txnbuild.TransactionFromXDR(challenge.Transaction)
	if err != nil {
		return "", fmt.Errorf("unable to parse challenge transaction: %v", err)
	}

	// Check if it's a regular transaction and unwrap it
	tx, ok := genericTx.Transaction()
	if !ok {
		return "", fmt.Errorf("parsed data is not a regular transaction")
	}

	// Sign the transaction
	networkPassphrase := network.TestNetworkPassphrase // or network.PublicNetworkPassphrase for production
	signedTx, err := tx.Sign(networkPassphrase, kp)
	if err != nil {
		return "", fmt.Errorf("unable to sign challenge transaction: %v", err)
	}

	// Convert the signed transaction to base64 XDR format to be ready for submission
	signedTxXDR, err := signedTx.Base64()
	if err != nil {
		return "", fmt.Errorf("unable to convert signed transaction to base64 XDR: %v", err)
	}

	// Submit the signed transaction
	reqBody, err := json.Marshal(map[string]string{"transaction": signedTxXDR})
	if err != nil {
		return "", err
	}

	verifyResp, err := http.Post(authURL, "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return "", err
	}
	defer verifyResp.Body.Close()

	verifyBodyBytes, _ := ioutil.ReadAll(verifyResp.Body)
	if verifyResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("verification failed: status code %d, response: %s", verifyResp.StatusCode, string(verifyBodyBytes))
	}

	var authResponse struct {
		Token string `json:"token"`
	}

	if err := json.Unmarshal(verifyBodyBytes, &authResponse); err != nil {
		return "", fmt.Errorf("error parsing verification response: %v", err)
	}

	return authResponse.Token, nil
}

func performSEP24Deposit(depositURL, token, account string) error {
	params := map[string]string{
		"account":                     account,
		"asset_code":                  "USDC",
		"lang":                        "en",
		"claimable_balance_supported": "false",
	}
	jsonData, err := json.Marshal(params)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", depositURL+"/transactions/deposit/interactive", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed deposit: %s", string(body))
	}

	fmt.Println("Deposit response status:", resp.Status)
	fmt.Println("Deposit response body:", string(body))
	return nil
}
