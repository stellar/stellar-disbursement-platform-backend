package events

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stellar/go/support/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_ProduceEvent(t *testing.T) {
	ctx := context.Background()

	msg := Message{
		Topic:    "test_topic",
		Key:      "test_key",
		TenantID: "test_tenant_id",
		Type:     "test_type",
		Data:     "some-data",
	}

	testCases := []struct {
		name            string
		getProducer     func(t *testing.T) Producer
		msg             *Message
		assertLog       func(t *testing.T, logEntries []logrus.Entry)
		wantErrContains string
	}{
		{
			name:        "when the message is nil, logs a warning and does not produce the event",
			getProducer: func(t *testing.T) Producer { return NewMockProducer(t) },
			msg:         nil,
			assertLog: func(t *testing.T, logEntries []logrus.Entry) {
				assert.Len(t, logEntries, 1)
				assert.Equal(t, "message is nil, not producing event", logEntries[0].Message)
				assert.Equal(t, logrus.WarnLevel, logEntries[0].Level)
			},
		},
		{
			name:        "when the producer is nil, logs an error",
			getProducer: func(t *testing.T) Producer { return nil },
			msg:         &msg,
			assertLog: func(t *testing.T, logEntries []logrus.Entry) {
				assert.Len(t, logEntries, 1)
				expectedLogText := fmt.Sprintf("event producer is nil, could not publish message %+v", msg)
				assert.Equal(t, expectedLogText, logEntries[0].Message)
				assert.Equal(t, logrus.ErrorLevel, logEntries[0].Level)
			},
		},
		{
			name: "returns an error when WriteMessage fails",
			getProducer: func(t *testing.T) Producer {
				mProducer := NewMockProducer(t)
				mProducer.
					On("WriteMessages", ctx, []Message{msg}).
					Return(errors.New("some issue ocurred")).
					Once()
				return mProducer
			},
			msg:             &msg,
			wantErrContains: fmt.Sprintf("writing message %+v on event producer: %v", msg, errors.New("some issue ocurred")),
		},
		{
			name: "🎉 successfully write the message",
			getProducer: func(t *testing.T) Producer {
				mProducer := NewMockProducer(t)
				mProducer.
					On("WriteMessages", ctx, []Message{msg}).
					Return(nil).
					Once()
				return mProducer
			},
			msg: &msg,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			getEntries := log.DefaultLogger.StartTest(log.DebugLevel)

			producer := tc.getProducer(t)
			err := ProduceEvent(ctx, producer, tc.msg)
			if tc.wantErrContains != "" {
				require.ErrorContains(t, err, tc.wantErrContains)
			} else {
				require.NoError(t, err)
			}

			logEntries := getEntries()
			if tc.assertLog != nil {
				tc.assertLog(t, logEntries)
			}
		})
	}
}
