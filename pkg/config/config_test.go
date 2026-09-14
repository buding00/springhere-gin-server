package config

import (
	"testing"
	"time"
)

func TestValidateRequiresAuthIdentity(t *testing.T) {
	cfg := validConfig()
	cfg.Auth.RefreshCookieName = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected refresh cookie name to be required")
	}
}

func TestValidateRequiresCaptchaInProduction(t *testing.T) {
	cfg := validConfig()
	cfg.App.Env = "production"
	cfg.Auth.RefreshCookieSecure = true
	cfg.Captcha.Enabled = false
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected production captcha to be required")
	}
}

func TestValidateRejectsCaptchaLength(t *testing.T) {
	cfg := validConfig()
	cfg.Captcha.Length = 3
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected captcha length 3 to be rejected")
	}
	cfg.Captcha.Length = 7
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected captcha length 7 to be rejected")
	}
}

func TestValidateRejectsNonPositiveCaptchaSettings(t *testing.T) {
	cfg := validConfig()
	cfg.Captcha.TTL = 0
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected captcha ttl to be required")
	}
}

func validConfig() Config {
	cfg := defaults()
	cfg.Database.DSN = "postgres://springhere:springhere@localhost:5432/springhere?sslmode=disable"
	cfg.Auth.JWTSecret = "replace-with-at-least-32-byte-secret!"
	cfg.Auth.AccessTokenTTL = time.Minute
	cfg.Auth.RefreshTokenTTL = time.Hour
	return cfg
}
