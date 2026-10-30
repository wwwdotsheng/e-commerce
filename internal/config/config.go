package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
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
	GinMode    string `mapstructure:"gin_mode"`
	Name       string `mapstructure:"name"`
	Port       int    `mapstructure:"port"`
	SSL        bool   `mapstructure:"ssl"`
	SSLCrtPath string `mapstructure:"ssl_crt_path"`
	SSLKeyPath string `mapstructure:"ssl_key_path"`

	// These switches control diagnostics directly; no environment-name branches.
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
	Enabled bool   `mapstructure:"enabled"`
	Host    string `mapstructure:"host"`
	Port    int    `mapstructure:"port"`
}

type OrderMQConfig struct {
	Exchange      string `mapstructure:"exchange"`
	ConsumerQueue string `mapstructure:"consumer_queue"`
	DelayQueue    string `mapstructure:"delay_queue"`
	RoutingKey    string `mapstructure:"routing_key"`
	TTLMs         int    `mapstructure:"ttl_ms"`
}

// LoadOptions controls configuration sources, not deployment environments.
// CONFIG_PATH selects YAML; DOTENV_PATH opts into a dotenv file.
type LoadOptions struct {
	ConfigPath         string
	DotenvPath         string
	AllowMissingConfig bool
}

func Defaults() AppConfig {
	return AppConfig{
		App:           AppSection{Name: "ecommerce-api", Port: 8080, GinMode: "release"},
		Database:      DatabaseSection{Host: "localhost", Port: 5432, User: "postgres", DBName: "ecommerce", SSLMode: "disable", MaxIdleConns: 20, MaxOpenConns: 50, ConnMaxLifetime: 10, TimeZone: "Asia/Shanghai", LogLevel: "info"},
		Redis:         RedisSection{Host: "localhost", Port: 6379, PoolSize: 10},
		RabbitMQ:      RabbitMQSection{Host: "localhost", Port: 5672, User: "ecommerce"},
		Auth:          AuthSection{AccessTokenExpire: 15 * time.Minute, RefreshTokenExpire: 168 * time.Hour},
		Log:           LogSection{ConsoleLevel: "info", MaxSize: 100, MaxBackups: 3, MaxAge: 28},
		Otel:          OtelSection{ServiceName: "e-commerce"},
		Elasticsearch: ElasticsearchSection{Host: "localhost", Port: 9200},
		TestImages:    TestImagesSection{Postgres: "postgres:15-alpine", Redis: "redis:7-alpine", RabbitMQ: "rabbitmq:3-alpine"},
		OrderMQ:       OrderMQConfig{Exchange: "dlx_exchange", ConsumerQueue: "order_timeout_queue", DelayQueue: "order_delay_queue", RoutingKey: "timeout", TTLMs: 30000},
	}
}

func Init() (*AppConfig, error) {
	// An explicitly empty CONFIG_PATH selects environment-only configuration.
	path, explicit := os.LookupEnv("CONFIG_PATH")
	if !explicit {
		path = filepath.Join("configs", "config.yaml")
	}
	return Load(LoadOptions{ConfigPath: path, DotenvPath: os.Getenv("DOTENV_PATH"), AllowMissingConfig: !explicit})
}

// schemaFields includes zero-value fields so ENV-only loading and overrides for
// keys omitted from YAML behave exactly like fields present in the file.
func schemaFields(value reflect.Value, prefix string, visit func(string, interface{}) error) error {
	typ := value.Type()
	for i := 0; i < value.NumField(); i++ {
		key := typ.Field(i).Tag.Get("mapstructure")
		if key == "" || key == "-" {
			continue
		}
		if prefix != "" {
			key = prefix + "." + key
		}
		field := value.Field(i)
		if field.Kind() == reflect.Struct {
			if err := schemaFields(field, key, visit); err != nil {
				return err
			}
		} else if err := visit(key, field.Interface()); err != nil {
			return err
		}
	}
	return nil
}

func envNames(key string) []string {
	names := []string{"APP_" + strings.ToUpper(strings.ReplaceAll(key, ".", "_"))}
	// Backwards compatible aliases; the APP_ spelling takes priority.
	if key == "registry.prefix" {
		names = append(names, "IMAGE_REPOSITORY")
	}
	if key == "app.gin_mode" {
		names = append(names, "GIN_MODE")
	}
	return names
}

