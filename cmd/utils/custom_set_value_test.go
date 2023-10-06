package utils

import (
	"go/types"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/stellar/go/support/config"
	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// customSetterTestCase is a test case to test a custom_set_value function.
type customSetterTestCase[T any] struct {
	name            string
	args            []string
	envValue        string
	wantErrContains string
	wantResult      T
}

// customSetterTester tests a custom_set_value function, according with the customSetterTestCase provided.
func customSetterTester[T any](t *testing.T, tc customSetterTestCase[T], co config.ConfigOption) {
	ClearTestEnvironment(t)
	if tc.envValue != "" {
		envName := strings.ToUpper(co.Name)
		envName = strings.ReplaceAll(envName, "-", "_")
		t.Setenv(envName, tc.envValue)
	}

	// start the CLI command
	testCmd := cobra.Command{
		RunE: func(cmd *cobra.Command, args []string) error {
			co.Require()
			return co.SetValue()
		},
	}
	// mock the command line output
	buf := new(strings.Builder)
	testCmd.SetOut(buf)

	// Initialize the command for the given option
	err := co.Init(&testCmd)
	require.NoError(t, err)

	// execute command line
	if len(tc.args) > 0 {
		testCmd.SetArgs(tc.args)
	}
	err = testCmd.Execute()

	// check the result
	if tc.wantErrContains != "" {
		assert.Error(t, err)
		assert.Contains(t, err.Error(), tc.wantErrContains)
	} else {
		assert.NoError(t, err)
	}

	if !utils.IsEmpty(tc.wantResult) {
		destPointer := utils.UnwrapInterfaceToPointer[T](co.ConfigKey)
		assert.Equal(t, tc.wantResult, *destPointer)
	}
}

func Test_SetConfigOptionMessengerType(t *testing.T) {
	opts := struct{ messengerType message.MessengerType }{}

	co := config.ConfigOption{
		Name:           "message-sender-type",
		OptType:        types.String,
		CustomSetValue: SetConfigOptionMessengerType,
		ConfigKey:      &opts.messengerType,
	}

	testCases := []customSetterTestCase[message.MessengerType]{
		{
			name:            "returns an error if the messenger type is empty",
			args:            []string{},
			wantErrContains: `couldn't parse messenger type: invalid message sender type ""`,
		},
		{
			name:            "returns an error if the messenger type is invalid",
			args:            []string{"--message-sender-type", "test"},
			wantErrContains: `couldn't parse messenger type: invalid message sender type "TEST"`,
		},
		{
			name:       "🎉 handles messenger type TWILIO_SMS (through CLI args)",
			args:       []string{"--message-sender-type", "TwIliO_sms"},
			wantResult: message.MessengerTypeTwilioSMS,
		},
		{
			name:       "🎉 handles messenger type TWILIO_SMS (through ENV vars)",
			envValue:   "TwIliO_sms",
			wantResult: message.MessengerTypeTwilioSMS,
		},
		{
			name:       "🎉 handles messenger type AWS_SMS (through CLI args)",
			args:       []string{"--message-sender-type", "AWs_SMS"},
			wantResult: message.MessengerTypeAWSSMS,
		},
		{
			name:       "🎉 handles messenger type AWS_SMS (through ENV vars)",
			envValue:   "AWs_SMS",
			wantResult: message.MessengerTypeAWSSMS,
		},
		{
			name:       "🎉 handles messenger type AWS_EMAIL (through CLI args)",
			args:       []string{"--message-sender-type", "AWS_EMAIL"},
			wantResult: message.MessengerTypeAWSEmail,
		},
		{
			name:       "🎉 handles messenger type AWS_EMAIL (through ENV vars)",
			envValue:   "AWS_EMAIL",
			wantResult: message.MessengerTypeAWSEmail,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			opts.messengerType = ""
			customSetterTester[message.MessengerType](t, tc, co)
		})
	}
}

