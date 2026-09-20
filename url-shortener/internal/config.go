package internal

import (
	"strings"

	"github.com/spf13/viper"
)

type AppConfig struct {
	App struct {
		LogLevel                 string `mapstructure:"log_level"`
		Port                     int    `mapstructure:"port"`
		ReadHeaderTimeoutSeconds int    `mapstructure:"read_header_timeout_seconds"`
		ReadTimeoutSeconds       int    `mapstructure:"read_timeout_seconds"`
		WriteTimeoutSeconds      int    `mapstructure:"write_timeout_seconds"`
		IdleTimeoutSeconds       int    `mapstructure:"idle_timeout_seconds"`
	} `mapstructure:"app"`

	Postgres struct {
		DbName   string `mapstructure:"db_name"`
		Host     string `mapstructure:"host"`
		Port     int    `mapstructure:"port"`
		User     string `mapstructure:"user"`
		Password string `mapstructure:"password"`

		Pool struct {
			MinConns int `mapstructure:"min_conns"`
			MaxConns int `mapstructure:"max_conns"`
		} `mapstructure:"pool"`
	} `mapstructure:"postgres"`

	Redis struct {
		Host     string `mapstructure:"host"`
		Port     int    `mapstructure:"port"`
		User     string `mapstructure:"user"`
		Password string `mapstructure:"password"`
	} `mapstructure:"redis"`
}

type Environment string

const (
	Local Environment = "local"
	Dev   Environment = "dev"
	Prod  Environment = "prod"
)

func EnvironmentFromString(env string) (Environment, bool) {
	environmentMap := map[string]Environment{
		"local": Local,
		"dev":   Dev,
		"prod":  Prod,
	}
	environment, ok := environmentMap[strings.ToLower(env)]
	return environment, ok
}

func NewAppConfig(environment Environment) (*AppConfig, error) {
	viper.SetConfigName(string(environment))
	viper.SetConfigType("yaml")
	viper.AddConfigPath("config")

	if err := viper.ReadInConfig(); err != nil {
		return nil, err
	}
	if err := viper.BindEnv("postgres.user", "POSTGRES_USER"); err != nil {
		return nil, err
	}
	if err := viper.BindEnv("postgres.password", "POSTGRES_PASSWORD"); err != nil {
		return nil, err
	}
	if err := viper.BindEnv("redis.user", "REDIS_USER"); err != nil {
		return nil, err
	}
	if err := viper.BindEnv("redis.password", "REDIS_PASSWORD"); err != nil {
		return nil, err
	}

	var config AppConfig
	err := viper.Unmarshal(&config)
	return &config, err
}
