package config

import (
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig       `mapstructure:"server"`
	Redis    RedisConfig        `mapstructure:"redis"`
	Services map[string]Service `mapstructure:"services"`
	Security SecurityConfig     `mapstructure:"security"`
	Cors     CorsConfig         `mapstructure:"cors"`
}

type ServerConfig struct {
	Port         string        `mapstructure:"port"`
	Environment  string        `mapstructure:"environment"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
}

type RedisConfig struct {
	Address  string `mapstructure:"address"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type Service struct {
	Name           string        `mapstructure:"name"`
	URL            string        `mapstructure:"url"`
	Timeout        time.Duration `mapstructure:"timeout"`
	CircuitBreaker bool          `mapstructure:"circuit_breaker"`
}

type SecurityConfig struct {
	JWTSecret       string        `mapstructure:"jwt_secret"`
	TokenExpiration time.Duration `mapstructure:"token_expiration"`
}

type CorsConfig struct {
	AllowOrigins []string `mapstructure:"allow_origins"`
}

func Load() (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("./config")
	viper.AddConfigPath("../config")

	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	viper.SetDefault("server.port", "8080")
	viper.SetDefault("server.read_timeout", 10*time.Second)
	viper.SetDefault("server.write_timeout", 10*time.Second)
	viper.SetDefault("security.token_expiration", 1*time.Hour)

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, err
		}
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