func Test_SetConfigOptionLogLevel(t *testing.T) {
	opts := struct{ logrusLevel logrus.Level }{}

	co := config.ConfigOption{
		Name:           "log-level",
		OptType:        types.String,
		CustomSetValue: SetConfigOptionLogLevel,
		ConfigKey:      &opts.logrusLevel,
	}

	testCases := []customSetterTestCase[logrus.Level]{
		{
			name:            "returns an error if the log level is empty",
			args:            []string{},
			wantErrContains: `couldn't parse log level: not a valid logrus Level: ""`,
		},
		{
			name:            "returns an error if the log level is invalid",
			args:            []string{"--log-level", "test"},
			wantErrContains: `couldn't parse log level: not a valid logrus Level: "test"`,
		},
		{
			name:       "🎉 handles messenger type TRACE (through CLI args)",
			args:       []string{"--log-level", "TRACE"},
			wantResult: logrus.TraceLevel,
		},
		{
			name:       "🎉 handles messenger type TRACE (through ENV vars)",
			envValue:   "TRACE",
			wantResult: logrus.TraceLevel,
		},
		{
			name:       "🎉 handles messenger type INFO (through CLI args)",
			args:       []string{"--log-level", "iNfO"},
			wantResult: logrus.InfoLevel,
		},
		{
			name:       "🎉 handles messenger type INFO (through ENV vars)",
			envValue:   "INFO",
			wantResult: logrus.InfoLevel,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			opts.logrusLevel = 0
			customSetterTester[logrus.Level](t, tc, co)
		})
	}
}

func Test_SetConfigOptionMetricType(t *testing.T) {
	opts := struct{ metricType monitor.MetricType }{}

	co := config.ConfigOption{
		Name:           "metrics-type",
		OptType:        types.String,
		CustomSetValue: SetConfigOptionMetricType,
		ConfigKey:      &opts.metricType,
	}

	testCases := []customSetterTestCase[monitor.MetricType]{
		{
			name:            "returns an error if the value is empty",
			args:            []string{},
			wantErrContains: `couldn't parse metric type: invalid metric type ""`,
		},
		{
			name:            "returns an error if the value is not supported",
			args:            []string{"--metrics-type", "test"},
			wantErrContains: `couldn't parse metric type: invalid metric type "TEST"`,
		},
		{
			name:       "🎉 handles crash tracker type (through CLI args): PROMETHEUS",
			args:       []string{"--metrics-type", "PROMETHEUS"},
			wantResult: monitor.MetricTypePrometheus,
		},
		{
			name:       "🎉 handles crash tracker type (through ENV vars): PROMETHEUS",
			envValue:   "PROMETHEUS",
			wantResult: monitor.MetricTypePrometheus,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			opts.metricType = ""
			customSetterTester[monitor.MetricType](t, tc, co)
		})
	}
}

func Test_SetConfigOptionCrashTrackerType(t *testing.T) {
	opts := struct{ crashTrackerType crashtracker.CrashTrackerType }{}

	co := config.ConfigOption{
		Name:           "crash-tracker-type",
		OptType:        types.String,
		CustomSetValue: SetConfigOptionCrashTrackerType,
		ConfigKey:      &opts.crashTrackerType,
	}

	testCases := []customSetterTestCase[crashtracker.CrashTrackerType]{
		{
			name:            "returns an error if the value is empty",
			args:            []string{},
			wantErrContains: `couldn't parse crash tracker type: invalid crash tracker type ""`,
		},
		{
			name:            "returns an error if the value is not supported",
			args:            []string{"--crash-tracker-type", "test"},
			wantErrContains: `couldn't parse crash tracker type: invalid crash tracker type "TEST"`,
		},
		{
			name:       "🎉 handles crash tracker type (through CLI args): SENTRY",
			args:       []string{"--crash-tracker-type", "SeNtRy"},
			wantResult: crashtracker.CrashTrackerTypeSentry,
		},
		{
			name:       "🎉 handles crash tracker type (through ENV vars): SENTRY",
			envValue:   "SENTRY",
			wantResult: crashtracker.CrashTrackerTypeSentry,
		},
		{
			name:       "🎉 handles crash tracker type (through CLI args): DRY_RUN",
			args:       []string{"--crash-tracker-type", "DRY_RUN"},
			wantResult: crashtracker.CrashTrackerTypeDryRun,
		},
		{
			name:       "🎉 handles crash tracker type (through ENV vars): DRY_RUN",
			envValue:   "DRY_RUN",
			wantResult: crashtracker.CrashTrackerTypeDryRun,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			opts.crashTrackerType = ""
			customSetterTester[crashtracker.CrashTrackerType](t, tc, co)
		})
	}
}