func setNested(root map[string]interface{}, key string, value interface{}) {
	parts := strings.Split(key, ".")
	current := root
	for _, part := range parts[:len(parts)-1] {
		next, ok := current[part].(map[string]interface{})
		if !ok {
			next = map[string]interface{}{}
			current[part] = next
		}
		current = next
	}
	current[parts[len(parts)-1]] = value
}

func Load(options LoadOptions) (*AppConfig, error) {
	if _, exists := os.LookupEnv("APP_APP_ENV"); exists {
		return nil, errors.New("APP_APP_ENV is no longer supported; configure individual features")
	}
	if _, exists := os.LookupEnv("APP_DATABASE_AUTO_MIGRATE"); exists {
		return nil, errors.New("APP_DATABASE_AUTO_MIGRATE has been removed; apply versioned migrations with Atlas")
	}
	v := viper.New()
	v.SetEnvPrefix("APP")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AllowEmptyEnv(true)
	if err := schemaFields(reflect.ValueOf(Defaults()), "", func(key string, value interface{}) error {
		v.SetDefault(key, value)
		args := append([]string{key}, envNames(key)...)
		return v.BindEnv(args...)
	}); err != nil {
		return nil, fmt.Errorf("bind configuration schema: %w", err)
	}
	if options.ConfigPath != "" {
		v.SetConfigFile(options.ConfigPath)
		if err := v.ReadInConfig(); err != nil {
			if !(options.AllowMissingConfig && errors.Is(err, os.ErrNotExist)) {
				return nil, fmt.Errorf("read configuration %s: %w", options.ConfigPath, err)
			}
		}
	}
	if options.DotenvPath != "" {
		entries, err := godotenv.Read(options.DotenvPath)
		if err != nil {
			return nil, fmt.Errorf("read dotenv %s: %w", options.DotenvPath, err)
		}
		if _, exists := entries["APP_APP_ENV"]; exists {
			return nil, errors.New("dotenv APP_APP_ENV is no longer supported")
		}
		if _, exists := entries["APP_DATABASE_AUTO_MIGRATE"]; exists {
			return nil, errors.New("dotenv APP_DATABASE_AUTO_MIGRATE has been removed; apply versioned migrations with Atlas")
		}
		overrides := map[string]interface{}{}
		_ = schemaFields(reflect.ValueOf(Defaults()), "", func(key string, _ interface{}) error {
			for _, name := range envNames(key) {
				if value, ok := entries[name]; ok {
					setNested(overrides, key, value)
					break
				}
			}
			return nil
		})
		if err := v.MergeConfigMap(overrides); err != nil {
			return nil, fmt.Errorf("merge dotenv: %w", err)
		}
	}
	for _, key := range []string{"auth.access_token_expire", "auth.refresh_token_expire"} {
		switch value := v.Get(key).(type) {
		case time.Duration:
		case string:
			if _, err := time.ParseDuration(value); err != nil {
				return nil, fmt.Errorf("%s must be a duration with units", key)
			}
		default:
			return nil, fmt.Errorf("%s must be a duration with units", key)
		}
	}
	var cfg AppConfig
	if err := v.UnmarshalExact(&cfg); err != nil {
		return nil, fmt.Errorf("decode configuration: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate configuration: %w", err)
	}
	return &cfg, nil
}

