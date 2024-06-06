package utils

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/strkey"
	"github.com/stellar/go/support/config"
	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

func SetConfigOptionMessengerType(co *config.ConfigOption) error {
	senderType := viper.GetString(co.Name)

	messengerType, err := message.ParseMessengerType(senderType)
	if err != nil {
		return fmt.Errorf("couldn't parse messenger type in %s: %w", co.Name, err)
	}

	*(co.ConfigKey.(*message.MessengerType)) = messengerType
	return nil
}

func SetConfigOptionMetricType(co *config.ConfigOption) error {
	metricType := viper.GetString(co.Name)

	metricTypeParsed, err := monitor.ParseMetricType(metricType)
	if err != nil {
		return fmt.Errorf("couldn't parse metric type in %s: %w", co.Name, err)
	}

	*(co.ConfigKey.(*monitor.MetricType)) = metricTypeParsed
	return nil
}

func SetConfigOptionCrashTrackerType(co *config.ConfigOption) error {
	ctType := viper.GetString(co.Name)

	ctTypeParsed, err := crashtracker.ParseCrashTrackerType(ctType)
	if err != nil {
		return fmt.Errorf("couldn't parse crash tracker type in %s: %w", co.Name, err)
	}

	*(co.ConfigKey.(*crashtracker.CrashTrackerType)) = ctTypeParsed
	return nil
}

func SetConfigOptionDistributionSignerType(co *config.ConfigOption) error {
	ssType := viper.GetString(co.Name)

	ssTypeParsed, err := signing.ParseSignatureClientDistributionType(ssType)
	if err != nil {
		return fmt.Errorf("couldn't parse signature client distribution type in %s: %w", co.Name, err)
	}

	*(co.ConfigKey.(*signing.SignatureClientType)) = ssTypeParsed
	return nil
}

func SetConfigOptionLogLevel(co *config.ConfigOption) error {
	// parse string to logLevel object
	logLevelStr := viper.GetString(co.Name)
	logLevel, err := logrus.ParseLevel(logLevelStr)
	if err != nil {
		return fmt.Errorf("couldn't parse log level in %s: %w", co.Name, err)
	}

	// update the configKey
	key, ok := co.ConfigKey.(*logrus.Level)
	if !ok {
		return fmt.Errorf("%s configKey has an invalid type %T", co.Name, co.ConfigKey)
	}
	*key = logLevel

	// Log for debugging
	if config.IsExplicitlySet(co) {
		log.Debugf("Setting log level to: %q", logLevel)
		log.DefaultLogger.SetLevel(*key)
	} else {
		log.Debugf("Using default log level: %q", logLevel)
	}
	return nil
}

// SetConfigOptionEC256PublicKey parses the config option incoming value and validates if it is a valid EC256PublicKey.
func SetConfigOptionEC256PublicKey(co *config.ConfigOption) error {
	key, ok := co.ConfigKey.(*string)
	if !ok {
		return fmt.Errorf("not a valid EC256PublicKey in %s: the expected type for this config key is a string, but got a %T instead", co.Name, co.ConfigKey)
	}

	publicKey := viper.GetString(co.Name)

	// We must remove the literal \n in case of the config options being set this way
	publicKey = strings.Replace(publicKey, `\n`, "\n", -1)

	_, err := utils.ParseStrongECPublicKey(publicKey)
	if err != nil {
		return fmt.Errorf("parsing EC256PublicKey in %s: %w", co.Name, err)
	}

	*key = publicKey
	return nil
}

// SetConfigOptionEC256PrivateKey parses the config option incoming value and validates if it is a valid EC256PrivateKey.
func SetConfigOptionEC256PrivateKey(co *config.ConfigOption) error {
	key, ok := co.ConfigKey.(*string)
	if !ok {
		return fmt.Errorf("not a valid EC256PrivateKey in %s: the expected type for this config key is a string, but got a %T instead", co.Name, co.ConfigKey)
	}

	privateKey := viper.GetString(co.Name)

	// We must remove the literal \n in case of the config options being set this way
	privateKey = strings.Replace(privateKey, `\n`, "\n", -1)

	_, err := utils.ParseStrongECPrivateKey(privateKey)
	if err != nil {
		return fmt.Errorf("parsing EC256PrivateKey in %s: %w", co.Name, err)
	}

	*key = privateKey
	return nil
}

