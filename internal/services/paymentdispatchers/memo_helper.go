package paymentdispatchers

import (
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

func GetMemoAndType(receiverWallet data.ReceiverWallet) (string, schema.MemoType) {
	if receiverWallet.StellarMemo != "" {
		return receiverWallet.StellarMemo, schema.MemoTypeID
	}

	return "", ""
	// return "foo-bar", schema.MemoTypeText
}
