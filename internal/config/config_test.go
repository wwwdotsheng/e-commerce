package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func isolateConfigEnv(t *testing.T) {
	t.Helper()
	for _, entry := range os.Environ() {
		key, value, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "APP_") || key == "IMAGE_REPOSITORY" || key == "GIN_MODE" || key == "CONFIG_PATH" || key == "DOTENV_PATH" {
			t.Setenv(key, value)
			if err := os.Unsetenv(key); err != nil {
				t.Fatal(err)
			}
		}
	}
}
func requiredEnv(t *testing.T) {
	isolateConfigEnv(t)
	t.Helper()
	t.Setenv("APP_DATABASE_PASSWORD", "test-postgres-password")
	t.Setenv("APP_RABBITMQ_PASSWORD", "test-rabbit-password")
	t.Setenv("APP_AUTH_TOKEN_SECRET", "test-only-signing-secret")
}
func configFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
func TestEnvOnlyAndDefaults(t *testing.T) {
	requiredEnv(t)
	t.Setenv("APP_APP_PORT", "9091")
	t.Setenv("APP_AUTH_ACCESS_TOKEN_EXPIRE", "2m")
	t.Setenv("APP_ELASTICSEARCH_ENABLED", "false")
	cfg, err := Load(LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.App.Port != 9091 || cfg.Auth.AccessTokenExpire != 2*time.Minute {
		t.Fatal("ENV-only overrides missing")
	}
	if cfg.App.GinMode != "release" || cfg.Otel.Enabled || cfg.Elasticsearch.Enabled || cfg.App.ExposeDevMsg {
		t.Fatal("unsafe or environment-dependent defaults")
	}
}
func TestEnvOverridesOmittedYamlKey(t *testing.T) {
	requiredEnv(t)
	t.Setenv("APP_APP_EXPOSE_DEV_MSG", "true")
	cfg, err := Load(LoadOptions{ConfigPath: configFile(t, "app:\n  port: 8088\n")})
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.App.ExposeDevMsg || cfg.App.Port != 8088 {
		t.Fatal("a key omitted from YAML must still bind to ENV")
	}
}
func TestExplicitSourcePrecedence(t *testing.T) {
	isolateConfigEnv(t)
	t.Setenv("APP_AUTH_TOKEN_SECRET", "process-signing-secret")
	t.Setenv("APP_DATABASE_PASSWORD", "process-db-password")
	cfg, err := Load(LoadOptions{
		ConfigPath: configFile(t, "database:\n  password: yaml-db-password\nrabbitmq:\n  password: yaml-mq-password\nauth:\n  token_secret: yaml-signing-secret\napp:\n  port: 8081\n"),
		DotenvPath: configFile(t, "APP_DATABASE_PASSWORD=dotenv-db-password\nAPP_RABBITMQ_PASSWORD=dotenv-mq-password\nAPP_AUTH_TOKEN_SECRET=dotenv-signing-secret\nAPP_APP_PORT=8082\n"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Database.Password != "process-db-password" || cfg.Auth.TokenSecret != "process-signing-secret" ||
		cfg.RabbitMQ.Password != "dotenv-mq-password" || cfg.App.Port != 8082 {
		t.Fatal("expected ENV > dotenv > YAML > defaults")
	}
	if _, exists := os.LookupEnv("APP_APP_PORT"); exists {
		t.Fatal("dotenv loading must not mutate process environment")
	}
}
func TestExplicitEmptyEnvIsNotIgnored(t *testing.T) {
	requiredEnv(t)
	t.Setenv("APP_DATABASE_PASSWORD", "")
	_, err := Load(LoadOptions{ConfigPath: configFile(t, "database:\n  password: yaml-secret\n")})
	if err == nil || !strings.Contains(err.Error(), "database.password") {
		t.Fatal("blank ENV must override YAML and fail required validation")
	}
	if strings.Contains(err.Error(), "yaml-secret") {
		t.Fatal("validation error leaked a secret")
	}
}
func TestConfigurationFileErrors(t *testing.T) {
	requiredEnv(t)
	for _, tc := range []struct{ name, content string }{
		{"invalid YAML", "app: [broken"},
		{"removed AutoMigrate option", "database:\n  auto_migrate: true\n"},
		{"duration without units", "auth:\n  access_token_expire: 15\n"},
		{"unknown key", "database:\n  auto_migratte: true\n"},
		{"removed environment selector", "app:\n  env: Dev\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Load(LoadOptions{ConfigPath: configFile(t, tc.content)}); err == nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
	missing := filepath.Join(t.TempDir(), "missing.yaml")
	if _, err := Load(LoadOptions{ConfigPath: missing}); err == nil {
		t.Fatal("explicit missing path accepted")
	}
	if _, err := Load(LoadOptions{ConfigPath: missing, AllowMissingConfig: true}); err != nil {
		t.Fatal(err)
	}
}
func TestRemovedEnvironmentVariable(t *testing.T) {
	requiredEnv(t)
	t.Setenv("APP_APP_ENV", "Prod")
	if _, err := Load(LoadOptions{}); err == nil || !strings.Contains(err.Error(), "APP_APP_ENV") {
		t.Fatal("removed environment selector silently accepted")
	}
}
func TestRemovedAutoMigrationEnvironmentVariable(t *testing.T) {
	requiredEnv(t)
	t.Setenv("APP_DATABASE_AUTO_MIGRATE", "true")
	if _, err := Load(LoadOptions{}); err == nil || !strings.Contains(err.Error(), "has been removed") {
		t.Fatal("removed AutoMigration ENV must fail with migration guidance")
	}

	requiredEnv(t)
	dotenv := configFile(t, "APP_DATABASE_AUTO_MIGRATE=true\n")
	if _, err := Load(LoadOptions{DotenvPath: dotenv}); err == nil || !strings.Contains(err.Error(), "has been removed") {
		t.Fatal("removed AutoMigration dotenv key must fail with migration guidance")
	}
}

func TestMissingDotenvFails(t *testing.T) {
	requiredEnv(t)
	if _, err := Load(LoadOptions{DotenvPath: filepath.Join(t.TempDir(), "missing.env")}); err == nil {
		t.Fatal("explicit dotenv path should fail when missing")
	}
}
func TestFeatureDependencyValidation(t *testing.T) {
	requiredEnv(t)
	for _, tc := range []struct {
		name, content string
		wantError     bool
	}{
		{"disabled telemetry", "otel:\n  enabled: false\n", false},
		{"enabled telemetry without endpoints", "otel:\n  enabled: true\n", true},
		{"configured telemetry", "otel:\n  enabled: true\n  endpoint:\n    trace_endpoint: tempo:4317\n    metric_endpoint: prometheus:9090\n", false},
		{"disabled search", "elasticsearch:\n  enabled: false\n  host: \"\"\n", false},
		{"enabled search without host", "elasticsearch:\n  enabled: true\n  host: \"\"\n", true},
		{"TLS without certificates", "app:\n  ssl: true\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(LoadOptions{ConfigPath: configFile(t, tc.content)})
			if (err != nil) != tc.wantError {
				t.Fatalf("error=%v, wantError=%v", err, tc.wantError)
			}
		})
	}
}
func TestInvalidTypedSettings(t *testing.T) {
	requiredEnv(t)
	for _, tc := range []struct{ key, value string }{
		{"APP_APP_PORT", "0"}, {"APP_APP_PORT", "65536"}, {"APP_AUTH_ACCESS_TOKEN_EXPIRE", "0s"},
		{"APP_AUTH_ACCESS_TOKEN_EXPIRE", "bad"}, {"APP_AUTH_ACCESS_TOKEN_EXPIRE", "10"},
		{"APP_AUTH_REFRESH_TOKEN_EXPIRE", "1m"}, {"APP_ORDER_MQ_TTL_MS", "0"},
		{"APP_DATABASE_MAX_IDLE_CONNS", "51"}, {"APP_REDIS_POOL_SIZE", "0"},
		{"APP_LOG_CONSOLE_LEVEL", "verbose"}, {"APP_APP_GIN_MODE", "Prod"},
	} {
		t.Run(tc.key+"="+tc.value, func(t *testing.T) {
			t.Setenv(tc.key, tc.value)
			if _, err := Load(LoadOptions{}); err == nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
}
func TestLegacyAliasesAndCanonicalPrecedence(t *testing.T) {
	requiredEnv(t)
	t.Setenv("IMAGE_REPOSITORY", "mirror.example/cache")
	t.Setenv("GIN_MODE", "debug")
	cfg, err := Load(LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ImageRef("postgres:15") != "mirror.example/cache/postgres:15" || cfg.App.GinMode != "debug" {
		t.Fatal("legacy aliases not respected")
	}
	t.Setenv("APP_REGISTRY_PREFIX", "preferred.example/")
	t.Setenv("APP_APP_GIN_MODE", "release")
	cfg, err = Load(LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ImageRef("postgres:15") != "preferred.example/postgres:15" || cfg.App.GinMode != "release" {
		t.Fatal("canonical aliases must win")
	}
}
func TestRedactedDiagnostics(t *testing.T) {
	original := Defaults()
	original.Database.Password = "db-sensitive"
	original.Redis.Password = "redis-sensitive"
	original.RabbitMQ.Password = "mq-sensitive"
	original.Auth.TokenSecret = "jwt-sensitive"
	safe := original.Redacted()
	raw, err := json.Marshal(safe)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"db-sensitive", "redis-sensitive", "mq-sensitive", "jwt-sensitive"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("secret present in diagnostics")
		}
	}
	if original.Database.Password != "db-sensitive" || original.Auth.TokenSecret != "jwt-sensitive" {
		t.Fatal("redaction mutated the active configuration")
	}
}

func TestInitConfigPathSelection(t *testing.T) {
	t.Run("unset path reads default YAML", func(t *testing.T) {
		requiredEnv(t)
		root := t.TempDir()
		t.Chdir(root)
		configDir := filepath.Join(root, "configs")
		if err := os.MkdirAll(configDir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte("app:\n  port: 8181\n"), 0600); err != nil {
			t.Fatal(err)
		}

		cfg, err := Init()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.App.Port != 8181 {
			t.Fatalf("default YAML port = %d, want 8181", cfg.App.Port)
		}
	})

	t.Run("explicit empty path skips default YAML", func(t *testing.T) {
		requiredEnv(t)
		t.Setenv("CONFIG_PATH", "")
		root := t.TempDir()
		t.Chdir(root)
		configDir := filepath.Join(root, "configs")
		if err := os.MkdirAll(configDir, 0700); err != nil {
			t.Fatal(err)
		}
		// This invalid default file proves that an explicit empty path is not read.
		if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte("invalid: [yaml"), 0600); err != nil {
			t.Fatal(err)
		}

		cfg, err := Init()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.App.Port != Defaults().App.Port {
			t.Fatalf("ENV-only default port = %d, want %d", cfg.App.Port, Defaults().App.Port)
		}
	})
}
