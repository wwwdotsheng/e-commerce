package config

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

const (
	EnvDev  = "DEV"
	EnvTest = "TEST"
	EnvProd = "PROD"
)

type AppConfig struct {
	App        AppSection        `mapstructure:"app"`
	Database   DatabaseSection   `mapstructure:"database"`
	Redis      RedisSection      `mapstructure:"redis"`
	Log        LogSection        `mapstructure:"log"`
	Auth       AuthSection       `mapstructure:"auth"`
	Otel       OtelSection       `mapstructure:"otel"`
	TestImages TestImagesSection `mapstructure:"test_images"`
}

type AppSection struct {
	Name       string `mapstructure:"name"`
	Port       int    `mapstructure:"port"`
	Env        string `mapstructure:"env"`
	SSL        bool   `mapstructure:"ssl"`
	SSLCrtPath string `mapstructure:"ssl_crt_path"`
	SSLKeyPath string `mapstructure:"ssl_key_path"`
}

type DatabaseSection struct {
	Host            string `mapstructure:"host"`
	Port            int    `mapstructure:"port"`
	User            string `mapstructure:"user"`
	Password        string `mapstructure:"password"`
	DBName          string `mapstructure:"db_name"`
	SSLMode         string `mapstructure:"ssl_mode"`
	MaxIdleConns    int    `mapstructure:"max_idle_conns"`
	MaxOpenConns    int    `mapstructure:"max_open_conns"`
	ConnMaxLifetime int    `mapstructure:"conn_max_lifetime"`
	TimeZone        string `mapstructure:"time_zone"`
	LogLevel        string `mapstructure:"log_level"`
}

type RedisSection struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
	PoolSize int    `mapstructure:"pool_size"`
}

type LogSection struct {
	ConsoleLevel string `mapstructure:"console_level"`
	FileLevel    string `mapstructure:"file_level"`
	Filename     string `mapstructure:"filename"`
	MaxSize      int    `mapstructure:"max_size"`
	MaxBackups   int    `mapstructure:"max_backups"`
	MaxAge       int    `mapstructure:"max_age"`
}

type OtelSection struct {
	Enabled  bool `mapstructure:"enabled"`
	Endpoint struct {
		Trace  string `mapstructure:"trace_endpoint"`
		Metric string `mapstructure:"metric_endpoint"`
	} `mapstructure:"endpoint"`
	ServiceName string `mapstructure:"service_name"`
}

type AuthSection struct {
	AccessTokenExpire  time.Duration `mapstructure:"access_token_expire"`
	RefreshTokenExpire time.Duration `mapstructure:"refresh_token_expire"`
	TokenSecret        string        `mapstructure:"token_secret"`
}

type TestImagesSection struct {
	Postgres string `mapstructure:"postgres"`
	Redis    string `mapstructure:"redis"`
}

func Init() (*AppConfig, error) {
	var cfg AppConfig
	_ = godotenv.Load()

	v := viper.New()

	v.SetDefault("APP_ENV", EnvProd)

	v.SetEnvPrefix("APP")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	cfgPath := os.Getenv("CONFIG_PATH")
	if cfgPath == "" {
		cfgPath = "configs/config.yaml"
	}

	v.SetConfigFile(cfgPath)

	if err := v.ReadInConfig(); err != nil {
		log.Printf("Warning: using ENV only, config file not found: %v", err)
	}

	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("config unmarshal failed: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &cfg, nil
}

func (c *AppConfig) Validate() error {
	switch strings.ToUpper(c.App.Env) {
	case EnvDev, EnvTest, EnvProd:
	case "":
		return errors.New("app.env 不能为空")
	default:
		return fmt.Errorf("无效的环境配置: %s, 必须是 DEV/TEST/PROD 之一", c.App.Env)
	}

	if c.Database.Host == "" {
		return errors.New("database.host required (ENV: APP_DATABASE_HOST)")
	}
	if c.Database.User == "" {
		return errors.New("database.user required")
	}
	return nil
}

// ===== 环境判断 =====

func (c *AppConfig) IsDev() bool  { return strings.ToUpper(c.App.Env) == EnvDev }
func (c *AppConfig) IsTest() bool { return strings.ToUpper(c.App.Env) == EnvTest }
func (c *AppConfig) IsProd() bool { return strings.ToUpper(c.App.Env) == EnvProd }
