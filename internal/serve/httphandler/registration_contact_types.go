package httphandler

import (
	"net/http"
	"slices"

	"github.com/stellar/go/support/render/httpjson"
	"golang.org/x/exp/maps"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
)

type RegistrationContactTypesHandler struct{}

func (c RegistrationContactTypesHandler) Get(w http.ResponseWriter, r *http.Request) {
	allTypes := data.GetAllReceiverContactTypes()
	allTypesWithWalletAddress := make(map[string]bool, 2*len(allTypes))
	for _, t := range allTypes {
		allTypesWithWalletAddress[string(t)] = true
		allTypesWithWalletAddress[string(t)+"_AND_WALLET_ADDRESS"] = true
	}

	sortedKeys := maps.Keys(allTypesWithWalletAddress)
	slices.Sort(sortedKeys)

	httpjson.Render(w, sortedKeys, httpjson.JSON)
}
