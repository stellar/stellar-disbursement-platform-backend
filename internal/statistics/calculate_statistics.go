package statistics

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

var ErrResourcesNotFound = errors.New("resources not found")

type PaymentCounters struct {
	Draft    int64 `json:"draft"`
	Ready    int64 `json:"ready"`
	Pending  int64 `json:"pending"`
	Paused   int64 `json:"paused"`
	Success  int64 `json:"success"`
	Failed   int64 `json:"failed"`
	Canceled int64 `json:"canceled"`
	Total    int64 `json:"total"`
}

type PaymentAmounts struct {
	Draft    string `json:"draft"`
	Ready    string `json:"ready"`
	Pending  string `json:"pending"`
	Paused   string `json:"paused"`
	Success  string `json:"success"`
	Failed   string `json:"failed"`
	Canceled string `json:"canceled"`
	Average  string `json:"average"`
	Total    string `json:"total"`
}

type PaymentAmountsByAsset struct {
	AssetCode      string         `json:"asset_code"`
	PaymentAmounts PaymentAmounts `json:"payment_amounts"`
}

type GeneralStatistics struct {
	DisbursementsStatistics
	TotalDisbursement int64 `json:"total_disbursements"`
}

type DisbursementsStatistics struct {
	PaymentCounters         PaymentCounters         `json:"payment_counters"`
	PaymentAmountsByAsset   []PaymentAmountsByAsset `json:"payment_amounts_by_asset"`
	ReceiverWalletsCounters ReceiverWalletsCounters `json:"receiver_wallets_counters"`
	TotalReceivers          int64                   `json:"total_receivers"`
}

type ReceiverWalletsCounters struct {
	Draft      int64 `json:"draft"`
	Ready      int64 `json:"ready"`
	Registered int64 `json:"registered"`
	Flagged    int64 `json:"flagged"`
	Total      int64 `json:"total"`
}

// getPaymentsStats returns payment statistics aggregated by payment status, if a disbursement ID
// is sent in the parameters the payment stats will be calculated for a specific disbursement.
func getPaymentsStats(ctx context.Context, sqlExec db.SQLExecuter, disbursementID string) (*PaymentCounters, []PaymentAmountsByAsset, error) {
	query := []string{
		0: "SELECT code, status, Count(*), Sum(p.amount)",
		1: "FROM payments p",
		2: "JOIN assets a ON p.asset_id=a.id",
		3: "",
		4: "GROUP BY (a.code, p.status)",
		5: "ORDER BY (a.code);",
	}

	var args []interface{}
	if disbursementID != "" {
		query[3] = "WHERE p.disbursement_id = $1"
		args = append(args, disbursementID)
	}

	rows, err := sqlExec.QueryxContext(ctx, strings.Join(query, " "), args...)
	if err != nil {
		return nil, nil, fmt.Errorf("getting payments data in getPaymentsStats: %w", err)
	}

	defer db.CloseRows(ctx, rows)

	currentCode := ""
	paymentCounters := PaymentCounters{}
	paymentAmounts := PaymentAmounts{}

	paymentsAmountsByAsset := []PaymentAmountsByAsset{}
	var totalAmount float64
	var totalCount int64

	for rows.Next() {
		var (
			code, status, amount string
			count                int64
		)

		err = rows.Scan(&code, &status, &count, &amount)
		if err != nil {
			return nil, nil, fmt.Errorf("attributing values to rows in getPaymentsStats: %w", err)
		}

		if currentCode != code {

			if currentCode != "" {
				avg := totalAmount / float64(totalCount)
				paymentAmounts.Total = utils.FloatToString(totalAmount)
				paymentAmounts.Average = utils.FloatToString(avg)
				totalAmount = 0
				totalCount = 0

				paymentsAmountsByAsset = append(
					paymentsAmountsByAsset,
					PaymentAmountsByAsset{
						AssetCode:      currentCode,
						PaymentAmounts: paymentAmounts,
					},
				)

				paymentAmounts = PaymentAmounts{}
			}

			currentCode = code
		}

		switch data.PaymentStatus(status) {
		case data.DraftPaymentStatus:
			paymentCounters.Draft += count
			paymentAmounts.Draft = amount

		case data.PendingPaymentStatus:
			paymentCounters.Pending += count
			paymentAmounts.Pending = amount

		case data.ReadyPaymentStatus:
			paymentCounters.Ready += count
			paymentAmounts.Ready = amount

		case data.SuccessPaymentStatus:
			paymentCounters.Success += count
			paymentAmounts.Success = amount

		case data.FailedPaymentStatus:
			paymentCounters.Failed += count
			paymentAmounts.Failed = amount

		case data.PausedPaymentStatus:
			paymentCounters.Paused += count
			paymentAmounts.Paused = amount

		case data.CanceledPaymentStatus:
			paymentCounters.Canceled += count
			paymentAmounts.Canceled = amount
		default:
			return nil, nil, fmt.Errorf("status %v is not a valid payment status", status)
		}

		paymentCounters.Total += count

		totalCount += count
		if value, parseErr := strconv.ParseFloat(amount, 64); parseErr != nil {
			return nil, nil, fmt.Errorf("error parsing payment amount: %w", err)
		} else {
			totalAmount += value
		}
	}

	if err = rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("end scanning: %w", err)
	}

	if currentCode != "" {
		avg := totalAmount / float64(totalCount)
		paymentAmounts.Total = utils.FloatToString(totalAmount)
		paymentAmounts.Average = utils.FloatToString(avg)

		paymentsAmountsByAsset = append(
			paymentsAmountsByAsset,
			PaymentAmountsByAsset{
				AssetCode:      currentCode,
				PaymentAmounts: paymentAmounts,
			},
		)
	}

	return &paymentCounters, paymentsAmountsByAsset, nil
}

