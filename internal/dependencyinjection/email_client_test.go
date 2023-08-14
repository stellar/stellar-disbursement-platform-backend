package dependencyinjection

import (
	"testing"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewEmailClient(t *testing.T) {
	t.Run("should return an error on a invalid EMAIL type", func(t *testing.T) {
		defer ClearInstancesTestHelper(t)

		mockEmailClientOptions := EmailClientOptions{EmailType: "foo-bar"}

		gotClient, err := NewEmailClient(mockEmailClientOptions)
		require.Nil(t, gotClient)
		require.Error(t, err)
		assert.EqualError(t, err, `trying to create a Email client with a non-supported Email type: "foo-bar"`)
	})

	t.Run("should return the same instance when called twice for the same Email type", func(t *testing.T) {
		defer ClearInstancesTestHelper(t)

		// STEP 1: assert that DRY_RUN email client should not be instantiated more than once
		mockEmailClientOptions := EmailClientOptions{
			EmailType: message.MessengerTypeDryRun,
		}

		dryRunClient1, err := NewEmailClient(mockEmailClientOptions)
		require.NoError(t, err)
		dryRunClient2, err := NewEmailClient(mockEmailClientOptions)
		require.NoError(t, err)
		assert.Equal(t, &dryRunClient1, &dryRunClient2)

		// STEP 2: assert that AWS email client should not be instantiated more than once
		mockEmailClientOptions = EmailClientOptions{
			EmailType: message.MessengerTypeAWSEmail,
			MessengerOptions: &message.MessengerOptions{
				Environment:        "dev",
				AWSAccessKeyID:     "testtesttesttesttesttest",
				AWSSecretAccessKey: "testtesttesttesttesttest",
				AWSRegion:          "testtesttesttesttesttest",
				AWSSESSenderID:     "test_email@email.com",
			},
		}

		awsClient1, err := NewEmailClient(mockEmailClientOptions)
		require.NoError(t, err)
		awsClient2, err := NewEmailClient(mockEmailClientOptions)
		require.NoError(t, err)
		assert.Equal(t, &awsClient1, &awsClient2)

		// STEP 3: assert that twilio and aws clients are different
		assert.NotEqual(t, &dryRunClient1, &awsClient1)
		assert.NotEqual(t, dryRunClient1, awsClient1)
	})

	t.Run("should return an error on a invalid instance", func(t *testing.T) {
		defer ClearInstancesTestHelper(t)

		mockTestEmailClientOptions := EmailClientOptions{
			EmailType: message.MessengerTypeAWSEmail,
			MessengerOptions: &message.MessengerOptions{
				Environment:        "test",
				AWSAccessKeyID:     "testtesttesttesttesttest",
				AWSSecretAccessKey: "testtesttesttesttesttest",
				AWSRegion:          "testtesttesttesttesttest",
				AWSSESSenderID:     "test_email@email.com",
			},
		}

		preExistingEmailClientWithInvalidType := struct{}{}
		setInstance(buildEmailClientInstanceName(message.MessengerTypeAWSEmail), preExistingEmailClientWithInvalidType)

		gotClient, err := NewEmailClient(mockTestEmailClientOptions)
		assert.Nil(t, gotClient)
		assert.EqualError(t, err, "trying to cast pre-existing Email client for depencency injection")
	})
}
