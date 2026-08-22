package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config 全局配置结构
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Redis    RedisConfig    `mapstructure:"redis"`
	Auth     AuthConfig     `mapstructure:"auth"`
	KMS      KMSConfig      `mapstructure:"kms"`
	Informer InformerConfig `mapstructure:"informer"`
	Breaker  BreakerConfig  `mapstructure:"breaker"`
	Backup   BackupConfig   `mapstructure:"backup"`
	Log      LogConfig      `mapstructure:"log"`
	Security SecurityConfig `mapstructure:"security"`
}

type ServerConfig struct {
	Name       string `mapstructure:"name"`
	Mode       string `mapstructure:"mode"`
	Host       string `mapstructure:"host"`
	Port       int    `mapstructure:"port"`
	WorkerPort int    `mapstructure:"worker_port"`
}

type DatabaseConfig struct {
	Driver          string `mapstructure:"driver"`
	Host            string `mapstructure:"host"`
	Port            int    `mapstructure:"port"`
	Username        string `mapstructure:"username"`
	Password        string `mapstructure:"password"`
	DBName          string `mapstructure:"dbname"`
	SSLMode         string `mapstructure:"sslmode"`
	Timezone        string `mapstructure:"timezone"`
	MaxIdleConns    int    `mapstructure:"max_idle_conns"`
	MaxOpenConns    int    `mapstructure:"max_open_conns"`
	ConnMaxLifetime int    `mapstructure:"conn_max_lifetime"`
	AutoMigrate     bool   `mapstructure:"auto_migrate"`
}

func (c DatabaseConfig) DSN() string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s TimeZone=%s",
		c.Host, c.Port, c.Username, c.Password, c.DBName, c.SSLMode, c.Timezone)
}

type RedisConfig struct {
	Host          string        `mapstructure:"host"`
	Port          int           `mapstructure:"port"`
	Password      string        `mapstructure:"password"`
	DB            int           `mapstructure:"db"`
	PoolSize      int           `mapstructure:"pool_size"`
	MinIdleConns  int           `mapstructure:"min_idle_conns"`
	DialTimeout   time.Duration `mapstructure:"dial_timeout"`
	ReadTimeout   time.Duration `mapstructure:"read_timeout"`
	WriteTimeout  time.Duration `mapstructure:"write_timeout"`
}

func (c RedisConfig) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

type AuthConfig struct {
	JWTSecret        string `mapstructure:"jwt_secret"`
	JWTIssuer        string `mapstructure:"jwt_issuer"`
	AccessTokenTTL   int64  `mapstructure:"access_token_ttl"`
	RefreshTokenTTL  int64  `mapstructure:"refresh_token_ttl"`
	BcryptCost       int    `mapstructure:"bcrypt_cost"`
}

type KMSConfig struct {
	Provider         string `mapstructure:"provider"`
	LocalAESKeyHex   string `mapstructure:"local_aes_key_hex"`
	KeyVersion       string `mapstructure:"key_version"`
	PreviousAESKeyHex string `mapstructure:"previous_aes_key_hex"`
}

type InformerConfig struct {
	LazyStart          bool          `mapstructure:"lazy_start"`
	IdleTTL            time.Duration `mapstructure:"idle_ttl"`
	StartupConcurrency int           `mapstructure:"startup_concurrency"`
	ListQPS            int           `mapstructure:"list_qps"`
	ListBurst          int           `mapstructure:"list_burst"`
	ListPageSize       int64         `mapstructure:"list_page_size"`
	ListPageInterval   time.Duration `mapstructure:"list_page_interval"`
	ListTimeout        time.Duration `mapstructure:"list_timeout"`
	WatchBackoffMin    time.Duration `mapstructure:"watch_backoff_min"`
	WatchBackoffMax    time.Duration `mapstructure:"watch_backoff_max"`
	WatchMaxRetries    int           `mapstructure:"watch_max_retries"`
}

type BreakerConfig struct {
	OpenThreshold       int           `mapstructure:"open_threshold"`
	OpenDuration        time.Duration `mapstructure:"open_duration"`
	HalfOpenMaxRequests uint32        `mapstructure:"half_open_max_requests"`
}

