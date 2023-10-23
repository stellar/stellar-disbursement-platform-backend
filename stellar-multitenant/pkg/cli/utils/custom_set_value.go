package utils

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/viper"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/support/config"
	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

func SetConfigOptionEmailSenderType(co *config.ConfigOption) error {
	senderType := viper.GetString(co.Name)
	if senderType == "" {
		return nil
	}

	esType, err := tenant.ParseEmailSenderType(senderType)
	if err != nil {
		return fmt.Errorf("couldn't parse messenger type: %w", err)
	}

	*(co.ConfigKey.(**tenant.EmailSenderType)) = &esType
	return nil
}

func SetConfigOptionSMSSenderType(co *config.ConfigOption) error {
	senderType := viper.GetString(co.Name)
	if senderType == "" {
		return nil
	}

	smsSenderType, err := tenant.ParseSMSSenderType(senderType)
	if err != nil {
		return fmt.Errorf("couldn't parse messenger type: %w", err)
	}

	*(co.ConfigKey.(**tenant.SMSSenderType)) = &smsSenderType
	return nil
}

func SetConfigOptionStellarPublicKey(co *config.ConfigOption) error {
	publicKey := viper.GetString(co.Name)
	if publicKey == "" {
		return nil
	}

	kp, err := keypair.ParseAddress(publicKey)
	if err != nil {
		return fmt.Errorf("error validating public key: %w", err)
	}

	key, ok := co.ConfigKey.(**string)
	if !ok {
		return fmt.Errorf("the expected type for this config key is a string, but got a %T instead", co.ConfigKey)
	}
	addr := kp.Address()
	*key = &addr

	return nil
}

func SetCORSAllowedOrigins(co *config.ConfigOption) error {
	corsAllowedOriginsOptions := viper.GetString(co.Name)
	if corsAllowedOriginsOptions == "" {

		return nil
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

func SetConfigOptionURLString(co *config.ConfigOption) error {
	u := viper.GetString(co.Name)
	if u == "" {
		return nil
	}

	_, err := url.ParseRequestURI(u)
	if err != nil {
		return fmt.Errorf("error parsing ui base url: %w", err)
	}

	key, ok := co.ConfigKey.(**string)
	if !ok {
		return fmt.Errorf("the expected type for this config key is a string, but got a %T instead", co.ConfigKey)
	}
	*key = &u

	return nil
}

func SetConfigOptionOptionalBoolean(co *config.ConfigOption) error {
	b := viper.GetString(co.Name)
	if b == "" {
		return nil
	}

	value, err := strconv.ParseBool(b)
	if err != nil {
		return fmt.Errorf("parsing %q as a boolean value: %w", b, err)
	}

	key, ok := co.ConfigKey.(**bool)
	if !ok {
		return fmt.Errorf("the expected type for this config key is a boolean, but got a %T instead", co.ConfigKey)
	}
	*key = &value
	return nil
}
