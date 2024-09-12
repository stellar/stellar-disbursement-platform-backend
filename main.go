package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"

	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"
	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/network"
	"github.com/stellar/go/strkey"
	"github.com/stellar/go/support/log"
	"github.com/stellar/go/txnbuild"
	"github.com/stellar/go/xdr"

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
	AccountSecret        = "SAARF2ZWAHZJMKA6LXIFVNIHUBEUTMKV5NWCCUZV6ORPKLUK6RSOYZ4D"
	TestTxHash           = "0d692b8dbb41bc9f2efce7a9be3d0502fb2348ab28fd4b2574bd7fe1a488cb36"
	NetworkPassphrase    = network.TestNetworkPassphrase
	HorizonURL           = "https://horizon-testnet.stellar.org"
	SorobanURL           = "https://soroban-testnet.stellar.org"
	FactoryContractID    = "CCXAAMITX4NT5MU7NVCCCY2SFK7VY5EPAUXCOPNHZQTDFHS3HJ7RU57G"
	ContractIDHelloWorld = "CALNHFGOO2JVHGKJEDES5NAKVAMEBZIGVUQHLP4DN7O52OOYP4DCKSHT"
)

func main2() {
	// The base64-encoded XDR string
	xdrBase64 := "AAAAAAAAAAIAAAAGAAAAARbTlM52k1OZSSDJLrQKqBhA5QatIHW/g2/d3TnYfwYlAAAAFAAAAAEAAAAHtgisgPJo0fHEan/FTBa3UZCpuh492P1v12bOJd8t3mYAAAAAAAeLBgAAAugAAAAAAAAAAAAA/aQ="

	sorobanTxData, err := DecodeXDR[xdr.SorobanTransactionData](xdrBase64)
	if err != nil {
		log.Panicf("Failed to decode XDR: %v", err)
	}
	fmt.Printf("Deserialized SorobanTransactionData: %+v\n", sorobanTxData)

	ext, err := xdr.NewTransactionExt(1, sorobanTxData)
	if err != nil {
		log.Panicf("Failed to create TransactionExt: %v", err)
	}
	fmt.Printf("Created TransactionExt: %+v\n", ext)
}

func DecodeXDR[T any](xdrBase64 string) (T, error) {
	xdrBytes, err := base64.StdEncoding.DecodeString(xdrBase64)
	if err != nil {
		return *new(T), fmt.Errorf("decoding base64: %w", err)
	}

	var xdrObj T
	err = xdr.SafeUnmarshal(xdrBytes, &xdrObj)
	if err != nil {
		return *new(T), fmt.Errorf("unmarshalling XDR: %w", err)
	}

	return xdrObj, nil
}

func main() {
	if err := godotenv.Load(); err != nil {
		log.Debug("No .env file found")
	}

	preConfigureLogger()
	ctx := context.Background()

	s := NewSorobanService()

	fmt.Println("\n==================================== printHealth")
	s.printHealth(ctx)
	fmt.Println("\n==================================== printGetTransaction")
	s.printGetTransaction(ctx, TestTxHash)
	// fmt.Println("\n==================================== printSendNativePaymentTransaction")
	// s.printSendNativePaymentTransaction(ctx)
	fmt.Println("\n==================================== printSendInvokeContractHelloWorldTransaction")
	s.printSendInvokeContractTxHelloWorld(ctx)
}

// preConfigureLogger will set the log level to Trace, so logs works from the
// start. This will eventually be overwritten in cmd/root.go
func preConfigureLogger() {
	log.DefaultLogger = log.New()
	log.DefaultLogger.SetLevel(logrus.TraceLevel)
}

//////// SorobanService

type SorobanService struct {
	rpcClient         *sorobanrpc.Client
	sourceAccKP       *keypair.Full
	networkPassphrase string
	horizonClient     horizonclient.ClientInterface
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
	resp, err := s.rpcClient.GetHealth(ctx, 1)
	if err != nil {
		log.Ctx(ctx).Panicf("Error calling RPC.GetHealth: %v", err)
	}
	log.Ctx(ctx).Infof("RPC GetHealth: %+v", resp.Result)
}