// getReceiverWalletsStats returns receiver wallets statistics aggregated by receiver wallet status, if a disbursement
// ID is sent in the parameters the receiver wallet stats will be calculated for a specific disbursement.
func getReceiverWalletsStats(ctx context.Context, sqlExec db.SQLExecuter, disbursementID string) (*ReceiverWalletsCounters, error) {
	query := []string{
		0: "SELECT rw.status, Count(DISTINCT rw.receiver_id)",
		1: "FROM receiver_wallets rw",
		2: "LEFT JOIN payments p ON p.receiver_wallet_id=rw.id",
		3: "",
		4: "GROUP BY (rw.status);",
	}

	var args []interface{}
	if disbursementID != "" {
		query[3] = "WHERE p.disbursement_id = $1"
		args = append(args, disbursementID)
	}

	rows, err := sqlExec.QueryxContext(ctx, strings.Join(query, " "), args...)
	if err != nil {
		return nil, fmt.Errorf("getting receivers wallet data by asset: %w", err)
	}

	defer db.CloseRows(ctx, rows)

	receiverWalletsCounters := ReceiverWalletsCounters{}

	for rows.Next() {
		var (
			status string
			count  int64
		)

		err = rows.Scan(&status, &count)

		if err != nil {
			return nil, fmt.Errorf("attributing values to rows: %w", err)
		}

		switch data.ReceiversWalletStatus(status) {
		case data.DraftReceiversWalletStatus:
			receiverWalletsCounters.Draft = count

		case data.ReadyReceiversWalletStatus:
			receiverWalletsCounters.Ready = count

		case data.RegisteredReceiversWalletStatus:
			receiverWalletsCounters.Registered = count

		case data.FlaggedReceiversWalletStatus:
			receiverWalletsCounters.Flagged = count

		default:
			return nil, fmt.Errorf("status %v is not a valid receiver wallet status", status)
		}

		receiverWalletsCounters.Total += count
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("end scanning: %w", err)
	}

	return &receiverWalletsCounters, nil
}

// getTotalReceivers returns total amount of receivers, if a disbursement ID is sent in the parameters
// then the total amount of receivers present in the specific disbursement is returned.
func getTotalReceivers(ctx context.Context, sqlExec db.SQLExecuter, disbursementID string) (int64, error) {
	var args []interface{}
	query := "SELECT COUNT(DISTINCT r.id) FROM receivers r"

	if disbursementID != "" {
		query += " JOIN payments p ON p.receiver_id = r.id WHERE p.disbursement_id = $1"
		args = append(args, disbursementID)
	}

	var totalReceivers int64
	err := sqlExec.GetContext(ctx, &totalReceivers, query, args...)
	if err != nil {
		return 0, fmt.Errorf("getting total receiver data: %w", err)
	}

	return totalReceivers, nil
}

