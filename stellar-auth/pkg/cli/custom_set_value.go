package cli

import (
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/stellar/go/support/config"
	"github.com/stellar/go/support/log"
)

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

func setConfigOptionRoles(co *config.ConfigOption) error {
	rolesStr := viper.GetString(co.Name)
	rolesSplit := strings.FieldsFunc(rolesStr, func(r rune) bool {
		return r == ','
	})

	roles := make([]string, 0, len(rolesSplit))
	for _, role := range rolesSplit {
		roles = append(roles, strings.TrimSpace(role))
	}

	key, ok := co.ConfigKey.(*[]string)
	if !ok {
		return fmt.Errorf("the expected type for this config key is a string slice, but got a %T instead", co.ConfigKey)
	}
	*key = roles

	return nil
}
