package data

import (
	"context"
	"fmt"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
)

type DisbursementReceiver struct {
	ID             string          `json:"id" db:"id"`
	Email          string          `json:"email,omitempty" db:"email"`
	PhoneNumber    string          `json:"phone_number" db:"phone_number"`
	ExternalID     string          `json:"external_id" db:"external_id"`
	ReceiverWallet *ReceiverWallet `json:"receiver_wallet" db:"receiver_wallet"`
	Payment        *Payment        `json:"payment" db:"payment"`
	CreatedAt      time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at" db:"updated_at"`
}

type DisbursementReceiverModel struct {
	dbConnectionPool db.DBConnectionPool
}

func (m DisbursementReceiverModel) Count(ctx context.Context, sqlExec db.SQLExecuter, disbursementID string) (int, error) {
	var count int
	query := `
		SELECT
			count(*)
		FROM
			receivers r
		JOIN payments p ON r.id = p.receiver_id
		WHERE p.disbursement_id = $1
		`

	err := sqlExec.GetContext(ctx, &count, query, disbursementID)
	if err != nil {
		return 0, fmt.Errorf("error counting disbursement receivers for disbursement ID %s: %w", disbursementID, err)
	}
	return count, nil
}

func (m DisbursementReceiverModel) GetAll(ctx context.Context, sqlExec db.SQLExecuter, queryParams *QueryParams, disbursementID string) ([]*DisbursementReceiver, error) {
	var receivers []*DisbursementReceiver
	baseQuery := `
		SELECT
			` + ReceiverColumnNames("r", "") + `,
			` + ReceiverWalletColumnNames("rw", "receiver_wallet") + `,
			` + WalletColumnNames("w", "receiver_wallet.wallet", false) + `,
			p.id as "payment.id",
			p.amount as "payment.amount",
			p.status as "payment.status",
			COALESCE(p.stellar_transaction_id, '') as "payment.stellar_transaction_id",
			COALESCE(p.stellar_operation_id, '') as "payment.stellar_operation_id",
			p.created_at as "payment.created_at",
			p.updated_at as "payment.updated_at",
			` + AssetColumnNames("a", "payment.asset", false) + `
		FROM
			receivers r
		JOIN payments p ON r.id = p.receiver_id
		JOIN receiver_wallets rw ON rw.id = p.receiver_wallet_id
		JOIN wallets w ON rw.wallet_id = w.id
		JOIN assets a ON p.asset_id = a.id
		`

	query, params := m.newDisbursementReceiversQuery(baseQuery, queryParams, disbursementID)
	err := sqlExec.SelectContext(ctx, &receivers, query, params...)
	if err != nil {
		return nil, fmt.Errorf("error getting receivers: %w", err)
	}
	return receivers, nil
}

func (m DisbursementReceiverModel) newDisbursementReceiversQuery(baseQuery string, queryParams *QueryParams, disbursementID string) (string, []interface{}) {
	qb := NewQueryBuilder(baseQuery)
	qb.AddCondition("p.disbursement_id = ?", disbursementID)
	qb.AddSorting(queryParams.SortBy, queryParams.SortOrder, "r")
	qb.AddPagination(queryParams.Page, queryParams.PageLimit)
	query, params := qb.Build()
	return m.dbConnectionPool.Rebind(query), params
}
