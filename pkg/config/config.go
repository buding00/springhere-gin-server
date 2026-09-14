package config

import (
	"fmt"
	"net/url"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// DefaultPath is the configuration shipped at the repository root.
// A caller can still supply another path, for example in deployment manifests.
const DefaultPath = "application.yaml"

type Config struct {
	App      AppConfig      `yaml:"app"`
	Server   ServerConfig   `yaml:"server"`
	Logger   LoggerConfig   `yaml:"logger"`
	Database DatabaseConfig `yaml:"database"`
	Redis    RedisConfig    `yaml:"redis"`
	Auth     AuthConfig     `yaml:"auth"`
	Captcha  CaptchaConfig  `yaml:"captcha"`
}

type AppConfig struct {
	Env string `yaml:"env"`
}

type ServerConfig struct {
	Addr                string        `yaml:"addr"`
	ReadHeaderTimeout   time.Duration `yaml:"read_header_timeout"`
	ReadTimeout         time.Duration `yaml:"read_timeout"`
	WriteTimeout        time.Duration `yaml:"write_timeout"`
	ShutdownTimeout     time.Duration `yaml:"shutdown_timeout"`
	MaxHeaderBytes      int           `yaml:"max_header_bytes"`
	MaxRequestBodyBytes int64         `yaml:"max_request_body_bytes"`
}

type LoggerConfig struct {
	Level            string   `yaml:"level"`
	Format           string   `yaml:"format"`
	OutputPaths      []string `yaml:"output_paths"`
	ErrorOutputPaths []string `yaml:"error_output_paths"`
}

type DatabaseConfig struct {
	DSN string `yaml:"dsn"`
}

type RedisConfig struct {
	Addr         string        `yaml:"addr"`
	Username     string        `yaml:"username"`
	Password     string        `yaml:"password"`
	DB           int           `yaml:"db"`
	DialTimeout  time.Duration `yaml:"dial_timeout"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
}

type AuthConfig struct {
	JWTSecret           string        `yaml:"jwt_secret"`
	Issuer              string        `yaml:"issuer"`
	Audience            string        `yaml:"audience"`
	AccessTokenTTL      time.Duration `yaml:"access_token_ttl"`
	RefreshTokenTTL     time.Duration `yaml:"refresh_token_ttl"`
	RefreshCookieName   string        `yaml:"refresh_cookie_name"`
	RefreshCookieSecure bool          `yaml:"refresh_cookie_secure"`
	AllowedOrigins      []string      `yaml:"allowed_origins"`
}

type CaptchaConfig struct {
	Enabled             bool          `yaml:"enabled"`
	Length              int           `yaml:"length"`
	TTL                 time.Duration `yaml:"ttl"`
	Width               int           `yaml:"width"`
	Height              int           `yaml:"height"`
	IssueLimitPerMinute int           `yaml:"issue_limit_per_minute"`
}

func New(path string) Config {
	config, err := load(path)
	if err != nil {
		panic(err)
	}
	return config
}

func load(path string) (Config, error) {
	c := defaults()
	if path == "" {
		path = DefaultPath
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config %q: %w", path, err)
	}
	if err := yaml.Unmarshal(body, &c); err != nil {
		return Config{}, fmt.Errorf("parse config %q: %w", path, err)
	}
	override(&c)
	return c, c.Validate()
}

func defaults() Config {
	var c Config
	c.App.Env = "development"
	c.Server.Addr = ":8080"
	c.Server.ReadHeaderTimeout = 5 * time.Second
	c.Server.ReadTimeout = 10 * time.Second
	c.Server.WriteTimeout = 15 * time.Second
	c.Server.ShutdownTimeout = 10 * time.Second
	c.Server.MaxHeaderBytes = 1 << 20
	c.Server.MaxRequestBodyBytes = 1 << 20
	c.Logger.Level = "info"
	c.Logger.Format = "json"
	c.Logger.OutputPaths = []string{"stdout"}
	c.Logger.ErrorOutputPaths = []string{"stderr"}
	c.Redis.Addr = "127.0.0.1:6379"
	c.Redis.DialTimeout = 5 * time.Second
	c.Redis.ReadTimeout = 3 * time.Second
	c.Redis.WriteTimeout = 3 * time.Second
	c.Auth.Issuer = "springhere-gin-server"
	c.Auth.Audience = "springhere-react-admin"
	c.Auth.AccessTokenTTL = 15 * time.Minute
	c.Auth.RefreshTokenTTL = 30 * 24 * time.Hour
	c.Auth.RefreshCookieName = "springhere_refresh"
	c.Captcha.Enabled = true
	c.Captcha.Length = 4
	c.Captcha.TTL = 2 * time.Minute
	c.Captcha.Width = 160
	c.Captcha.Height = 48
	c.Captcha.IssueLimitPerMinute = 20
	return c
}

func override(c *Config) {
	if v := os.Getenv("APP_ENV"); v != "" {
		c.App.Env = v
	}
	if v := os.Getenv("SERVER_ADDR"); v != "" {
		c.Server.Addr = v
	}
	if v := os.Getenv("DATABASE_DSN"); v != "" {
		c.Database.DSN = v
	}
	if v := os.Getenv("REDIS_ADDR"); v != "" {
		c.Redis.Addr = v
	}
	if v := os.Getenv("REDIS_USERNAME"); v != "" {
		c.Redis.Username = v
	}
	if v := os.Getenv("REDIS_PASSWORD"); v != "" {
		c.Redis.Password = v
	}
	if v := os.Getenv("AUTH_JWT_SECRET"); v != "" {
		c.Auth.JWTSecret = v
	}
}

func (c Config) Validate() error {
	if c.Server.Addr == "" || c.Database.DSN == "" {
		return fmt.Errorf("server.addr and database.dsn are required")
	}
	if c.Server.ReadHeaderTimeout <= 0 || c.Server.ReadTimeout <= 0 || c.Server.WriteTimeout <= 0 || c.Server.ShutdownTimeout <= 0 || c.Server.MaxHeaderBytes <= 0 || c.Server.MaxRequestBodyBytes <= 0 {
		return fmt.Errorf("server timeouts and size limits must be positive")
	}
	if c.Redis.Addr == "" || c.Redis.DB < 0 || c.Redis.DialTimeout <= 0 || c.Redis.ReadTimeout <= 0 || c.Redis.WriteTimeout <= 0 {
		return fmt.Errorf("redis address, database index, and timeouts must be valid")
	}
	if len(c.Auth.JWTSecret) < 32 {
		return fmt.Errorf("auth.jwt_secret must be at least 32 bytes")
	}
	if c.Auth.Issuer == "" || c.Auth.Audience == "" || c.Auth.RefreshCookieName == "" {
		return fmt.Errorf("auth issuer, audience, and refresh cookie name are required")
	}
	if c.Auth.AccessTokenTTL <= 0 || c.Auth.RefreshTokenTTL <= 0 {
		return fmt.Errorf("token TTL must be positive")
	}
	if c.App.Env == "production" && !c.Auth.RefreshCookieSecure {
		return fmt.Errorf("production requires a secure refresh cookie")
	}
	if c.Captcha.TTL <= 0 || c.Captcha.Width <= 0 || c.Captcha.Height <= 0 || c.Captcha.IssueLimitPerMinute <= 0 {
		return fmt.Errorf("captcha ttl, size, and issue limit must be positive")
	}
	if c.Captcha.Length < 4 || c.Captcha.Length > 6 {
		return fmt.Errorf("captcha length must be between 4 and 6")
	}
	if c.App.Env == "production" && !c.Captcha.Enabled {
		return fmt.Errorf("production requires captcha")
	}
	for _, origin := range c.Auth.AllowedOrigins {
		u, err := url.ParseRequestURI(origin)
		if err != nil || u.Scheme == "" || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("auth.allowed_origins contains an invalid origin")
		}
	}
	return nil
}
