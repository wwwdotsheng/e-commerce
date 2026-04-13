package config

import (
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

var Conf = new(AppConfig)

const (
	EnvDev  = "DEV"
	EnvTest = "TEST"
	EnvProd = "PROD"
)

type AppConfig struct {
	App      AppSection      `mapstructure:"app"`
	Database DatabaseSection `mapstructure:"database"`
	Redis    RedisSection    `mapstructure:"redis"`
	Log      LogSection      `mapstructure:"log"`
	Auth     AuthSection     `mapstructure:"auth"`
	Otel     OtelSection     `mapstructure:"otel"`
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

func Init(configPath string) (*AppConfig, error) {
	_ = godotenv.Load()
	viper.SetConfigFile(configPath)
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		return nil, err
	}
	if err := viper.Unmarshal(Conf); err != nil {
		return nil, err
	}
	return Conf, nil
}

func IsDev() bool {
	return strings.ToUpper(Conf.App.Env) == EnvDev
}

func IsTest() bool {
	return strings.ToUpper(Conf.App.Env) == EnvTest
}

func IsProd() bool {
	return strings.ToUpper(Conf.App.Env) == EnvProd
}