type BackupConfig struct {
	DefaultStorage string             `mapstructure:"default_storage"`
	Local          LocalBackupConfig  `mapstructure:"local"`
	S3             *S3BackupConfig    `mapstructure:"s3"`
	NFS            *NFSBackupConfig   `mapstructure:"nfs"`
}

type LocalBackupConfig struct {
	BaseDir string `mapstructure:"base_dir"`
}

type S3BackupConfig struct {
	Endpoint  string `mapstructure:"endpoint"`
	AccessKey string `mapstructure:"access_key"`
	SecretKey string `mapstructure:"secret_key"`
	Bucket    string `mapstructure:"bucket"`
	Region    string `mapstructure:"region"`
	UseSSL    bool   `mapstructure:"use_ssl"`
}

type NFSBackupConfig struct {
	BasePath string `mapstructure:"base_path"`
}

type LogConfig struct {
	Level        string   `mapstructure:"level"`
	Format       string   `mapstructure:"format"`
	OutputPaths  []string `mapstructure:"output_paths"`
	ErrorFile    string   `mapstructure:"error_file"`
	MaxSize      int      `mapstructure:"max_size"`
	MaxAge       int      `mapstructure:"max_age"`
	MaxBackups   int      `mapstructure:"max_backups"`
	Compress     bool     `mapstructure:"compress"`
}

type SecurityConfig struct {
	CORSAllowOrigins     []string `mapstructure:"cors_allow_origins"`
	CORSAllowCredentials bool     `mapstructure:"cors_allow_credentials"`
	AuditBodyMaxBytes    int      `mapstructure:"audit_body_max_bytes"`
}

// Load 加载配置文件。configPath 为空时按默认路径查找
// 查找顺序: 命令行 --config > 环境变量 CONFIG_FILE > ./config.yaml > ./configs/config.yaml
func Load(configPath string) (*Config, error) {
	v := viper.New()

	v.SetConfigType("yaml")
	v.SetEnvPrefix("K8SPLATFORM")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		v.SetConfigName("config")
		v.AddConfigPath(".")
		v.AddConfigPath("./configs")
	}

	// 设置默认值
	setDefaults(v)

	if err := v.ReadInConfig(); err != nil {
		// 配置文件不存在仅 warn，允许全部用环境变量
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("read config error: %w", err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config error: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("server.name", "k8s-platform")
	v.SetDefault("server.mode", "debug")
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.worker_port", 8081)

	v.SetDefault("database.driver", "postgres")
	v.SetDefault("database.max_idle_conns", 10)
	v.SetDefault("database.max_open_conns", 50)

	v.SetDefault("redis.pool_size", 50)
	v.SetDefault("redis.db", 0)

	v.SetDefault("auth.jwt_issuer", "k8s-platform")
	v.SetDefault("auth.access_token_ttl", 7200)
	v.SetDefault("auth.refresh_token_ttl", 604800)
	v.SetDefault("auth.bcrypt_cost", 10)

	v.SetDefault("kms.provider", "local_aes")

	v.SetDefault("informer.lazy_start", true)
	v.SetDefault("informer.idle_ttl", "30m")
	v.SetDefault("informer.startup_concurrency", 2)
	v.SetDefault("informer.list_qps", 5)
	v.SetDefault("informer.list_burst", 20)
	v.SetDefault("informer.list_page_size", 500)
	v.SetDefault("informer.list_page_interval", "1s")
	v.SetDefault("informer.list_timeout", "60s")

	v.SetDefault("breaker.open_threshold", 5)
	v.SetDefault("breaker.open_duration", "5m")

	v.SetDefault("backup.default_storage", "local")
	v.SetDefault("backup.local.base_dir", "./data/backups")

	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "json")

	v.SetDefault("security.audit_body_max_bytes", 4096)
}

func (c *Config) validate() error {
	if c.Auth.JWTSecret == "" || len(c.Auth.JWTSecret) < 16 {
		return fmt.Errorf("auth.jwt_secret must be at least 16 characters")
	}
	if c.KMS.Provider == "local_aes" {
		if len(c.KMS.LocalAESKeyHex) != 64 {
			return fmt.Errorf("kms.local_aes_key_hex must be 64 chars (32 bytes hex) for AES-256, got %d", len(c.KMS.LocalAESKeyHex))
		}
	}
	return nil
}
