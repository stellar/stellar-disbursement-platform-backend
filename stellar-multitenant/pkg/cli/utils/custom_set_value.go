package utils

import (
	"fmt"
	"net/url"

	"github.com/spf13/viper"
	"github.com/stellar/go/support/config"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
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

func SetConfigOptionNetworkType(co *config.ConfigOption) error {
	networkType := viper.GetString(co.Name)
	value, err := utils.GetNetworkTypeFromString(networkType)
	if err != nil {
		return fmt.Errorf("getting network type from string: %w", err)
	}

	key, ok := co.ConfigKey.(*string)
	if !ok {
		return fmt.Errorf("the expected type for this config key is a string, but got a %T instead", co.ConfigKey)
	}
	*key = string(value)
	return nil
}

func SetConfigOptionMessengerType(co *config.ConfigOption) error {
	senderType := viper.GetString(co.Name)

	messengerType, err := message.ParseMessengerType(senderType)
	if err != nil {
		return fmt.Errorf("couldn't parse messenger type: %w", err)
	}

	*(co.ConfigKey.(*message.MessengerType)) = messengerType
	return nil
}