func Test_SetConfigOptionEC256PublicKey(t *testing.T) {
	opts := struct{ ec256PublicKey string }{}

	co := config.ConfigOption{
		Name:           "ec256-public-key",
		OptType:        types.String,
		CustomSetValue: SetConfigOptionEC256PublicKey,
		ConfigKey:      &opts.ec256PublicKey,
	}

	expectedPublicKey := `-----BEGIN PUBLIC KEY-----
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAER88h7AiQyVDysRTxKvBB6CaiO/kS
cvGyimApUE/12gFhNTRf37SE19CSCllKxstnVFOpLLWB7Qu5OJ0Wvcz3hg==
-----END PUBLIC KEY-----`

	testCases := []customSetterTestCase[string]{
		{
			name:            "returns an error if the value is not a PEM string",
			args:            []string{"--ec256-public-key", "not-a-pem-string"},
			wantErrContains: "parsing EC256PublicKey: failed to decode PEM block containing public key",
		},
		{
			name:            "returns an error if the value is not a x509 string",
			args:            []string{"--ec256-public-key", "-----BEGIN MY STRING-----\nYWJjZA==\n-----END MY STRING-----"},
			wantErrContains: "parsing EC256PublicKey: failed to parse EC public key",
		},
		{
			name:            "returns an error if the value is not a ECDSA public key",
			args:            []string{"--ec256-public-key", "-----BEGIN PUBLIC KEY-----\nMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAyNPqmozv8a2PnXHIkV+F\nmWMFy2YhOFzX12yzjjWkJ3rI9QSEomz4Unkwc6oYrnKEDYlnAgCiCqL2zPr5qNkX\nk5MPU87/wLgEqp7uAk0GkJZfrhJIYZ5AuG9+o69BNeQDEi7F3YdMJj9bvs2Ou1FN\n1zG/8HV969rJ/63fzWsqlNon1j4H5mJ0YbmVh/QLcYPmv7feFZGEj4OSZ4u+eJsw\nat5NPyhMgo6uB/goNS3fEY29UNvXoSIN3hnK3WSxQ79Rjn4V4so7ehxzCVPjnm/G\nFFTgY0hGBobmnxbjI08hEZmYKosjan4YqydGETjKR3UlhBx9y/eqqgL+opNJ8vJs\n2QIDAQAB\n-----END PUBLIC KEY-----"},
			wantErrContains: "parsing EC256PublicKey: not a valid elliptic curve public key",
		},
		{
			name:       "🎉 handles EC256 public key through the CLI flag",
			args:       []string{"--ec256-public-key", expectedPublicKey},
			wantResult: expectedPublicKey,
		},
		{
			name:       "🎉 handles EC256 public key through the ENV vars",
			envValue:   expectedPublicKey,
			wantResult: expectedPublicKey,
		},
		{
			name:       "🎉 handles EC256 public key through the ENV vars & inline line-breaks",
			envValue:   "-----BEGIN PUBLIC KEY-----\nMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAER88h7AiQyVDysRTxKvBB6CaiO/kS\ncvGyimApUE/12gFhNTRf37SE19CSCllKxstnVFOpLLWB7Qu5OJ0Wvcz3hg==\n-----END PUBLIC KEY-----",
			wantResult: expectedPublicKey,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			opts.ec256PublicKey = ""
			customSetterTester[string](t, tc, co)
		})
	}
}

