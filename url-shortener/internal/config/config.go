package config

import (
	"time"

	"github.com/spf13/viper"
)

type App struct {
	LogLevel          string        `mapstructure:"log_level"`
	Port              int           `mapstructure:"port"`
	ReadHeaderTimeout time.Duration `mapstructure:"read_header_timeout"`
	ReadTimeout       time.Duration `mapstructure:"read_timeout"`
	WriteTimeout      time.Duration `mapstructure:"write_timeout"`
	IdleTimeout       time.Duration `mapstructure:"idle_timeout"`
	MaxInFlight       int           `mapstructure:"max_in_flight"`
	// Where pprof listens, apart from the API. Empty turns it off.
	PprofAddr string `mapstructure:"pprof_addr"`
	// HTTPS, on a port of its own next to the plain one, which the gateway
	// and the metrics scrape keep using. Off unless both files are set (from
	// TLS_CERT_FILE and TLS_KEY_FILE).
	TLS struct {
		Port     int    `mapstructure:"port"`
		CertFile string `mapstructure:"cert_file"`
		KeyFile  string `mapstructure:"key_file"`
	} `mapstructure:"tls"`
}

type Postgres struct {
	Uri string `mapstructure:"uri"`

	Timeouts struct {
		Acquire time.Duration `mapstructure:"acquire"`
		Query   time.Duration `mapstructure:"query"`
	} `mapstructure:"timeouts"`

	Pool struct {
		MinConns int `mapstructure:"min_conns"`
		MaxConns int `mapstructure:"max_conns"`
	} `mapstructure:"pool"`
}

type Redis struct {
	Uri string `mapstructure:"uri"`

	Timeouts struct {
		Query time.Duration `mapstructure:"query"`
	} `mapstructure:"timeouts"`

	Pool struct {
		Size int `mapstructure:"size"`
	} `mapstructure:"pool"`
}

type Config struct {
	App      App      `mapstructure:"app"`
	Postgres Postgres `mapstructure:"postgres"`
	Redis    Redis    `mapstructure:"redis"`
}

// Load reads config/<env>.yaml. An unknown env has no file, and fails here.
func Load(env string) (*Config, error) {
	v := viper.New()
	v.SetConfigName(env)
	v.SetConfigType("yaml")
	v.AddConfigPath("config")

	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}
	if err := v.BindEnv("postgres.uri", "POSTGRES_URI"); err != nil {
		return nil, err
	}
	if err := v.BindEnv("redis.uri", "REDIS_URI"); err != nil {
		return nil, err
	}
	if err := v.BindEnv("app.tls.cert_file", "TLS_CERT_FILE"); err != nil {
		return nil, err
	}
	if err := v.BindEnv("app.tls.key_file", "TLS_KEY_FILE"); err != nil {
		return nil, err
	}

	var c Config
	if err := v.Unmarshal(&c); err != nil {
		return nil, err
	}
	return &c, nil
}
