package dependencyinjection

import (
	"testing"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewSMSClient(t *testing.T) {
	t.Run("should return an error on a invalid SMS type", func(t *testing.T) {
		defer ClearInstancesTestHelper(t)

		mockSMSClientOptions := SMSClientOptions{SMSType: "foo-bar"}

		gotClient, err := NewSMSClient(mockSMSClientOptions)
		require.Nil(t, gotClient)
		require.Error(t, err)
		assert.EqualError(t, err, `trying to create a SMS client with a non-supported SMS type: "foo-bar"`)
	})

	t.Run("should return the same instance when called twice for the same SMS type", func(t *testing.T) {
		defer ClearInstancesTestHelper(t)

		// STEP 1: assert that Twilio client should not be instantiated more than once
		mockSMSClientOptions := SMSClientOptions{
			SMSType: message.MessengerTypeTwilioSMS,
			MessengerOptions: &message.MessengerOptions{
				Environment:      "dev",
				TwilioAccountSID: "testtesttesttesttest",
				TwilioAuthToken:  "testtesttesttesttest",
				TwilioServiceSID: "testtesttesttesttest",
			},
		}

		twilioClient1, err := NewSMSClient(mockSMSClientOptions)
		require.NoError(t, err)
		twilioClient2, err := NewSMSClient(mockSMSClientOptions)
		require.NoError(t, err)
		assert.Equal(t, &twilioClient1, &twilioClient2)

		// STEP 2: assert that AWS sms client should not be instantiated more than once
		mockSMSClientOptions = SMSClientOptions{
			SMSType: message.MessengerTypeAWSSMS,
			MessengerOptions: &message.MessengerOptions{
				Environment:        "dev",
				AWSAccessKeyID:     "testtesttesttesttesttest",
				AWSSecretAccessKey: "testtesttesttesttesttest",
				AWSRegion:          "testtesttesttesttesttest",
			},
		}

		awsClient1, err := NewSMSClient(mockSMSClientOptions)
		require.NoError(t, err)
		awsClient2, err := NewSMSClient(mockSMSClientOptions)
		require.NoError(t, err)
		assert.Equal(t, &awsClient1, &awsClient2)

		// STEP 3: assert that twilio and aws clients are different
		assert.NotEqual(t, &twilioClient1, &awsClient1)
		assert.NotEqual(t, twilioClient1, awsClient1)
	})

	t.Run("should return an error on a invalid pre-existing instance", func(t *testing.T) {
		defer ClearInstancesTestHelper(t)

		mockTestSMSClientOptions := SMSClientOptions{
			SMSType: message.MessengerTypeTwilioSMS,
			MessengerOptions: &message.MessengerOptions{
				Environment:      "test",
				TwilioAccountSID: "testtesttesttesttest",
				TwilioAuthToken:  "testtesttesttesttest",
				TwilioServiceSID: "testtesttesttesttest",
			},
		}

		preExistingSMSClientWithInvalidType := struct{}{}
		setInstance(buildSMSClientInstanceName(message.MessengerTypeTwilioSMS), preExistingSMSClientWithInvalidType)

		gotClient, err := NewSMSClient(mockTestSMSClientOptions)
		assert.Nil(t, gotClient)
		assert.EqualError(t, err, "trying to cast pre-existing SMS client for depencency injection")
	})
}
