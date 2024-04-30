package monitor

import (
	"context"
	"database/sql"
	"slices"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stellar/go/support/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	sdpMonitor "github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	sdpMonitorMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/monitor/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
)

func Test_TSSMonitorService_LogAndMonitorTransaction(t *testing.T) {
	mMonitorClient := sdpMonitorMocks.MockMonitorClient{}
	tssMonitorSvc := TSSMonitorService{
		Client:        &mMonitorClient,
		GitCommitHash: "0xABC",
		Version:       "0.01",
	}

	time := time.Now()
	assetCode := "USDC"
	srcChannelAcc := "0xSRC"
	destAcc := "0xDEST"
	xdr := sql.NullString{String: "AAAAAAAAAMgAAAAAAAAAAgAAAAAAAAAGAAAAAAAAAAAAAAAFAAAAAAAAAAA=", Valid: true}
	txHash := "0xSUCCESS"
	tenantID := "test_tenant_id"
	txID := "test_tx_id"
	errStr := "error!"

	testCases := []struct {
		name       string
		metricTag  sdpMonitor.MetricTag
		txModel    store.Transaction
		txMetadata TxMetadata
		eventType  string
		logLevel   logrus.Level
		fieldsMap  map[string]interface{}
	}{
		{
			name:      "monitor payment_processing_started",
			metricTag: sdpMonitor.PaymentProcessingStartedTag,
			txMetadata: TxMetadata{
				PaymentEventType: sdpMonitor.PaymentProcessingStartedLabel,
				SrcChannelAcc:    srcChannelAcc,
			},
			txModel: store.Transaction{
				ID:          "test_tx_id",
				CreatedAt:   &time,
				UpdatedAt:   &time,
				AssetCode:   assetCode,
				Destination: destAcc,
				TenantID:    "test_tenant_id",
			},
			logLevel: log.DebugLevel,
			fieldsMap: map[string]interface{}{
				"app_version":     tssMonitorSvc.Version,
				"channel_account": srcChannelAcc,
				"event_type":      sdpMonitor.PaymentProcessingStartedLabel,
				"git_commit_hash": tssMonitorSvc.GitCommitHash,
				"tenant_id":       tenantID,
				"tx_id":           txID,
			},
		},
		{
			name:      "monitor payment_reconciliatoin_successful",
			metricTag: sdpMonitor.PaymentReconciliationSuccessfulTag,
			txMetadata: TxMetadata{
				PaymentEventType: sdpMonitor.PaymentReconciliationTransactionSuccessfulLabel,
				SrcChannelAcc:    srcChannelAcc,
			},
			txModel: store.Transaction{
				ID:                     txID,
				TenantID:               tenantID,
				CreatedAt:              &time,
				UpdatedAt:              &time,
				CompletedAt:            &time,
				AssetCode:              assetCode,
				Destination:            destAcc,
				XDRSent:                xdr,
				XDRReceived:            xdr,
				StellarTransactionHash: sql.NullString{String: txHash, Valid: true},
			},
			logLevel: log.InfoLevel,
			fieldsMap: map[string]interface{}{
				"app_version":     tssMonitorSvc.Version,
				"channel_account": srcChannelAcc,
				"completed_at":    time.String(),
				"event_type":      sdpMonitor.PaymentReconciliationTransactionSuccessfulLabel,
				"git_commit_hash": tssMonitorSvc.GitCommitHash,
				"tenant_id":       tenantID,
				"tx_hash":         txHash,
				"tx_id":           txID,
				"xdr_received":    xdr.String,
				"xdr_sent":        xdr.String,
			},
		},
		{
			name:      "monitor payment_reconciliatoin_successful",
			metricTag: sdpMonitor.PaymentErrorTag,
			txMetadata: TxMetadata{
				PaymentEventType: sdpMonitor.PaymentFailedLabel,
				SrcChannelAcc:    srcChannelAcc,
				IsHorizonErr:     true,
				ErrStack:         errStr,
			},
			txModel: store.Transaction{
				ID:                     txID,
				TenantID:               tenantID,
				CreatedAt:              &time,
				UpdatedAt:              &time,
				AssetCode:              assetCode,
				Destination:            destAcc,
				XDRSent:                xdr,
				StellarTransactionHash: sql.NullString{String: txHash, Valid: true},
			},
			logLevel: log.InfoLevel,
			fieldsMap: map[string]interface{}{
				"app_version":     tssMonitorSvc.Version,
				"channel_account": srcChannelAcc,
				"error":           errStr,
				"event_type":      sdpMonitor.PaymentFailedLabel,
				"git_commit_hash": tssMonitorSvc.GitCommitHash,
				"horizon_error?":  true,
				"tenant_id":       tenantID,
				"tx_hash":         txHash,
				"tx_id":           txID,
				"xdr_sent":        xdr.String,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			getLogEntries := log.DefaultLogger.StartTest(tc.logLevel)

			ctx := context.Background()

			mMonitorClient.On("MonitorCounters", tc.metricTag, mock.Anything).Return(nil).Once()
			tssMonitorSvc.LogAndMonitorTransaction(ctx, tc.txModel, tc.metricTag, tc.txMetadata)

			logEntries := getLogEntries()
			assert.NotEmpty(t, logEntries[0])

			logFieldsThatCannotBeAsserted := []string{"event_id", "event_time", "pid"}
			assert.Len(t, logEntries[0].Data, len(tc.fieldsMap)+len(logFieldsThatCannotBeAsserted))

			for k, v := range logEntries[0].Data {
				if slices.Contains(logFieldsThatCannotBeAsserted, k) {
					continue
				}
				assert.Equal(t, v, tc.fieldsMap[k], "failed value comparison for key: %s", k)
			}

			assert.Contains(t, logEntries[0].Message, string(tc.metricTag))
		})
	}
}
