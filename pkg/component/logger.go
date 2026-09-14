package component

import (
	"fmt"

	"github.com/fast-template/springhere-gin-server/pkg/config"
	"go.uber.org/zap"
)

func NewLogger(cfg config.LoggerConfig) (*zap.Logger, func()) {
	level := zap.NewAtomicLevel()
	if err := level.UnmarshalText([]byte(cfg.Level)); err != nil {
		panic(fmt.Errorf("initialize logger: parse log level: %w", err))
	}
	built := zap.NewProductionConfig()
	built.Level = level
	if cfg.Format == "console" {
		built.Encoding = "console"
	}
	if cfg.Format != "" && cfg.Format != "json" && cfg.Format != "console" {
		panic(fmt.Errorf("initialize logger: unsupported log format %q", cfg.Format))
	}
	if len(cfg.OutputPaths) > 0 {
		built.OutputPaths = cfg.OutputPaths
	}
	if len(cfg.ErrorOutputPaths) > 0 {
		built.ErrorOutputPaths = cfg.ErrorOutputPaths
	}
	logger, err := built.Build()
	if err != nil {
		panic(fmt.Errorf("initialize logger: build logger: %w", err))
	}
	return logger, func() { _ = logger.Sync() }
}
