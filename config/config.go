package config

import (
	"github.com/spf13/viper"
)

type Config struct {
	RABBITMQ_URL   string            `mapstructure:"rabbitmq_url"`
	DSN_URL        string            `mapstructure:"dsn_url"`
	DSN_OPTIONS    string            `mapstructure:"dsn_options"`
	MIGRATIONS_URL string            `mapstructure:"migrations_url"`
	LOG_LEVEL      string            `mapstructure:"log_level"`
	LOG_OUT        string            `mapstructure:"log_out"`
	WORKERS        map[string]string `mapstructure:"workers"`
}

func LoadConfig(dir string, configName string, configType string) (config Config, err error) {
	viper.SetConfigName(configName)
	viper.AddConfigPath(dir)
	viper.SetConfigType(configType)

	viper.AutomaticEnv()

	// Find and read the config file
	err = viper.ReadInConfig()

	// Handle errors
	if err != nil {
		return Config{}, err
	}

	err = viper.Unmarshal(&config)

	// Handle errors
	if err != nil {
		return Config{}, err
	}

	return
}
