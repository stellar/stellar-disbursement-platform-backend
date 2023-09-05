package monitor

import (
	"context"
	"database/sql"
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

func Test_TSSMonitorService_MonitorPayment(t *testing.T) {
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
	errStr := "error!"

	testCases := []struct {
		name       string
		metricTag  sdpMonitor.MetricTag
		txModel    store.Transaction
		txMetadata TxMetadata
		eventType  string
		logLevel   logrus.Level
		fieldsMap  map[string]string
	}{
		{
			name:      "monitor payment_processing_started",
			metricTag: sdpMonitor.PaymentProcessingStartedTag,
			txMetadata: TxMetadata{
				PaymentEventType: sdpMonitor.PaymentProcessingStartedLabel,
				SrcChannelAcc:    srcChannelAcc,
			},
			txModel: store.Transaction{
				CreatedAt:   &time,
				UpdatedAt:   &time,
				AssetCode:   assetCode,
				Destination: destAcc,
			},
			logLevel: log.DebugLevel,
			fieldsMap: map[string]string{
				"created_at":          time.String(),
				"updated_at":          time.String(),
				"asset":               assetCode,
				"channel_account":     srcChannelAcc,
				"destination_account": destAcc,
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
			fieldsMap: map[string]string{
				"created_at":          time.String(),
				"updated_at":          time.String(),
				"asset":               assetCode,
				"channel_account":     srcChannelAcc,
				"destination_account": destAcc,
				"xdr_sent":            xdr.String,
				"xdr_received":        xdr.String,
				"completed_at":        time.String(),
				"tx_hash":             txHash,
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
				CreatedAt:   &time,
				UpdatedAt:   &time,
				AssetCode:   assetCode,
				Destination: destAcc,
				XDRSent:     xdr,
			},
			logLevel: log.InfoLevel,
			fieldsMap: map[string]string{
				"created_at":          time.String(),
				"updated_at":          time.String(),
				"asset":               assetCode,
				"channel_account":     srcChannelAcc,
				"destination_account": destAcc,
				"xdr_sent":            xdr.String,
				"error":               errStr,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			getLogEntries := log.DefaultLogger.StartTest(tc.logLevel)

			ctx := context.Background()

			mMonitorClient.On("MonitorCounters", tc.metricTag, mock.Anything).Return(nil).Once()
			tssMonitorSvc.MonitorPayment(ctx, tc.txModel, tc.metricTag, tc.txMetadata)

			logEntries := getLogEntries()
			assert.NotEmpty(t, logEntries[0])

			data := logEntries[0].Data
			for k, v := range tc.fieldsMap {
				assert.Equal(t, v, data[k])
			}

			assert.Contains(t, logEntries[0].Message, string(tc.metricTag))
		})
	}
}
