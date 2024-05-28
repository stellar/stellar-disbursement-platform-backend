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

func Test_ProduceEvents(t *testing.T) {
	ctx := context.Background()

	msg1 := &Message{
		Topic:    "test_topic_1",
		Key:      "test_key_1",
		TenantID: "test_tenant_id_1",
		Type:     "test_type_1",
		Data:     "some-data_1",
	}
	msg2 := &Message{
		Topic:    "test_topic_2",
		Key:      "test_key_2",
		TenantID: "test_tenant_id_2",
		Type:     "test_type_2",
		Data:     "some-data_2",
	}

	testCases := []struct {
		name            string
		getProducer     func(t *testing.T) Producer
		messages        []*Message
		assertLog       func(t *testing.T, logEntries []logrus.Entry)
		wantErrContains string
	}{
		{
			name:        "when the producer is nil, logs an error",
			getProducer: func(t *testing.T) Producer { return nil },
			messages:    []*Message{msg1},
			assertLog: func(t *testing.T, logEntries []logrus.Entry) {
				assert.Len(t, logEntries, 1)
				expectedLogText := fmt.Sprintf("event producer is nil, could not publish messages %+v", []Message{*msg1})
				assert.Equal(t, expectedLogText, logEntries[0].Message)
				assert.Equal(t, logrus.ErrorLevel, logEntries[0].Level)
			},
		},
		{
			name:        "when all messages are nil, log warnings and does not produce any event",
			getProducer: func(t *testing.T) Producer { return NewMockProducer(t) },
			messages:    []*Message{nil, nil},
			assertLog: func(t *testing.T, logEntries []logrus.Entry) {
				assert.Len(t, logEntries, 3)
				assert.Equal(t, "message at index 0 is nil, not producing event", logEntries[0].Message)
				assert.Equal(t, logrus.WarnLevel, logEntries[0].Level)
				assert.Equal(t, "message at index 1 is nil, not producing event", logEntries[1].Message)
				assert.Equal(t, logrus.WarnLevel, logEntries[1].Level)
				assert.Equal(t, "not producing events, since there are zero not-nil messages to produce", logEntries[2].Message)
				assert.Equal(t, logrus.WarnLevel, logEntries[2].Level)
			},
		},
		{
			name:        "when there's no messages, logs a warning and does not produce any event",
			getProducer: func(t *testing.T) Producer { return NewMockProducer(t) },
			messages:    nil,
			assertLog: func(t *testing.T, logEntries []logrus.Entry) {
				assert.Len(t, logEntries, 1)
				assert.Equal(t, "not producing events, since there are zero not-nil messages to produce", logEntries[0].Message)
				assert.Equal(t, logrus.WarnLevel, logEntries[0].Level)
			},
		},
		{
			name: "returns an error when WriteMessage fails",
			getProducer: func(t *testing.T) Producer {
				mProducer := NewMockProducer(t)
				mProducer.
					On("WriteMessages", ctx, []Message{*msg1}).
					Return(errors.New("some issue ocurred")).
					Once()
				return mProducer
			},
			messages:        []*Message{msg1},
			wantErrContains: fmt.Sprintf("writing messages %+v on event producer: %v", []*Message{msg1}, errors.New("some issue ocurred")),
		},
		{
			name: "🎉 successfully writes one message",
			getProducer: func(t *testing.T) Producer {
				mProducer := NewMockProducer(t)
				mProducer.
					On("WriteMessages", ctx, []Message{*msg1}).
					Return(nil).
					Once()
				return mProducer
			},
			messages: []*Message{msg1},
		},
		{
			name: "🎉 successfully writes only not-nil messages",
			getProducer: func(t *testing.T) Producer {
				mProducer := NewMockProducer(t)
				mProducer.
					On("WriteMessages", ctx, []Message{*msg1, *msg2}).
					Return(nil).
					Once()
				return mProducer
			},
			messages: []*Message{msg1, nil, msg2},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			getEntries := log.DefaultLogger.StartTest(log.DebugLevel)

			producer := tc.getProducer(t)
			err := ProduceEvents(ctx, producer, tc.messages...)
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
