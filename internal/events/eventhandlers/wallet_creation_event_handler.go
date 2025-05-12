package eventhandlers

import "github.com/stellar/stellar-disbursement-platform-backend/db"

type WalletCreationEventHandlerOptions struct {
	AdminDBConnectionPool db.DBConnectionPool
	MtnDBConnectionPool   db.DBConnectionPool
	TSSDBConnectionPool   db.DBConnectionPool
}
