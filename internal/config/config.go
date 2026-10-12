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

type AppConfig struct {
	App           AppSection           `mapstructure:"app"`
	Database      DatabaseSection      `mapstructure:"database"`
	Redis         RedisSection         `mapstructure:"redis"`
	RabbitMQ      RabbitMQSection      `mapstructure:"rabbitmq"`
	Registry      RegistrySection      `mapstructure:"registry"`
	Log           LogSection           `mapstructure:"log"`
	Auth          AuthSection          `mapstructure:"auth"`
	Otel          OtelSection          `mapstructure:"otel"`
	TestImages    TestImagesSection    `mapstructure:"test_images"`
	Elasticsearch ElasticsearchSection `mapstructure:"elasticsearch"`
	OrderMQ       OrderMQConfig        `mapstructure:"order_mq"`
}

type AppSection struct {
	Name       string `mapstructure:"name"`
	Port       int    `mapstructure:"port"`
	SSL        bool   `mapstructure:"ssl"`
	SSLCrtPath string `mapstructure:"ssl_crt_path"`
	SSLKeyPath string `mapstructure:"ssl_key_path"`

	// 下面两个是显式开关，默认值都取「安全的那一侧」：
	// 想让它们变成 true，用环境变量显式覆盖（APP_APP_PRINT_CONFIG / APP_APP_EXPOSE_DEV_MSG）。
	//
	// 之前这里靠 app.env 是不是 "prod" 来决定行为，问题有两个：
	//   1. 部署时忘了改 env，行为就悄悄变了（比如 AutoMigrate 跑去生产库建表）
	//   2. 读代码时看不出「这次部署到底会怎样」，得先去查 env
	PrintConfig  bool `mapstructure:"print_config"`
	ExposeDevMsg bool `mapstructure:"expose_dev_msg"`
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

	// AutoMigrate 默认 false —— 生产环境的 schema 变更走 migrations/，
	// 不该由应用启动时自动建表。本地开发用 APP_DATABASE_AUTO_MIGRATE=true 打开。
	AutoMigrate bool `mapstructure:"auto_migrate"`
}

type RedisSection struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
	PoolSize int    `mapstructure:"pool_size"`
}

type RabbitMQSection struct {
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
}

type RegistrySection struct {
	Prefix string `mapstructure:"prefix"`
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
	RabbitMQ string `mapstructure:"rabbitmq"`
}

type ElasticsearchSection struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
}

type OrderMQConfig struct {
	Exchange      string `mapstructure:"exchange"`
	ConsumerQueue string `mapstructure:"consumer_queue"`
	DelayQueue    string `mapstructure:"delay_queue"`
	RoutingKey    string `mapstructure:"routing_key"`
	TTLMs         int    `mapstructure:"ttl_ms"`
}

func Init() (*AppConfig, error) {
	var cfg AppConfig
	_ = godotenv.Load()

	v := viper.New()

	v.SetEnvPrefix("APP")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	v.BindEnv("registry.prefix", "IMAGE_REPOSITORY")
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
	if c.Database.Host == "" {
		return errors.New("database.host required (ENV: APP_DATABASE_HOST)")
	}
	if c.Database.User == "" {
		return errors.New("database.user required")
	}
	if c.Database.Password == "" {
		return errors.New("database.password required (ENV: APP_DATABASE_PASSWORD)")
	}
	if c.RabbitMQ.Password == "" {
		return errors.New("rabbitmq.password required (ENV: APP_RABBITMQ_PASSWORD)")
	}

	// 这一条不再看 app.env —— 不管是哪个环境，空密钥和示例密钥都是错的。
	// 本地开发的值放到 .env 里（config.Init 会 godotenv.Load()）。
	if c.Auth.TokenSecret == "" {
		return errors.New("auth.token_secret 不能为空 (ENV: APP_AUTH_TOKEN_SECRET)")
	}
	if c.Auth.TokenSecret == "12345678" || strings.Contains(c.Auth.TokenSecret, "replace-with") {
		return errors.New("auth.token_secret 不能使用示例值 (ENV: APP_AUTH_TOKEN_SECRET)")
	}
	return nil
}

// ImageRef 拼接 registry 前缀与镜像名
func (c *AppConfig) ImageRef(image string) string {
	return c.Registry.Prefix + image
}