func (s *SorobanService) printGetTransaction(ctx context.Context, hash string) {
	resp, err := s.rpcClient.GetTransaction(ctx, 1, hash)
	if err != nil {
		log.Ctx(ctx).Panicf("Error calling RPC.GetTransaction: %v", err)
	}
	log.Ctx(ctx).Infof("RPC GetTransaction: %+v", resp.Result)
}

func (s *SorobanService) printSendNativePaymentClassicTx(ctx context.Context) {
	tx, err := s.buildAndSignNativePaymentTx(ctx)
	if err != nil {
		log.Ctx(ctx).Panicf("Error building&signing transaction: %v", err)
	}
	resp, err := s.rpcClient.SendTransaction(ctx, 1, tx.XDR)
	if err != nil {
		log.Ctx(ctx).Panicf("Error calling RPC.SendTransaction: %v", err)
	}
	log.Ctx(ctx).Infof("RPC SendTransaction (hash=%s): %+v", tx.Hash, resp.Result)
}

func (s *SorobanService) prepareSorobanTx(ctx context.Context, txXDR string) (*TxWithMetadata, error) {
	simulatedTxResp, err := s.rpcClient.SimulateTransaction(ctx, 1, txXDR)
	if err != nil {
		return nil, fmt.Errorf("calling RPC with getTransaction: %w", err)
	}
	if simulatedTxResp.Error != nil {
		return nil, fmt.Errorf("simulated transaction failed: %+v", simulatedTxResp.Error)
	}

	genericTx, err := txnbuild.TransactionFromXDR(txXDR)
	if err != nil {
		return nil, fmt.Errorf("creating generic transaction from XDR: %w", err)
	}
	originalTx, ok := genericTx.Transaction()
	if !ok {
		return nil, fmt.Errorf("casting generic transaction to xdr.Transaction")
	}

	minResourceFee, err := strconv.ParseInt(simulatedTxResp.Result.MinResourceFee, 10, 64) // Base 10, bit size 64
	if err != nil {
		return nil, fmt.Errorf("parsing minResourceFee: %w", err)
	}

	hAcc, err := s.horizonClient.AccountDetail(horizonclient.AccountRequest{AccountID: s.sourceAccKP.Address()})
	if err != nil {
		return nil, tssUtils.NewHorizonErrorWrapper(err)
	}

	txParams := txnbuild.TransactionParams{
		SourceAccount: &hAcc,
		BaseFee:       originalTx.BaseFee() + minResourceFee,
		Preconditions: txnbuild.Preconditions{
			TimeBounds: originalTx.Timebounds(),
		},
		IncrementSequenceNum: true,
		Memo:                 originalTx.Memo(),
	}

	ops := originalTx.Operations()
	invokeContractOp, ok := ops[0].(*txnbuild.InvokeHostFunction)
	if !ok {
		return nil, fmt.Errorf("casting operation to InvokeHostFunction")
	}

	sorobanTxData, err := DecodeXDR[xdr.SorobanTransactionData](simulatedTxResp.Result.TransactionData)
	if err != nil {
		log.Ctx(ctx).Panicf("Failed to decode XDR: %v", err)
	}
	ext, err := xdr.NewTransactionExt(1, sorobanTxData)
	if err != nil {
		log.Ctx(ctx).Panicf("Failed to create TransactionExt: %v", err)
	}
	invokeContractOp.Ext = ext

	txParams.Operations = []txnbuild.Operation{invokeContractOp}
	newTx, err := txnbuild.NewTransaction(txParams)
	if err != nil {
		return nil, fmt.Errorf("creating transaction: %w", err)
	}

	newTx, err = newTx.Sign(s.networkPassphrase, s.sourceAccKP)
	if err != nil {
		return nil, fmt.Errorf("signing transaction: %w", err)
	}

	return txWithMetadata(newTx, s)
}

