package sorobanutils

import (
	"fmt"

	"github.com/stellar/go/strkey"
	"github.com/stellar/go/txnbuild"
	"github.com/stellar/go/xdr"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

// makeScContract creates an xdr.ScAddress from a contract ID string.
func MakeScContract(contractID string) (xdr.ScAddress, error) {
	decoded, err := strkey.Decode(strkey.VersionByteContract, contractID)
	if err != nil {
		return xdr.ScAddress{}, fmt.Errorf("decoding contract ID: %w", err)
	}

	return xdr.ScAddress{
		Type:       xdr.ScAddressTypeScAddressTypeContract,
		ContractId: utils.Ptr(xdr.Hash(decoded)),
	}, nil
}

type InvokeContractOptions struct {
	OpSourceAccount string
	ContractID      string
	FunctionName    string
	Args            []xdr.ScVal
}

func CreateContractInvocationOp(opts InvokeContractOptions) (txnbuild.InvokeHostFunction, error) {
	contractScAddress, err := MakeScContract(opts.ContractID)
	if err != nil {
		return txnbuild.InvokeHostFunction{}, fmt.Errorf("making contract address: %w", err)
	}

	return txnbuild.InvokeHostFunction{
		SourceAccount: opts.OpSourceAccount,
		// The HostFunction must be constructed using `xdr` objects, unlike other operations that utilize `txnbuild` objects or native Go types.
		HostFunction: xdr.HostFunction{
			Type: xdr.HostFunctionTypeHostFunctionTypeInvokeContract,
			InvokeContract: &xdr.InvokeContractArgs{
				ContractAddress: contractScAddress,
				FunctionName:    xdr.ScSymbol(opts.FunctionName),
				Args:            opts.Args,
			},
		},
	}, nil
}
