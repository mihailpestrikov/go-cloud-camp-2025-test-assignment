package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server      ServerConfig      `mapstructure:"server"`
	Logging     LoggerConfig      `mapstructure:"logging"`
	Backends    []BackendConfig   `mapstructure:"backends"`
	Balancer    BalancerConfig    `mapstructure:"balancer"`
	HealthCheck HealthCheckConfig `mapstructure:"health_check"`
	RateLimit   RateLimitConfig   `mapstructure:"rate_limit"`
}

type ServerConfig struct {
	Port    int           `mapstructure:"port"`
	Timeout time.Duration `mapstructure:"timeout"`
}

type LoggerConfig struct {
	Level    string `mapstructure:"level"`
	Format   string `mapstructure:"format"`
	Output   string `mapstructure:"output"`
	FilePath string `mapstructure:"file_path"`
}

type BackendConfig struct {
	URL string `mapstructure:"url"`
}

type BalancerConfig struct {
	Algorithm string `mapstructure:"algorithm"`
}

type HealthCheckConfig struct {
	Enabled  bool          `mapstructure:"enabled"`
	Interval time.Duration `mapstructure:"interval"`
	Path     string        `mapstructure:"path"`
}

type RateLimitConfig struct {
	Enabled bool              `mapstructure:"enabled"`
	Redis   RedisConfig       `mapstructure:"redis"`
	Default TokenBucketConfig `mapstructure:"default"`
}

type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type TokenBucketConfig struct {
	Capacity   int `mapstructure:"capacity"`
	RefillRate int `mapstructure:"refill_rate"`
}

func LoadConfig(configPath string) (*Config, error) {
	v := viper.New()

	setDefaults(v)

	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
		v.AddConfigPath("./config")
		v.AddConfigPath("/etc/loadbalancer")
	}

	v.SetEnvPrefix("LB")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if errors.As(err, &configFileNotFoundError) {
			fmt.Fprintf(os.Stderr, "Warning: No config file found\n")
		}
	}

	var config Config
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("unable to decode config into struct: %w", err)
	}

	if err := validateConfig(&config); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &config, nil
}

func setDefaults(v *viper.Viper) {

	v.SetDefault("server.port", 8080)
	v.SetDefault("server.timeout", "10s")

	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "json")
	v.SetDefault("logging.output", "stdout")
	v.SetDefault("logging.file_path", "./logs/balancer.log")

	v.SetDefault("balancer.algorithm", "round_robin")

	v.SetDefault("health_check.enabled", true)
	v.SetDefault("health_check.interval", "5s")
	v.SetDefault("health_check.path", "/health")

	v.SetDefault("rate_limit.enabled", true)
	v.SetDefault("rate_limit.redis.addr", "localhost:6379")
	v.SetDefault("rate_limit.redis.password", "")
	v.SetDefault("rate_limit.redis.db", 0)
	v.SetDefault("rate_limit.default.capacity", 50)
	v.SetDefault("rate_limit.default.refill_rate", 10)
}

func validateConfig(config *Config) error {

	if config.Server.Port <= 0 || config.Server.Port > 65535 {
		return fmt.Errorf("server port must be between 1 and 65535")
	}

	if len(config.Backends) == 0 {
		return fmt.Errorf("at least one backend must be configured")
	}

	validAlgorithms := map[string]bool{
		"round_robin":       true,
		"least_connections": true,
		"random":            true,
	}
	if !validAlgorithms[config.Balancer.Algorithm] {
		return fmt.Errorf("invalid balancer algorithm: %s", config.Balancer.Algorithm)
	}

	validLogLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
	}
	if !validLogLevels[strings.ToLower(config.Logging.Level)] {
		return fmt.Errorf("invalid log level: %s", config.Logging.Level)
	}

	validLogFormats := map[string]bool{
		"json":    true,
		"console": true,
	}
	if !validLogFormats[strings.ToLower(config.Logging.Format)] {
		return fmt.Errorf("invalid log format: %s", config.Logging.Format)
	}

	validLogOutputs := map[string]bool{
		"stdout": true,
		"file":   true,
	}
	if !validLogOutputs[strings.ToLower(config.Logging.Output)] {
		return fmt.Errorf("invalid log output: %s", config.Logging.Output)
	}

	if strings.ToLower(config.Logging.Output) == "file" && config.Logging.FilePath == "" {
		return fmt.Errorf("file_path must be specified when output is set to file")
	}

	return nil
}