func Test_SetConfigOptionEC256PrivateKey(t *testing.T) {
	opts := struct{ ec256PrivateKey string }{}

	co := config.ConfigOption{
		Name:           "ec256-private-key",
		OptType:        types.String,
		CustomSetValue: SetConfigOptionEC256PrivateKey,
		ConfigKey:      &opts.ec256PrivateKey,
	}

	expectedPrivateKey := `-----BEGIN PRIVATE KEY-----
MIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQgIqI1MzMZIw2pQDLx
Jn0+FcNT/hNjwtn2TW43710JKZqhRANCAARHzyHsCJDJUPKxFPEq8EHoJqI7+RJy
8bKKYClQT/XaAWE1NF/ftITX0JIKWUrGy2dUU6kstYHtC7k4nRa9zPeG
-----END PRIVATE KEY-----`

	testCases := []customSetterTestCase[string]{
		{
			name:            "returns an error if the value is not a PEM string",
			args:            []string{"--ec256-private-key", "not-a-pem-string"},
			wantErrContains: "parsing EC256PrivateKey: failed to decode PEM block containing private key",
		},
		{
			name:            "returns an error if the value is not a x509 string",
			args:            []string{"--ec256-private-key", "-----BEGIN MY STRING-----\nYWJjZA==\n-----END MY STRING-----"},
			wantErrContains: "parsing EC256PrivateKey: failed to parse EC private key",
		},
		{
			name:            "returns an error if the value is not a ECDSA private key",
			args:            []string{"--ec256-private-key", "-----BEGIN PUBLIC KEY-----\nMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAyNPqmozv8a2PnXHIkV+F\nmWMFy2YhOFzX12yzjjWkJ3rI9QSEomz4Unkwc6oYrnKEDYlnAgCiCqL2zPr5qNkX\nk5MPU87/wLgEqp7uAk0GkJZfrhJIYZ5AuG9+o69BNeQDEi7F3YdMJj9bvs2Ou1FN\n1zG/8HV969rJ/63fzWsqlNon1j4H5mJ0YbmVh/QLcYPmv7feFZGEj4OSZ4u+eJsw\nat5NPyhMgo6uB/goNS3fEY29UNvXoSIN3hnK3WSxQ79Rjn4V4so7ehxzCVPjnm/G\nFFTgY0hGBobmnxbjI08hEZmYKosjan4YqydGETjKR3UlhBx9y/eqqgL+opNJ8vJs\n2QIDAQAB\n-----END PUBLIC KEY-----"},
			wantErrContains: "parsing EC256PrivateKey: failed to parse EC private key",
		},
		{
			name:       "🎉 handles EC256 private key through the CLI flag",
			args:       []string{"--ec256-private-key", expectedPrivateKey},
			wantResult: expectedPrivateKey,
		},
		{
			name:       "🎉 handles EC256 private key through the ENV vars",
			envValue:   expectedPrivateKey,
			wantResult: expectedPrivateKey,
		},
		{
			name:       "🎉 handles EC256 private key through the ENV vars & inline line-breaks",
			envValue:   `-----BEGIN PRIVATE KEY-----\nMIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQgIqI1MzMZIw2pQDLx\nJn0+FcNT/hNjwtn2TW43710JKZqhRANCAARHzyHsCJDJUPKxFPEq8EHoJqI7+RJy\n8bKKYClQT/XaAWE1NF/ftITX0JIKWUrGy2dUU6kstYHtC7k4nRa9zPeG\n-----END PRIVATE KEY-----`,
			wantResult: expectedPrivateKey,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			opts.ec256PrivateKey = ""
			customSetterTester[string](t, tc, co)
		})
	}
}

func Test_SetConfigOptionStellarPublicKey(t *testing.T) {
	opts := struct{ sep10SigningPublicKey string }{}

	co := config.ConfigOption{
		Name:           "sep10-signing-public-key",
		OptType:        types.String,
		CustomSetValue: SetConfigOptionStellarPublicKey,
		ConfigKey:      &opts.sep10SigningPublicKey,
	}
	expectedPublicKey := "GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S"

	testCases := []customSetterTestCase[string]{
		{
			name:            "returns an error if the public key is empty",
			wantErrContains: "error validating public key: strkey is 0 bytes long; minimum valid length is 5",
		},
		{
			name:            "returns an error if the public key is invalid",
			args:            []string{"--sep10-signing-public-key", "invalid_public_key"},
			wantErrContains: "error validating public key: base32 decode failed: illegal base32 data at input byte 18",
		},
		{
			name:            "returns an error if the public key is invalid (private key instead)",
			args:            []string{"--sep10-signing-public-key", "SDISQRUPIHAO5WIIGY4QRDCINZSA44TX3OIIUK3C63NUKN5DABKEQ276"},
			wantErrContains: "error validating public key: invalid version byte",
		},
		{
			name:       "🎉 handles Stellar public key through the CLI flag",
			args:       []string{"--sep10-signing-public-key", "GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S"},
			wantResult: expectedPublicKey,
		},
		{
			name:       "🎉 handles Stellar public key through the ENV vars",
			envValue:   "GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S",
			wantResult: expectedPublicKey,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			opts.sep10SigningPublicKey = ""
			customSetterTester[string](t, tc, co)
		})
	}
}