func SetCorsAllowedOrigins(co *config.ConfigOption) error {
	corsAllowedOriginsOptions := viper.GetString(co.Name)

	if corsAllowedOriginsOptions == "" {
		return fmt.Errorf("cors allowed addresses cannot be empty in %s", co.Name)
	}

	corsAllowedOrigins := strings.Split(corsAllowedOriginsOptions, ",")

	// validate addresses
	for _, address := range corsAllowedOrigins {
		_, err := url.ParseRequestURI(address)
		if err != nil {
			return fmt.Errorf("error parsing cors addresses in %s: %w", co.Name, err)
		}
		if address == "*" {
			log.Warn(`The value "*" for the CORS Allowed Origins is too permissive and not recommended.`)
		}
	}

	key, ok := co.ConfigKey.(*[]string)
	if !ok {
		return fmt.Errorf("the expected type for the config key in %s is a string slice, but a %T was provided instead", co.Name, co.ConfigKey)
	}
	*key = corsAllowedOrigins

	return nil
}

func SetConfigOptionStellarPublicKey(co *config.ConfigOption) error {
	publicKey := viper.GetString(co.Name)

	kp, err := keypair.ParseAddress(publicKey)
	if err != nil {
		return fmt.Errorf("error validating public key in %s: %w", co.Name, err)
	}

	key, ok := co.ConfigKey.(*string)
	if !ok {
		return fmt.Errorf("the expected type for the config key in %s is a string, but a %T was provided instead", co.Name, co.ConfigKey)
	}
	*key = kp.Address()

	return nil
}

func SetConfigOptionStellarPrivateKey(co *config.ConfigOption) error {
	privateKey := viper.GetString(co.Name)
	if privateKey == "" {
		return nil
	}

	isValid := strkey.IsValidEd25519SecretSeed(privateKey)
	if !isValid {
		return fmt.Errorf("error validating private key in %s: %q", co.Name, utils.TruncateString(privateKey, 2))
	}

	key, ok := co.ConfigKey.(*string)
	if !ok {
		return fmt.Errorf("the expected type for the config key in %s is a string, but a %T was provided instead", co.Name, co.ConfigKey)
	}
	*key = privateKey

	return nil
}

func SetConfigOptionURLString(co *config.ConfigOption) error {
	u := viper.GetString(co.Name)

	if u == "" {
		return fmt.Errorf("URL cannot be empty in %s", co.Name)
	}

	_, err := url.ParseRequestURI(u)
	if err != nil {
		return fmt.Errorf("parsing URL in %s: %w", co.Name, err)
	}

	key, ok := co.ConfigKey.(*string)
	if !ok {
		return fmt.Errorf("the expected type for the config key in %s is a string, but a %T was provided instead", co.Name, co.ConfigKey)
	}
	*key = u

	return nil
}

func SetConfigOptionURLList(co *config.ConfigOption) error {
	urlsStr := viper.GetString(co.Name)
	urlsStr = strings.TrimSpace(urlsStr)

	key, ok := co.ConfigKey.(*[]string)
	if !ok {
		return fmt.Errorf("the expected type for the config key in %s is a string slice, but a %T was provided instead", co.Name, co.ConfigKey)
	}

	if urlsStr == "" {
		if co.Required {
			return fmt.Errorf("URL list cannot be empty in %s", co.Name)
		}
		*key = make([]string, 0)
		return nil
	}

	urls := strings.Split(urlsStr, ",")
	for _, u := range urls {
		_, err := url.ParseRequestURI(strings.TrimSpace(u))
		if err != nil {
			return fmt.Errorf("error parsing URL in %s: %w", co.Name, err)
		}
	}

	*key = urls

	return nil
}

func SetConfigOptionStringList(co *config.ConfigOption) error {
	listStr := viper.GetString(co.Name)

	if listStr == "" {
		return fmt.Errorf("string list cannot be empty in %s", co.Name)
	}

	list := strings.Split(listStr, ",")
	for i, el := range list {
		list[i] = strings.TrimSpace(el)
	}

	key, ok := co.ConfigKey.(*[]string)
	if !ok {
		return fmt.Errorf("the expected type for the config key in %s is a string slice, but a %T was provided instead", co.Name, co.ConfigKey)
	}

	*key = list

	return nil
}

func SetConfigOptionEventBrokerType(co *config.ConfigOption) error {
	ebType := viper.GetString(co.Name)

	ebTypeParsed, err := events.ParseEventBrokerType(ebType)
	if err != nil {
		return fmt.Errorf("couldn't parse event broker type in %s: %w", co.Name, err)
	}

	*(co.ConfigKey.(*events.EventBrokerType)) = ebTypeParsed
	return nil
}

func SetConfigOptionKafkaSecurityProtocol(co *config.ConfigOption) error {
	protocol := viper.GetString(co.Name)
	if protocol == "" {
		return nil
	}

	protocolParsed, err := events.ParseKafkaSecurityProtocol(protocol)
	if err != nil {
		return fmt.Errorf("couldn't parse kafka security protocol: %w", err)
	}

	*(co.ConfigKey.(*events.KafkaSecurityProtocol)) = protocolParsed
	return nil
}