// getTotalDisbursements returns total amount of disbursements.
func getTotalDisbursements(ctx context.Context, sqlExec db.SQLExecuter) (totalDisbursement int64, err error) {
	q := "SELECT COUNT(*) FROM disbursements"
	err = sqlExec.GetContext(ctx, &totalDisbursement, q)
	if err != nil {
		return 0, fmt.Errorf("getting total disbursement data: %w", err)
	}

	return totalDisbursement, nil
}

// CalculateStatistics calculate statistics for all disbursements.
func CalculateStatistics(ctx context.Context, dbConnectionPool db.DBConnectionPool) (statistics *GeneralStatistics, err error) {
	// Start transaction
	dbTx, err := dbConnectionPool.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("starting transaction in CalculateStatistics: %w", err)
	}
	defer func() {
		db.DBTxRollback(ctx, dbTx, err, "error in CalculateStatistics")
	}()

	paymentCounters, paymentAmountByAsset, err := getPaymentsStats(ctx, dbTx, "")
	if err != nil {
		return nil, err
	}

	receiverWalletsCounters, err := getReceiverWalletsStats(ctx, dbTx, "")
	if err != nil {
		return nil, err
	}

	totalReceivers, err := getTotalReceivers(ctx, dbTx, "")
	if err != nil {
		return nil, err
	}

	totalDisbursement, err := getTotalDisbursements(ctx, dbTx)
	if err != nil {
		return nil, err
	}

	err = dbTx.Commit()
	if err != nil {
		return nil, fmt.Errorf("commiting transaction in CalculateStatistics: %w", err)
	}

	statistics = &GeneralStatistics{TotalDisbursement: totalDisbursement}
	statistics.PaymentCounters = *paymentCounters
	statistics.PaymentAmountsByAsset = paymentAmountByAsset
	statistics.ReceiverWalletsCounters = *receiverWalletsCounters
	statistics.TotalReceivers = totalReceivers
	return statistics, nil
}

// CalculateStatisticsByDisbursement calculate statistics for a specific disbursement.
func CalculateStatisticsByDisbursement(ctx context.Context, dbConnectionPool db.DBConnectionPool, disbursementID string) (statistics *DisbursementsStatistics, err error) {
	// Start transaction
	dbTx, err := dbConnectionPool.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("starting transaction in CalculateStatisticsByDisbursement: %w", err)
	}
	defer func() {
		db.DBTxRollback(ctx, dbTx, err, "error in CalculateStatisticsByDisbursement")
	}()

	disbursementExists, err := checkIfDisbursementExists(ctx, dbTx, disbursementID)
	if err != nil {
		return nil, fmt.Errorf("checking if disbursement exists in CalculateStatisticsByDisbursement: %w", err)
	}
	if !disbursementExists {
		return nil, ErrResourcesNotFound
	}

	paymentCounters, paymentAmountByAsset, err := getPaymentsStats(ctx, dbTx, disbursementID)
	if err != nil {
		return nil, err
	}

	receiverWalletsCounters, err := getReceiverWalletsStats(ctx, dbTx, disbursementID)
	if err != nil {
		return nil, err
	}

	totalReceivers, err := getTotalReceivers(ctx, dbTx, disbursementID)
	if err != nil {
		return nil, err
	}

	err = dbTx.Commit()
	if err != nil {
		return nil, fmt.Errorf("commiting transaction in CalculateStatisticsByDisbursement: %w", err)
	}

	statistics = &DisbursementsStatistics{
		PaymentCounters:         *paymentCounters,
		PaymentAmountsByAsset:   paymentAmountByAsset,
		ReceiverWalletsCounters: *receiverWalletsCounters,
		TotalReceivers:          totalReceivers,
	}
	return statistics, nil
}

func checkIfDisbursementExists(ctx context.Context, sqlExec db.SQLExecuter, disbursementID string) (exists bool, err error) {
	// Check if the disbursement exists
	query := "SELECT EXISTS(SELECT 1 FROM disbursements WHERE id = $1)"
	err = sqlExec.QueryRowxContext(ctx, query, disbursementID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking disbursement existence: %w", err)
	}

	return exists, nil
}