func Test_SetConfigOptionStellarPrivateKey(t *testing.T) {
	opts := struct{ sep10SigningPrivateKey string }{}

	co := config.ConfigOption{
		Name:           "sep10-signing-private-key",
		OptType:        types.String,
		CustomSetValue: SetConfigOptionStellarPrivateKey,
		ConfigKey:      &opts.sep10SigningPrivateKey,
	}
	expectedPrivateKey := "SBUSPEKAZKLZSWHRSJ2HWDZUK6I3IVDUWA7JJZSGBLZ2WZIUJI7FPNB5"

	testCases := []customSetterTestCase[string]{
		{
			name:            "returns an error if the private key is empty",
			wantErrContains: `error validating private key: ""`,
		},
		{
			name:            "returns an error if the private key is invalid",
			args:            []string{"--sep10-signing-private-key", "invalid_private_key"},
			wantErrContains: `error validating private key: "in...ey"`,
		},
		{
			name:            "returns an error if the private key is invalid (public key instead)",
			args:            []string{"--sep10-signing-private-key", "GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S"},
			wantErrContains: `error validating private key: "GA...7S"`,
		},
		{
			name:       "🎉 handles Stellar private key through the CLI flag",
			args:       []string{"--sep10-signing-private-key", "SBUSPEKAZKLZSWHRSJ2HWDZUK6I3IVDUWA7JJZSGBLZ2WZIUJI7FPNB5"},
			wantResult: expectedPrivateKey,
		},
		{
			name:       "🎉 handles Stellar private key through the ENV flag",
			envValue:   "SBUSPEKAZKLZSWHRSJ2HWDZUK6I3IVDUWA7JJZSGBLZ2WZIUJI7FPNB5",
			wantResult: expectedPrivateKey,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			opts.sep10SigningPrivateKey = ""
			customSetterTester[string](t, tc, co)
		})
	}
}

func Test_SetCorsAllowedOriginsFunc(t *testing.T) {
	opts := struct{ corsAddressesFlag []string }{}

	co := config.ConfigOption{
		Name:           "cors-allowed-origins",
		OptType:        types.String,
		CustomSetValue: SetCorsAllowedOrigins,
		ConfigKey:      &opts.corsAddressesFlag,
		Required:       false,
	}

	testCases := []customSetterTestCase[[]string]{
		{
			name:            "returns an error if the cors flag is empty",
			args:            []string{"--cors-allowed-origins", ""},
			wantErrContains: "cors allowed addresses cannot be empty",
		},
		{
			name:            "returns an error if the cors flag results in an empty array",
			args:            []string{"--cors-allowed-origins", ","},
			wantErrContains: `error parsing cors addresses: parse ""`,
		},
		{
			name:       "🎉 handles one url successfully (from CLI args)",
			args:       []string{"--cors-allowed-origins", "https://foo.test/*"},
			wantResult: []string{"https://foo.test/*"},
		},
		{
			name:       "🎉 handles two urls successfully (from CLI args)",
			args:       []string{"--cors-allowed-origins", "https://foo.test/*,https://bar.test/*"},
			wantResult: []string{"https://foo.test/*", "https://bar.test/*"},
		},
		{
			name:       "🎉 handles one url successfully (from ENV vars)",
			envValue:   "https://foo.test/*",
			wantResult: []string{"https://foo.test/*"},
		},
		{
			name:       "🎉 handles two urls successfully (from ENV vars)",
			envValue:   "https://foo.test/*,https://bar.test/*",
			wantResult: []string{"https://foo.test/*", "https://bar.test/*"},
		},
		{
			name:       `logs a warning when the "*" value is used`,
			envValue:   "*",
			wantResult: []string{"*"},
		},
	}

	getEntries := log.DefaultLogger.StartTest(log.WarnLevel)
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			opts.corsAddressesFlag = nil
			customSetterTester[[]string](t, tc, co)
		})
	}

	entries := getEntries()
	require.Len(t, entries, 1)
	assert.Equal(t, `The value "*" for the CORS Allowed Origins is too permissive and not recommended.`, entries[0].Message)
}

func Test_SetConfigOptionURLString(t *testing.T) {
	opts := struct{ uiBaseURL string }{}

	co := config.ConfigOption{
		Name:           "sdp-ui-base-url",
		OptType:        types.String,
		CustomSetValue: SetConfigOptionURLString,
		ConfigKey:      &opts.uiBaseURL,
		FlagDefault:    "http://localhost:3000",
		Required:       false,
	}

	testCases := []customSetterTestCase[string]{
		{
			name:            "returns an error if the ui base url flag is empty",
			args:            []string{"--sdp-ui-base-url", ""},
			wantErrContains: "ui base url cannot be empty",
		},
		{
			name:       "🎉 handles ui base url successfully (from CLI args)",
			args:       []string{"--sdp-ui-base-url", "https://sdp-ui.org"},
			wantResult: "https://sdp-ui.org",
		},
		{
			name:       "🎉 handles ui base url successfully (from ENV vars)",
			envValue:   "https://sdp-ui.org",
			wantResult: "https://sdp-ui.org",
		},
		{
			name:       "🎉 handles ui base url DEFAULT value",
			wantResult: "http://localhost:3000",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			opts.uiBaseURL = ""
			customSetterTester[string](t, tc, co)
		})
	}
}
