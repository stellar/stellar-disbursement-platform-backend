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
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

func SetConfigOptionMessengerType(co *config.ConfigOption) error {
	senderType := viper.GetString(co.Name)

	messengerType, err := message.ParseMessengerType(senderType)
	if err != nil {
		return fmt.Errorf("couldn't parse messenger type: %w", err)
	}

	*(co.ConfigKey.(*message.MessengerType)) = messengerType
	return nil
}

func SetConfigOptionMetricType(co *config.ConfigOption) error {
	metricType := viper.GetString(co.Name)

	metricTypeParsed, err := monitor.ParseMetricType(metricType)
	if err != nil {
		return fmt.Errorf("couldn't parse metric type: %w", err)
	}

	*(co.ConfigKey.(*monitor.MetricType)) = metricTypeParsed
	return nil
}

func SetConfigOptionCrashTrackerType(co *config.ConfigOption) error {
	ctType := viper.GetString(co.Name)

	ctTypeParsed, err := crashtracker.ParseCrashTrackerType(ctType)
	if err != nil {
		return fmt.Errorf("couldn't parse crash tracker type: %w", err)
	}

	*(co.ConfigKey.(*crashtracker.CrashTrackerType)) = ctTypeParsed
	return nil
}

func SetConfigOptionLogLevel(co *config.ConfigOption) error {
	// parse string to logLevel object
	logLevelStr := viper.GetString(co.Name)
	logLevel, err := logrus.ParseLevel(logLevelStr)
	if err != nil {
		return fmt.Errorf("couldn't parse log level: %w", err)
	}

	// update the configKey
	key, ok := co.ConfigKey.(*logrus.Level)
	if !ok {
		return fmt.Errorf("configKey has an invalid type %T", co.ConfigKey)
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
		return fmt.Errorf("not a valid EC256PublicKey: the expected type for this config key is a string, but got a %T instead", co.ConfigKey)
	}

	publicKey := viper.GetString(co.Name)

	// We must remove the literal \n in case of the config options being set this way
	publicKey = strings.Replace(publicKey, `\n`, "\n", -1)

	_, err := utils.ParseStrongECPublicKey(publicKey)
	if err != nil {
		return fmt.Errorf("parsing EC256PublicKey: %w", err)
	}

	*key = publicKey
	return nil
}

// SetConfigOptionEC256PrivateKey parses the config option incoming value and validates if it is a valid EC256PrivateKey.
func SetConfigOptionEC256PrivateKey(co *config.ConfigOption) error {
	key, ok := co.ConfigKey.(*string)
	if !ok {
		return fmt.Errorf("not a valid EC256PrivateKey: the expected type for this config key is a string, but got a %T instead", co.ConfigKey)
	}

	privateKey := viper.GetString(co.Name)

	// We must remove the literal \n in case of the config options being set this way
	privateKey = strings.Replace(privateKey, `\n`, "\n", -1)

	_, err := utils.ParseStrongECPrivateKey(privateKey)
	if err != nil {
		return fmt.Errorf("parsing EC256PrivateKey: %w", err)
	}

	*key = privateKey
	return nil
}

func SetCorsAllowedOrigins(co *config.ConfigOption) error {
	corsAllowedOriginsOptions := viper.GetString(co.Name)

	if corsAllowedOriginsOptions == "" {
		return fmt.Errorf("cors allowed addresses cannot be empty")
	}

	corsAllowedOrigins := strings.Split(corsAllowedOriginsOptions, ",")

	// validate addresses
	for _, address := range corsAllowedOrigins {
		_, err := url.ParseRequestURI(address)
		if err != nil {
			return fmt.Errorf("error parsing cors addresses: %w", err)
		}
		if address == "*" {
			log.Warn(`The value "*" for the CORS Allowed Origins is too permissive and not recommended.`)
		}
	}

	key, ok := co.ConfigKey.(*[]string)
	if !ok {
		return fmt.Errorf("the expected type for this config key is a string slice, but got a %T instead", co.ConfigKey)
	}
	*key = corsAllowedOrigins

	return nil
}

func SetConfigOptionStellarPublicKey(co *config.ConfigOption) error {
	publicKey := viper.GetString(co.Name)

	kp, err := keypair.ParseAddress(publicKey)
	if err != nil {
		return fmt.Errorf("error validating public key: %w", err)
	}

	key, ok := co.ConfigKey.(*string)
	if !ok {
		return fmt.Errorf("the expected type for this config key is a string, but got a %T instead", co.ConfigKey)
	}
	*key = kp.Address()

	return nil
}

func SetConfigOptionStellarPrivateKey(co *config.ConfigOption) error {
	privateKey := viper.GetString(co.Name)

	isValid := strkey.IsValidEd25519SecretSeed(privateKey)
	if !isValid {
		return fmt.Errorf("error validating private key: %q", utils.TruncateString(privateKey, 2))
	}

	key, ok := co.ConfigKey.(*string)
	if !ok {
		return fmt.Errorf("the expected type for this config key is a string, but got a %T instead", co.ConfigKey)
	}
	*key = privateKey

	return nil
}

func SetConfigOptionURLString(co *config.ConfigOption) error {
	u := viper.GetString(co.Name)

	if u == "" {
		return fmt.Errorf("ui base url cannot be empty")
	}

	_, err := url.ParseRequestURI(u)
	if err != nil {
		return fmt.Errorf("error parsing ui base url: %w", err)
	}

	key, ok := co.ConfigKey.(*string)
	if !ok {
		return fmt.Errorf("the expected type for this config key is a string, but got a %T instead", co.ConfigKey)
	}
	*key = u

	return nil
}