func (s *SorobanService) printSendInvokeContractTxHelloWorld(ctx context.Context) {
	tx, err := s.buildAndSignContractHelloWorldTx(ctx, nil)
	if err != nil {
		log.Ctx(ctx).Panicf("Error calling buildAndSignContractHelloWorldTx: %v", err)
	}
	fmt.Println("1. original XDR:", tx.XDR)

	preparedTx, err := s.prepareSorobanTx(ctx, tx.XDR)
	if err != nil {
		log.Ctx(ctx).Panicf("Error calling prepareSorobanTx: %v", err)
	}
	fmt.Println("2. prepared XDR:", preparedTx.XDR)

	resp, err := s.rpcClient.SendTransaction(ctx, 1, preparedTx.XDR)
	if err != nil {
		log.Ctx(ctx).Panicf("Error calling RPC.SendTransaction: %v", err)
	}
	log.Ctx(ctx).Infof("RPC SendTransaction (hash=%s): %+v", preparedTx.Hash, resp.Result)
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

	return txWithMetadata(tx, s)
}

func txWithMetadata(tx *txnbuild.Transaction, s *SorobanService) (*TxWithMetadata, error) {
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

func (s *SorobanService) buildAndSignContractHelloWorldTx(_ context.Context, ext *xdr.TransactionExt) (*TxWithMetadata, error) {
	hAcc, err := s.horizonClient.AccountDetail(horizonclient.AccountRequest{AccountID: s.sourceAccKP.Address()})
	if err != nil {
		return nil, tssUtils.NewHorizonErrorWrapper(err)
	}

	paramWorld, err := xdr.NewScVal(xdr.ScValTypeScvSymbol, xdr.ScSymbol("world"))
	if err != nil {
		return nil, fmt.Errorf("creating contract invoke operation param: %w", err)
	}
	contractInvokeOp, err := NewInvokeContractOp(s.sourceAccKP.Address(), ContractIDHelloWorld, ext, "hello", paramWorld)
	if err != nil {
		return nil, fmt.Errorf("creating contract invoke operation: %w", err)
	}

	tx, err := txnbuild.NewTransaction(
		txnbuild.TransactionParams{
			SourceAccount:        &hAcc,
			Operations:           []txnbuild.Operation{contractInvokeOp},
			BaseFee:              txnbuild.MinBaseFee * 100,
			Preconditions:        txnbuild.Preconditions{TimeBounds: txnbuild.NewInfiniteTimeout()},
			IncrementSequenceNum: true,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("creating transaction: %w", err)
	}

	tx, err = tx.Sign(s.networkPassphrase, s.sourceAccKP)
	if err != nil {
		return nil, fmt.Errorf("signing transaction: %w", err)
	}

	return txWithMetadata(tx, s)
}

// type InvokeContractOpOptions struct {}
func NewInvokeContractOp(sourceAccount, contractID string, ext *xdr.TransactionExt, functionName xdr.ScSymbol, params ...xdr.ScVal) (*txnbuild.InvokeHostFunction, error) {
	contractStrKey, err := strkey.Decode(strkey.VersionByteContract, contractID)
	if err != nil {
		return nil, fmt.Errorf("decoding contract ID: %w", err)
	}
	fmt.Println("contractStrkey len:", len(contractStrKey))
	if len(contractStrKey) != 32 {
		return nil, fmt.Errorf("contract ID is not 32 bytes long")
	}

	contractIDHash := xdr.Hash(contractStrKey)

	contractInvokeOp := &txnbuild.InvokeHostFunction{
		HostFunction: xdr.HostFunction{
			Type: xdr.HostFunctionTypeHostFunctionTypeInvokeContract,
			InvokeContract: &xdr.InvokeContractArgs{
				ContractAddress: xdr.ScAddress{
					Type:       xdr.ScAddressTypeScAddressTypeContract,
					ContractId: &contractIDHash,
				},
				FunctionName: functionName,
				Args:         xdr.ScVec(params),
			},
		},
	}
	if ext != nil {
		contractInvokeOp.Ext = *ext
	}
	return contractInvokeOp, nil
}
