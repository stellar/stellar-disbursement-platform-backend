package main

import (
	"context"
	"fmt"

	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"
	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/network"
	"github.com/stellar/go/support/log"
	"github.com/stellar/go/txnbuild"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpclient"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/services/sorobanrpc"
	tssUtils "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/utils"
)

// Version is the official version of this application. Whenever it's changed
// here, it also needs to be updated at the `helmchart/Chart.yaml#appVersion“.
const Version = "2.1.0"

// GitCommit is populated at build time by
// go build -ldflags "-X main.GitCommit=$GIT_COMMIT"
var GitCommit string

const (
	AccountSecret     = "SAARF2ZWAHZJMKA6LXIFVNIHUBEUTMKV5NWCCUZV6ORPKLUK6RSOYZ4D"
	TestTxHash        = "0d692b8dbb41bc9f2efce7a9be3d0502fb2348ab28fd4b2574bd7fe1a488cb36"
	NetworkPassphrase = network.TestNetworkPassphrase
	HorizonURL        = "https://horizon-testnet.stellar.org"
	SorobanURL        = "https://soroban-testnet.stellar.org"
)

type SorobanService struct {
	rpcClient         *sorobanrpc.Client
	sourceAccKP       *keypair.Full
	networkPassphrase string
	horizonClient     horizonclient.ClientInterface
}

func main() {
	if err := godotenv.Load(); err != nil {
		log.Debug("No .env file found")
	}

	preConfigureLogger()
	ctx := context.Background()

	s := NewSorobanService()

	fmt.Println("\n====================================")
	s.printHealth(ctx)
	fmt.Println("\n====================================")
	s.printGetTransaction(ctx, TestTxHash)
	fmt.Println("\n====================================")
	s.printSendTransaction(ctx)
	fmt.Println("\n====================================")
}

func NewSorobanService() SorobanService {
	sourceAccKP := keypair.MustParseFull(AccountSecret)
	rpcClient := sorobanrpc.NewClient(SorobanURL, nil)
	horizonClient := &horizonclient.Client{
		HorizonURL: HorizonURL,
		HTTP:       httpclient.DefaultClient(),
	}
	s := SorobanService{
		rpcClient:         rpcClient,
		sourceAccKP:       sourceAccKP,
		networkPassphrase: NetworkPassphrase,
		horizonClient:     horizonClient,
	}
	return s
}

func (s *SorobanService) printHealth(ctx context.Context) {
	resp, err := s.rpcClient.Call(ctx, 1, "getHealth", nil)
	if err != nil {
		log.Ctx(ctx).Panicf("Error calling RPC with getHealth: %v", err)
	}
	log.Ctx(ctx).Infof("RPC Call(getHealth): %s", resp.Result)

	resp2, err := s.rpcClient.GetHealth(ctx, 1)
	if err != nil {
		log.Ctx(ctx).Panicf("Error calling RPC.GetHealth: %v", err)
	}
	log.Ctx(ctx).Infof("RPC GetHealth: %+v", resp2.Result)
}

func (s *SorobanService) printGetTransaction(ctx context.Context, hash string) {
	resp1, err := s.rpcClient.Call(ctx, 1, "getTransaction", map[string]string{"hash": hash})
	if err != nil {
		log.Ctx(ctx).Panicf("Error calling RPC with getTransaction: %v", err)
	}
	log.Ctx(ctx).Infof("RPC Call(getTransaction): %s", resp1.Result)

	resp2, err := s.rpcClient.GetTransaction(ctx, 1, hash)
	if err != nil {
		log.Ctx(ctx).Panicf("Error calling RPC.GetTransaction: %v", err)
	}
	log.Ctx(ctx).Infof("RPC GetTransaction: %+v", resp2.Result)
}

func (s *SorobanService) printSendTransaction(ctx context.Context) {
	tx1, err := s.buildAndSignNativePaymentTx(ctx)
	if err != nil {
		log.Ctx(ctx).Panicf("Error building&signing transaction: %v", err)
	}
	resp1, err := s.rpcClient.Call(ctx, 1, "sendTransaction", map[string]string{"transaction": tx1.XDR})
	if err != nil {
		log.Ctx(ctx).Panicf("Error calling RPC with getTransaction: %v", err)
	}
	log.Ctx(ctx).Infof("RPC Call(sendTransaction, hash=%s): %s", tx1.Hash, resp1.Result)

	tx2, err := s.buildAndSignNativePaymentTx(ctx)
	if err != nil {
		log.Ctx(ctx).Panicf("Error building&signing transaction: %v", err)
	}
	resp2, err := s.rpcClient.SendTransaction(ctx, 1, tx2.XDR)
	if err != nil {
		log.Ctx(ctx).Panicf("Error calling RPC.SendTransaction: %v", err)
	}
	log.Ctx(ctx).Infof("RPC SendTransaction (hash=%s): %+v", tx2.Hash, resp2.Result)
}

type TxWithMetadata struct {
	XDR  string
	Hash string
	tx   txnbuild.Transaction
}

func (s *SorobanService) buildAndSignNativePaymentTx(_ context.Context) (*TxWithMetadata, error) {
	hAcc, err := s.horizonClient.AccountDetail(horizonclient.AccountRequest{AccountID: s.sourceAccKP.Address()})
	if err != nil {
		return nil, tssUtils.NewHorizonErrorWrapper(err)
	}

	tx, err := txnbuild.NewTransaction(
		txnbuild.TransactionParams{
			SourceAccount:        &hAcc,
			IncrementSequenceNum: true,
			BaseFee:              txnbuild.MinBaseFee * 100,
			Preconditions: txnbuild.Preconditions{
				TimeBounds: txnbuild.NewTimeout(60),
			},
			Operations: []txnbuild.Operation{
				&txnbuild.Payment{
					Destination: s.sourceAccKP.Address(),
					Amount:      "1",
					Asset:       txnbuild.NativeAsset{},
				},
			},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("creating transaction: %w", err)
	}

	tx, err = tx.Sign(s.networkPassphrase, s.sourceAccKP)
	if err != nil {
		return nil, fmt.Errorf("signing transaction: %w", err)
	}

	txHash, err := tx.HashHex(s.networkPassphrase)
	if err != nil {
		return nil, fmt.Errorf("hashing transaction: %w", err)
	}

	txXDR, err := tx.Base64()
	if err != nil {
		return nil, fmt.Errorf("converting transaction to base64: %w", err)
	}

	return &TxWithMetadata{
		XDR:  txXDR,
		Hash: txHash,
		tx:   *tx,
	}, nil
}

// preConfigureLogger will set the log level to Trace, so logs works from the
// start. This will eventually be overwritten in cmd/root.go
func preConfigureLogger() {
	log.DefaultLogger = log.New()
	log.DefaultLogger.SetLevel(logrus.TraceLevel)
}