func (c *AppConfig) Validate() error {
	var problems []error
	required := func(key, value string) {
		if strings.TrimSpace(value) == "" {
			problems = append(problems, fmt.Errorf("%s required", key))
		}
	}
	port := func(key string, value int) {
		if value < 1 || value > 65535 {
			problems = append(problems, fmt.Errorf("%s must be between 1 and 65535", key))
		}
	}
	positive := func(key string, value int) {
		if value <= 0 {
			problems = append(problems, fmt.Errorf("%s must be positive", key))
		}
	}
	required("app.name", c.App.Name)
	port("app.port", c.App.Port)
	if c.App.GinMode != "release" && c.App.GinMode != "debug" && c.App.GinMode != "test" {
		problems = append(problems, errors.New("app.gin_mode must be release, debug or test"))
	}
	if c.App.SSL {
		required("app.ssl_crt_path", c.App.SSLCrtPath)
		required("app.ssl_key_path", c.App.SSLKeyPath)
	}
	required("database.host", c.Database.Host)
	port("database.port", c.Database.Port)
	required("database.user", c.Database.User)
	required("database.password", c.Database.Password)
	required("database.db_name", c.Database.DBName)
	positive("database.max_open_conns", c.Database.MaxOpenConns)
	if c.Database.MaxIdleConns < 0 || c.Database.MaxIdleConns > c.Database.MaxOpenConns {
		problems = append(problems, errors.New("database.max_idle_conns must be between zero and max_open_conns"))
	}
	positive("database.conn_max_lifetime", c.Database.ConnMaxLifetime)
	if _, err := time.LoadLocation(c.Database.TimeZone); err != nil {
		problems = append(problems, errors.New("database.time_zone must be a valid timezone"))
	}
	required("redis.host", c.Redis.Host)
	port("redis.port", c.Redis.Port)
	positive("redis.pool_size", c.Redis.PoolSize)
	if c.Redis.DB < 0 {
		problems = append(problems, errors.New("redis.db must be nonnegative"))
	}
	required("rabbitmq.host", c.RabbitMQ.Host)
	port("rabbitmq.port", c.RabbitMQ.Port)
	required("rabbitmq.user", c.RabbitMQ.User)
	required("rabbitmq.password", c.RabbitMQ.Password)
	if c.Auth.AccessTokenExpire <= 0 || c.Auth.RefreshTokenExpire <= 0 {
		problems = append(problems, errors.New("auth token lifetimes must be positive durations"))
	}
	if c.Auth.RefreshTokenExpire < c.Auth.AccessTokenExpire {
		problems = append(problems, errors.New("auth.refresh_token_expire must not be shorter than access_token_expire"))
	}
	if c.Otel.Enabled {
		required("otel.endpoint.trace_endpoint", c.Otel.Endpoint.Trace)
		required("otel.endpoint.metric_endpoint", c.Otel.Endpoint.Metric)
	}
	if c.Elasticsearch.Enabled {
		required("elasticsearch.host", c.Elasticsearch.Host)
		port("elasticsearch.port", c.Elasticsearch.Port)
	}
	required("order_mq.exchange", c.OrderMQ.Exchange)
	required("order_mq.consumer_queue", c.OrderMQ.ConsumerQueue)
	required("order_mq.delay_queue", c.OrderMQ.DelayQueue)
	required("order_mq.routing_key", c.OrderMQ.RoutingKey)
	positive("order_mq.ttl_ms", c.OrderMQ.TTLMs)
	if c.OrderMQ.DelayQueue == c.OrderMQ.ConsumerQueue {
		problems = append(problems, errors.New("order_mq delay and consumer queues must differ"))
	}
	for key, level := range map[string]string{"log.console_level": c.Log.ConsoleLevel, "database.log_level": c.Database.LogLevel, "log.file_level": c.Log.FileLevel} {
		if key == "log.file_level" && level == "" {
			continue
		}
		if key == "database.log_level" {
			if level != "silent" && level != "error" && level != "warn" && level != "info" {
				problems = append(problems, fmt.Errorf("%s is invalid", key))
			}
		} else {
			switch level {
			case "debug", "info", "warn", "error", "dpanic", "panic", "fatal":
			default:
				problems = append(problems, fmt.Errorf("%s is invalid", key))
			}
		}
	}
	if c.Log.FileLevel != "" {
		required("log.filename", c.Log.Filename)
	}
	if c.Log.MaxSize <= 0 || c.Log.MaxBackups < 0 || c.Log.MaxAge < 0 {
		problems = append(problems, errors.New("log rotation values are invalid"))
	}
	return errors.Join(problems...)
}

func (c AppConfig) Redacted() AppConfig {
	mask := func(value string) string {
		if value != "" {
			return "[REDACTED]"
		}
		return ""
	}
	c.Database.Password = mask(c.Database.Password)
	c.Redis.Password = mask(c.Redis.Password)
	c.RabbitMQ.Password = mask(c.RabbitMQ.Password)
	c.Auth.TokenSecret = mask(c.Auth.TokenSecret)
	return c
}

func (c *AppConfig) ImageRef(image string) string {
	if c.Registry.Prefix == "" {
		return image
	}
	return strings.TrimRight(c.Registry.Prefix, "/") + "/" + strings.TrimLeft(image, "/")
}
