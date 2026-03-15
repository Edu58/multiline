package config

import (
	"github.com/spf13/viper"
)

type Config struct {
	HOST           string `mapstructure:"host"`
	PORT           string `mapstructure:"port"`
	DSN_URL        string `mapstructure:"dsn_url"`
	DSN_OPTIONS    string `mapstructure:"dsn_options"`
	MIGRATIONS_URL string `mapstructure:"migrations_url"`
}

func LoadConfig(dir string, configName string, configType string) (config Config, err error) {
	viper.SetConfigName(configName)
	viper.AddConfigPath(dir)
	viper.SetConfigType(configType)

	viper.SetDefault("HOST", "localhost")
	viper.SetDefault("PORT", "4000")

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
