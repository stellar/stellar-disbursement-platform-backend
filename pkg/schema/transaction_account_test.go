package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_TransactionAccount_ID(t *testing.T) {
	testCases := []struct {
		account     TransactionAccount
		wantResult  string
		shouldPanic bool
	}{
		{
			account: TransactionAccount{
				Address: "GDNQ3FF7MUHK3OTZMYQ63XYMMROEIVCMEJABUGQQWUT7CTC2OUBHI32B",
				Type:    HostStellarEnv,
			},
			wantResult: "stellar:GDNQ3FF7MUHK3OTZMYQ63XYMMROEIVCMEJABUGQQWUT7CTC2OUBHI32B",
		},
		{
			account: TransactionAccount{
				Address: "GDNQ3FF7MUHK3OTZMYQ63XYMMROEIVCMEJABUGQQWUT7CTC2OUBHI32B",
				Type:    ChannelAccountStellarDB,
			},
			wantResult: "stellar:GDNQ3FF7MUHK3OTZMYQ63XYMMROEIVCMEJABUGQQWUT7CTC2OUBHI32B",
		},
		{
			account: TransactionAccount{
				Address: "GDNQ3FF7MUHK3OTZMYQ63XYMMROEIVCMEJABUGQQWUT7CTC2OUBHI32B",
				Type:    DistributionAccountStellarEnv,
			},
			wantResult: "stellar:GDNQ3FF7MUHK3OTZMYQ63XYMMROEIVCMEJABUGQQWUT7CTC2OUBHI32B",
		},
		{
			account: TransactionAccount{
				Address: "GDNQ3FF7MUHK3OTZMYQ63XYMMROEIVCMEJABUGQQWUT7CTC2OUBHI32B",
				Type:    DistributionAccountStellarDBVault,
			},
			wantResult: "stellar:GDNQ3FF7MUHK3OTZMYQ63XYMMROEIVCMEJABUGQQWUT7CTC2OUBHI32B",
		},
		{
			account: TransactionAccount{
				CircleWalletID: "1000066041",
				Type:           DistributionAccountCircleDBVault,
			},
			wantResult: "circle:1000066041",
		},
		{
			account:     TransactionAccount{Type: AccountType("unsupported")},
			shouldPanic: true,
		},
	}

	for _, tc := range testCases {
		t.Run(string(tc.account.Type), func(t *testing.T) {
			if tc.shouldPanic {
				assert.Panics(t, func() {
					tc.account.ID()
				})
			} else {
				assert.Equal(t, tc.wantResult, tc.account.ID())
			}
		})
	}
}
